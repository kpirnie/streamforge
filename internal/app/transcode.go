// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/kpirnie/streamforge/internal/media"
)

var downloadedEpisodePath = regexp.MustCompile(`^input/provider-[0-9a-f]{16}/[0-9]+/([^/]+) - S([0-9]{2,})E[0-9]{2,}$`)

type downloadedMedia struct {
	directory string
	movie     bool
}

func runTranscode(ctx context.Context, args []string, series bool, downloads map[string]downloadedMedia, titles *media.Titles, value func(string, string) string, dryRun, yes bool, report func(string, ...any), progress *operationProgress, finishItem func(string) error) error {
	profile := media.Conversion{
		FFmpeg:     mediaExecutable(value, "ffmpeg"),
		FFprobe:    mediaExecutable(value, "ffprobe"),
		Codec:      value("STREAMFORGE_CODEC", "hevc"),
		Container:  value("STREAMFORGE_CONTAINER", "keep"),
		Quality:    value("STREAMFORGE_QUALITY", "script"),
		Hardware:   value("STREAMFORGE_HARDWARE", "auto"),
		Resolution: value("STREAMFORGE_RESOLUTION", ""),
		FrameRate:  value("STREAMFORGE_FRAME_RATE", ""),
		Aspect:     value("STREAMFORGE_ASPECT", ""),
	}
	parent := value("STREAMFORGE_PLAYBACK_PARENT", "")
	exclude := value("STREAMFORGE_EXCLUDE", "")
	jobs := value("STREAMFORGE_JOBS", "")
	var target string
	var codecSet, containerSet, qualitySet, hdrSet bool
	force, err := strconv.ParseBool(value("STREAMFORGE_FORCE", "false"))
	if err != nil {
		return errors.New("STREAMFORGE_FORCE must be a boolean")
	}
	convertHDR, err := strconv.ParseBool(value("STREAMFORGE_CONVERT_HDR", "false"))
	if err != nil {
		return errors.New("STREAMFORGE_CONVERT_HDR must be a boolean")
	}
	remove, err := strconv.ParseBool(value("STREAMFORGE_DELETE_ORIGINAL", "false"))
	if err != nil {
		return errors.New("STREAMFORGE_DELETE_ORIGINAL must be a boolean")
	}
	profile.Force, profile.ConvertHDR = force, convertHDR
	for _, arg := range args {
		key, option, supplied := strings.Cut(arg, "=")
		switch key {
		case "--codec":
			profile.Codec, codecSet = option, true
		case "--container":
			profile.Container, containerSet = option, true
		case "--quality":
			profile.Quality, qualitySet = option, true
		case "--resolution":
			profile.Resolution = option
		case "--frame-rate":
			profile.FrameRate = option
		case "--aspect":
			profile.Aspect = option
		case "--hardware":
			profile.Hardware = option
		case "--jobs":
			jobs = option
		case "--exclude":
			exclude = option
		case "--playback-parent":
			parent = option
		case "--convert-hdr", "--force", "--delete-original":
			enabled := true
			if supplied {
				enabled, err = strconv.ParseBool(option)
				if err != nil {
					return errors.New("transcode switch must be a boolean")
				}
			}
			switch key {
			case "--convert-hdr":
				profile.ConvertHDR, hdrSet = enabled, true
			case "--force":
				profile.Force = enabled
			case "--delete-original":
				remove = enabled
			}
		default:
			if strings.HasPrefix(arg, "-") || target != "" {
				return errors.New("unexpected transcode argument")
			}
			target = arg
		}
	}
	if target == "" && len(downloads) == 0 {
		return errors.New("transcode requires a media path")
	}
	if parent != "" && !media.ValidPlaybackParent(parent) {
		return errors.New("playback parent must be an absolute path")
	}
	if remove && !dryRun && !yes {
		return errors.New("deleting originals requires --yes")
	}
	capacity := max(runtime.GOMAXPROCS(0), 1)
	parallel := max(1, capacity/2)
	if jobs != "" {
		count, err := strconv.Atoi(jobs)
		if err != nil || count < 1 {
			return errors.New("transcode jobs must be a positive integer")
		}
		parallel = count
	}
	root, err := os.OpenRoot(media.Working)
	if err != nil {
		return err
	}
	defer root.Close()
	var patterns []string
	if exclude != "" {
		patterns = strings.Split(exclude, ",")
	}
	fromDownload := len(downloads) > 0
	requested := slices.Sorted(maps.Keys(downloads))
	if !fromDownload {
		requested = []string{target}
	}
	var files []string
	for _, path := range requested {
		found, err := media.Files(root, path, patterns)
		if err != nil {
			return err
		}
		files = append(files, found...)
	}
	movieProfile, seriesProfile := profile, profile
	seriesProfile.Series = true
	seriesProfile.ConvertHDR = !hdrSet || profile.ConvertHDR
	if !codecSet && profile.Codec == "keep" {
		movieProfile.Codec, seriesProfile.Codec = "hevc", "hevc"
	}
	if !qualitySet && movieProfile.Quality == "balanced" {
		movieProfile.Quality = "script"
	}
	if !containerSet && profile.Container == "keep" {
		movieProfile.Container = "mkv"
	}
	type job struct {
		input, output, muxer string
		source               os.FileInfo
		profile              media.Conversion
	}
	queue := make([]job, 0, len(files))
	seen := make(map[string]bool)
	var estimate uint64
	for _, input := range files {
		if !strings.HasPrefix(input, "input"+string(filepath.Separator)) {
			return fmt.Errorf("transcode sources must be under %s", filepath.Join(media.Working, "input"))
		}
		info, err := root.Lstat(input)
		if err != nil {
			return err
		}
		estimate += uint64(info.Size())
		selected := movieProfile
		if series || (fromDownload && !downloads[input].movie) {
			selected = seriesProfile
		}
		output, muxer := selected.Target(input)
		directory := downloads[input].directory
		if directory == "" && series {
			stem := strings.TrimSuffix(input, filepath.Ext(input))
			if match := downloadedEpisodePath.FindStringSubmatch(filepath.ToSlash(stem)); match != nil && filepath.IsLocal(match[1]) && match[1] != "." {
				directory = filepath.Join(match[1], "Season "+match[2])
			}
		}
		if directory != "" {
			output = filepath.Join("output", directory, filepath.Base(output))
		}
		output, err = media.SelectTarget(root, output, input, seen)
		if err != nil {
			return err
		}
		seen[output] = true
		queue = append(queue, job{input, output, muxer, info, selected})
	}
	available, err := media.DestinationSpace(root, "output", false)
	if err != nil {
		return err
	}
	report("Estimated source size %d bytes, %d bytes available", estimate, available)
	if dryRun {
		report("Would consider %d files for transcoding", len(queue))
		return nil
	}
	if estimate > available && !yes {
		return errors.New("estimated output exceeds available space; use --yes to proceed")
	}
	for _, selected := range []media.Conversion{movieProfile, seriesProfile} {
		if err := selected.Validate(); err != nil {
			return err
		}
	}
	ctx, releaseCPUCap, err := media.WithCPUCap(ctx, parallel)
	if err != nil {
		return err
	}
	defer releaseCPUCap()
	encoders, err := profile.EncoderInventory(ctx)
	if err != nil {
		return err
	}
	var mu sync.Mutex
	progress.setTotal(len(queue))
	var failures, postFailures []error
	var done, skipped, cancelled, retries int
	workers := make(chan struct{}, min(parallel, len(queue)))
	var wait sync.WaitGroup
	for index, item := range queue {
		if ctx.Err() != nil {
			mu.Lock()
			cancelled += len(queue) - index
			mu.Unlock()
			break
		}
		workers <- struct{}{}
		wait.Go(func() {
			defer func() { <-workers }()
			progress.status(fmt.Sprintf("Processing media item %d", index+1))
			_, finish := progress.begin(ctx, "")
			item.profile.Threads = 1
			encoder, skip, attempts, err := item.profile.Convert(ctx, root, item.input, item.output, item.muxer, encoders)
			var companionErr, deleteErr error
			var companion string
			target := item.output
			if err == nil && skip && fromDownload && parent != "" {
				if info, statErr := root.Lstat(item.output); statErr != nil || !info.Mode().IsRegular() {
					target = item.input
				}
				companion, companionErr = media.WriteSTRMIn(ctx, root, target, item.input, parent, downloads[item.input].directory, titles, false)
			}
			if err == nil && !skip {
				if parent != "" {
					companion, companionErr = media.WriteSTRMIn(ctx, root, item.output, item.input, parent, downloads[item.input].directory, titles, false)
				}
				if remove && companionErr == nil && ctx.Err() == nil {
					current, err := root.Lstat(item.input)
					if err != nil {
						deleteErr = err
					} else if !os.SameFile(current, item.source) || current.Size() != item.source.Size() || !current.ModTime().Equal(item.source.ModTime()) {
						deleteErr = errors.New("source changed during conversion")
					} else {
						deleteErr = root.Remove(item.input)
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if fromDownload && err == nil && companionErr == nil {
				reportDownloadPaths(report, target, companion, parent)
				if finishItem != nil && ctx.Err() == nil && deleteErr == nil {
					if err := finishItem(item.input); err != nil {
						postFailures = append(postFailures, fmt.Errorf("update download queue: %w", err))
					}
				}
			}
			retries += attempts
			switch {
			case err != nil && ctx.Err() != nil:
				cancelled++
				finish("cancelled")
			case err != nil:
				failures = append(failures, fmt.Errorf("%s: %w", item.input, err))
				finish("failed")
			case skip:
				if companionErr != nil {
					postFailures = append(postFailures, fmt.Errorf("%s companion: %w", item.input, companionErr))
				}
				skipped++
				finish("skipped")
			default:
				done++
				finish("done")
				report("Transcoded media item %d with %s", index+1, encoder)
				if companionErr != nil {
					postFailures = append(postFailures, fmt.Errorf("%s companion: %w", item.input, companionErr))
				}
				if deleteErr != nil {
					postFailures = append(postFailures, fmt.Errorf("%s source removal: %w", item.input, deleteErr))
				}
			}
		})
	}
	wait.Wait()
	report("Transcoded %d files, %d skipped, %d failed, %d cancelled, %d retries, %d post-processing failures", done, skipped, len(failures), cancelled, retries, len(postFailures))
	return errors.Join(ctx.Err(), errors.Join(failures...), errors.Join(postFailures...))
}
