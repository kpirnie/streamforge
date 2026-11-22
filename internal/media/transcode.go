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
	codec := c.Codec
	hdr := false
	sourceCodec := ""
	for _, stream := range streams {
		if stream.Type != "video" {
			continue
		}
		if sourceCodec == "" {
			sourceCodec = stream.Codec
		}
		hdr = hdr || stream.Transfer == "smpte2084" || stream.Transfer == "arib-std-b67"
	}
	toneMap := hdr && c.ConvertHDR
	if codec == "keep" && toneMap {
		codec = sourceCodec
	}
	if codec == "keep" {
		codec = "copy"
	}
	if !c.Force && !toneMap && (codec == sourceCodec || (codec == "copy" && filepath.Ext(input) == filepath.Ext(target))) {
		return "", true, 0, nil
	}
	if codec != "copy" && hdr && !toneMap {
		return "", false, 0, errors.New("re-encoding HDR requires --convert-hdr to avoid losing HDR metadata")
	}
	exists, owned, err := ownership(root, target, input)
	if err != nil {
		return "", false, 0, err
	}
	if exists && !owned {
		return "", false, 0, errors.New("refusing to replace unrelated output")
	}
	if owned && !c.Force {
		return "", true, 0, nil
	}
	cpu := map[string]string{"copy": "copy", "hevc": "libx265", "av1": "libsvtav1", "h264": "libx264"}[codec]
	if cpu == "" {
		return "", false, 0, errors.New("source video codec has no supported tone-map encoder")
	}
	hardware := c.hardwareEncoder(codec, encoders, toneMap)
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
		err := AtomicWrite(ctx, root, target, info.Mode().Perm(), owned && c.Force, func(output *os.File) error {
			if err := c.encode(ctx, source, output, current, muxer, toneMap); err != nil {
				return err
			}
			_, err := Probe(ctx, c.FFprobe, output)
			if err == nil {
				err = output.Chmod(info.Mode().Perm())
				if err == nil {
					attrErr := syscall.Setxattr(descriptor(output), sourceAttribute, []byte(fingerprint(input)), 0)
					if attrErr != nil && !errors.Is(attrErr, syscall.ENOTSUP) {
						err = attrErr
					}
				}
			}
			return err
		})
		if err == nil {
			return current, false, i, nil
		}
		if errors.Is(err, os.ErrExist) && !c.Force {
			return current, true, i, nil
		}
		if i == len(attempts)-1 {
			return current, false, i, err
		}
	}
	return "", false, 0, errors.New("no available encoder")
}

func listed(inventory, encoder string) bool {
	return strings.Contains(inventory, " "+encoder+" ")
}

func (c Conversion) hardwareEncoder(codec, inventory string, toneMap bool) string {
	if c.Hardware == "cpu" || codec == "copy" || toneMap {
		return ""
	}
	for _, option := range []struct{ backend, suffix, device string }{
		{"nvidia", "nvenc", "/dev/nvidiactl"}, {"intel", "qsv", "/dev/dri/renderD128"}, {"amd", "vaapi", "/dev/dri/renderD128"},
	} {
		if c.Hardware != "auto" && c.Hardware != option.backend {
			continue
		}
		if _, err := os.Stat(option.device); err == nil && listed(inventory, codec+"_"+option.suffix) {
			return codec + "_" + option.suffix
		}
	}
	return ""
}

func (c Conversion) encode(ctx context.Context, input, output *os.File, encoder, muxer string, toneMap bool) error {
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y"}
	if strings.HasSuffix(encoder, "_vaapi") {
		args = append(args, "-vaapi_device", "/dev/dri/renderD128")
	}
	args = append(args, "-protocol_whitelist", "file,pipe", "-i", "/proc/self/fd/3", "-map", "0", "-map_metadata", "0", "-map_chapters", "0", "-c", "copy", "-c:v", encoder)
	if toneMap {
		args = append(args, "-vf", "zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv,format=yuv420p", "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709")
	}
	quality := map[string]int{"high": 20, "balanced": 24, "compression": 29}[c.Quality]
	switch {
	case encoder == "libsvtav1":
		args = append(args, "-crf", fmt.Sprint(quality+6), "-preset", "6")
	case encoder == "libx265" || encoder == "libx264":
		args = append(args, "-crf", fmt.Sprint(quality), "-preset", "medium")
	case strings.HasSuffix(encoder, "_nvenc"):
		args = append(args, "-cq", fmt.Sprint(quality), "-preset", "p5")
	case strings.HasSuffix(encoder, "_qsv"):
		args = append(args, "-global_quality", fmt.Sprint(quality))
	case strings.HasSuffix(encoder, "_vaapi"):
		args = append(args, "-vf", "format=nv12,hwupload", "-qp", fmt.Sprint(quality))
	}
	args = append(args, "-f", muxer, "/proc/self/fd/4")
	command := exec.CommandContext(ctx, c.FFmpeg, args...)
	command.ExtraFiles = []*os.File{input, output}
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("FFmpeg could not encode the selected streams in the output container")
	}
	return nil
}
