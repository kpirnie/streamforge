// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// Conversion is the session-wide profile used by FFmpeg.
type Conversion struct {
	FFmpeg     string
	FFprobe    string
	Codec      string
	Container  string
	Quality    string
	Hardware   string
	ConvertHDR bool
	Force      bool
	Threads    int
	Resolution string
	FrameRate  string
	Aspect     string
}

// Validate checks the supported profile and host executable paths.
func (c Conversion) Validate() error {
	switch c.Codec {
	case "keep", "hevc", "av1":
	default:
		return errors.New("codec must be keep, hevc, or av1")
	}
	switch c.Container {
	case "keep", "mkv", "mp4":
	default:
		return errors.New("container must be keep, mkv, or mp4")
	}
	switch c.Quality {
	case "balanced", "high", "compression":
	default:
		return errors.New("quality must be balanced, high, or compression")
	}
	switch c.Hardware {
	case "auto", "cpu", "nvidia", "intel", "amd":
	default:
		return errors.New("hardware must be auto, cpu, nvidia, intel, or amd")
	}
	if c.Resolution != "" && !resolutionPattern.MatchString(c.Resolution) {
		return errors.New("resolution must be WIDTHxHEIGHT with positive dimensions")
	}
	if !validRatio(c.FrameRate, "/") || !validRatio(c.Aspect, ":") {
		return errors.New("invalid frame rate or aspect ratio")
	}
	for _, binary := range []string{c.FFmpeg, c.FFprobe} {
		if !filepath.IsAbs(binary) {
			return errors.New("FFmpeg and FFprobe paths must be absolute")
		}
		info, err := os.Stat(binary)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return errors.New("host FFmpeg and FFprobe executables are required")
		}
	}
	return nil
}

// Target determines the output location and the explicit FFmpeg muxer.
func (c Conversion) Target(input string) (string, string) {
	relative := strings.TrimPrefix(input, "input/")
	extension := filepath.Ext(relative)
	stem := strings.TrimSuffix(relative, extension)
	container := c.Container
	if container == "keep" {
		switch strings.ToLower(extension) {
		case ".mp4", ".m4v":
			return filepath.Join("output", relative), "mp4"
		case ".mkv":
			return filepath.Join("output", relative), "matroska"
		case ".mov":
			return filepath.Join("output", relative), "mov"
		case ".ts", ".m2ts":
			return filepath.Join("output", relative), "mpegts"
		case ".webm":
			if c.Codec != "hevc" && !c.ConvertHDR {
				return filepath.Join("output", relative), "webm"
			}
		}
		container = "mkv"
	}
	if container == "mp4" {
		return filepath.Join("output", stem+".mp4"), "mp4"
	}
	return filepath.Join("output", stem+".mkv"), "matroska"
}

// EncoderInventory detects available FFmpeg video encoders once per batch.
func (c Conversion) EncoderInventory(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, c.FFmpeg, "-hide_banner", "-encoders").Output()
	if err != nil {
		return "", errors.New("could not query FFmpeg encoders")
	}
	return string(output), nil
}

// Convert probes a source, applies the profile and publishes a validated output.
func (c Conversion) Convert(ctx context.Context, root *os.Root, input, target, muxer, encoders string) (encoder string, skipped bool, retries int, err error) {
	info, err := root.Lstat(input)
	if err != nil {
		return "", false, 0, err
	}
	if !info.Mode().IsRegular() {
		return "", false, 0, errors.New("source is not a regular media file")
	}
	source, err := root.OpenFile(input, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", false, 0, err
	}
	defer source.Close()
	streams, err := Probe(ctx, c.FFprobe, source)
	if err != nil {
		return "", false, 0, err
	}
	subtitle, err := openSidecar(root, input)
	if err != nil {
		return "", false, 0, err
	}
	var subtitleCodec string
	if subtitle != nil {
		defer subtitle.Close()
		switch muxer {
		case "matroska":
			subtitleCodec = "subrip"
		case "mp4":
			subtitleCodec = "mov_text"
		default:
			return "", false, 0, fmt.Errorf("cannot embed external SRT in %s output; select MKV or MP4", muxer)
		}
	}
	codec := c.Codec
	hdr := false
	sourceCodec := ""
	var hdrStream Stream
	videoCount := 0
	subtitleIndex := 0
	for _, stream := range streams {
		if stream.Type == "subtitle" {
			subtitleIndex++
		}
		if stream.Type != "video" {
			continue
		}
		videoCount++
		if sourceCodec == "" {
			sourceCodec = stream.Codec
		}
		if stream.Transfer == "smpte2084" || stream.Transfer == "arib-std-b67" {
			hdr = true
			hdrStream = stream
		}
	}
	toneMap := hdr && c.ConvertHDR
	mustEncode := toneMap || c.Resolution != "" || c.FrameRate != ""
	if codec == "keep" && mustEncode {
		codec = sourceCodec
	}
	if codec == "keep" {
		codec = "copy"
	}
	if !c.Force && !mustEncode && c.Aspect == "" && (codec == sourceCodec || (codec == "copy" && filepath.Ext(input) == filepath.Ext(target))) {
		if subtitle == nil {
			return "", true, 0, nil
		}
		codec = "copy"
	}
	var hdrArgs []string
	if codec != "copy" && hdr && !toneMap {
		if codec != "hevc" || videoCount != 1 || hdrStream.PixelFormat != "yuv420p10le" {
			return "", false, 0, errors.New("this HDR source requires stream copy, HEVC 10-bit, or explicit SDR conversion")
		}
		hdrArgs, err = c.hdrOptions(ctx, source, hdrStream)
		if err != nil {
			return "", false, 0, err
		}
	}
	exists, owned, err := ownership(root, target, input)
	if err != nil {
		return "", false, 0, err
	}
	if exists && !owned {
		return "", false, 0, errors.New("refusing to replace unrelated output")
	}
	if owned && !c.Force {
		if subtitle == nil {
			return "", true, 0, nil
		}
		output, err := root.OpenFile(target, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return "", false, 0, err
		}
		existing, err := Probe(ctx, c.FFprobe, output)
		output.Close()
		if err == nil && hasSidecarStream(existing, subtitleIndex, subtitleCodec) {
			return "", true, 0, nil
		}
	}
	cpu := map[string]string{"copy": "copy", "hevc": "libx265", "av1": "libsvtav1", "h264": "libx264", "vp9": "libvpx-vp9"}[codec]
	if cpu == "" {
		return "", false, 0, errors.New("source video codec has no supported tone-map encoder")
	}
	hardware := c.hardwareEncoder(codec, encoders, toneMap || len(hdrArgs) > 0)
	attempts := []string{cpu}
	if hardware != "" {
		attempts = []string{hardware, hardware, cpu}
	}
	for i, current := range attempts {
		if ctx.Err() != nil {
			return current, false, i, ctx.Err()
		}
		if current != "copy" && !listed(encoders, current) {
			return current, false, i, errors.New("required FFmpeg encoder is unavailable")
		}
		err := AtomicWrite(ctx, root, target, 0600, owned, func(output *os.File) error {
			if err := c.encode(ctx, source, output, subtitle, subtitleIndex, subtitleCodec, current, muxer, toneMap, hdrArgs); err != nil {
				return err
			}
			validated, err := Probe(ctx, c.FFprobe, output)
			if err == nil && subtitle != nil && !hasSidecarStream(validated, subtitleIndex, subtitleCodec) {
				err = errors.New("output did not retain the additional English subtitle stream")
			}
			if err == nil && len(hdrArgs) > 0 {
				preserved := false
				for _, stream := range validated {
					if stream.Type == "video" && stream.Transfer == hdrStream.Transfer && stream.PixelFormat == hdrStream.PixelFormat {
						preserved = true
						break
					}
				}
				if !preserved {
					err = errors.New("output did not retain the source HDR transfer and pixel format")
				}
			}
			if err == nil {
				if err := markSource(output, input); err != nil {
					return err
				}
				if stat, ok := info.Sys().(*syscall.Stat_t); ok {
					ownershipErr := output.Chown(int(stat.Uid), int(stat.Gid))
					if ownershipErr != nil && !errors.Is(ownershipErr, syscall.EPERM) {
						return ownershipErr
					}
				}
				err = output.Chmod(info.Mode().Perm())
			}
			return err
		})
		if err == nil {
			return current, false, i, nil
		}
		if errors.Is(err, os.ErrExist) && !c.Force && subtitle == nil {
			return current, true, i, nil
		}
		if i == len(attempts)-1 {
			return current, false, i, err
		}
	}
	return "", false, 0, errors.New("no available encoder")
}

func openSidecar(root *os.Root, input string) (*os.File, error) {
	path := strings.TrimSuffix(input, filepath.Ext(input)) + ".srt"
	// Reject symlinks and prevent a named pipe from blocking the open.
	file, err := root.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open external SRT: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("inspect external SRT: %w", err)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, errors.New("external SRT is not a regular file")
	}
	return file, nil
}

func hasSidecarStream(streams []Stream, index int, codec string) bool {
	count := 0
	found := false
	for _, stream := range streams {
		if stream.Type != "subtitle" {
			continue
		}
		if count == index {
			found = stream.Codec == codec && stream.Tags.Language == "eng"
		}
		count++
	}
	return found && count == index+1
}

func listed(inventory, encoder string) bool {
	return strings.Contains(inventory, " "+encoder+" ")
}

func (c Conversion) hardwareEncoder(codec, inventory string, toneMap bool) string {
	if c.Hardware == "cpu" || codec == "copy" || toneMap {
		return ""
	}
	for _, option := range []struct{ backend, suffix string }{
		{"nvidia", "nvenc"}, {"intel", "qsv"}, {"amd", "vaapi"},
	} {
		if c.Hardware != "auto" && c.Hardware != option.backend {
			continue
		}
		if hardwareDevice(option.backend) != "" && listed(inventory, codec+"_"+option.suffix) {
			return codec + "_" + option.suffix
		}
	}
	return ""
}

func (c Conversion) encode(ctx context.Context, input, output, subtitle *os.File, subtitleIndex int, subtitleCodec, encoder, muxer string, toneMap bool, hdrArgs []string) error {
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y"}
	threads := fmt.Sprint(max(1, c.Threads))
	args = append(args, "-filter_threads", threads, "-filter_complex_threads", threads)
	if strings.HasSuffix(encoder, "_vaapi") {
		args = append(args, "-vaapi_device", hardwareDevice("amd"))
	}
	args = append(args, "-protocol_whitelist", "file,pipe", "-threads", threads, "-i", "/proc/self/fd/3")
	if subtitle != nil {
		args = append(args, "-protocol_whitelist", "file,pipe", "-f", "srt", "-i", "/proc/self/fd/5")
	}
	args = append(args, "-map", "0", "-map_metadata", "0", "-map_chapters", "0", "-c", "copy", "-c:v", encoder, "-threads:v", threads)
	if subtitle != nil {
		specifier := ":s:" + strconv.Itoa(subtitleIndex)
		args = append(args, "-map", "1:s:0", "-c"+specifier, subtitleCodec, "-metadata"+specifier, "language=eng")
	}
	var filters []string
	if toneMap {
		filters = append(filters, "zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv,format=yuv420p")
		args = append(args, "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709")
	}
	if c.Resolution != "" {
		filters = append(filters, "scale="+strings.Replace(c.Resolution, "x", ":", 1))
	}
	if strings.HasSuffix(encoder, "_vaapi") {
		filters = append(filters, "format=nv12", "hwupload")
	}
	if len(filters) > 0 {
		args = append(args, "-vf", strings.Join(filters, ","))
	}
	if c.FrameRate != "" {
		args = append(args, "-r", c.FrameRate)
	}
	if c.Aspect != "" {
		args = append(args, "-aspect", c.Aspect)
	}
	args = append(args, hdrArgs...)
	quality := map[string]int{"high": 20, "balanced": 26, "compression": 32}[c.Quality]
	switch {
	case encoder == "libvpx-vp9":
		args = append(args, "-crf", fmt.Sprint(quality), "-b:v", "0")
	case encoder == "libsvtav1":
		args = append(args, "-crf", fmt.Sprint(quality+6), "-preset", "6")
	case encoder == "libx265" || encoder == "libx264":
		if encoder == "libx265" && len(hdrArgs) == 0 {
			args = append(args, "-x265-params", "pools="+threads+":frame-threads=1")
		}
		args = append(args, "-crf", fmt.Sprint(quality), "-preset", "medium")
	case strings.HasSuffix(encoder, "_nvenc"):
		args = append(args, "-cq", fmt.Sprint(quality), "-preset", "p5")
	case strings.HasSuffix(encoder, "_qsv"):
		args = append(args, "-global_quality", fmt.Sprint(quality))
	case strings.HasSuffix(encoder, "_vaapi"):
		args = append(args, "-qp", fmt.Sprint(quality))
	}
	args = append(args, "-f", muxer, "/proc/self/fd/4")
	command := exec.CommandContext(ctx, c.FFmpeg, args...)
	command.ExtraFiles = []*os.File{input, output}
	if subtitle != nil {
		command.ExtraFiles = append(command.ExtraFiles, subtitle)
	}
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("FFmpeg could not encode the selected streams in the output container")
	}
	return nil
}

var resolutionPattern = regexp.MustCompile(`^[1-9][0-9]{0,4}x[1-9][0-9]{0,4}$`)

func validRatio(raw, separator string) bool {
	if raw == "" {
		return true
	}
	left, right, ok := strings.Cut(raw, separator)
	if ok {
		first, err := strconv.ParseUint(left, 10, 32)
		second, err2 := strconv.ParseUint(right, 10, 32)
		return err == nil && err2 == nil && first > 0 && second > 0
	}
	if strings.ContainsAny(raw, "/+-") {
		return false
	}
	value, err := strconv.ParseFloat(raw, 64)
	return err == nil && value > 0 && value <= 1000
}
