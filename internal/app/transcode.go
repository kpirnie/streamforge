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
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/kpirnie/streamforge/internal/media"
)

func runTranscode(ctx context.Context, args []string, seriesDirectories map[string]string, titles *media.Titles, value func(string, string) string, dryRun, yes bool, report func(string, ...any), progress *operationProgress, finishItem func(string) error) error {
	profile := media.Conversion{
		FFmpeg:     value("STREAMFORGE_FFMPEG", "/usr/local/bin/ffmpeg"),
		FFprobe:    value("STREAMFORGE_FFPROBE", "/usr/local/bin/ffprobe"),
		Codec:      value("STREAMFORGE_CODEC", "keep"),
		Container:  value("STREAMFORGE_CONTAINER", "keep"),
		Quality:    value("STREAMFORGE_QUALITY", "balanced"),
		Hardware:   value("STREAMFORGE_HARDWARE", "auto"),
		Resolution: value("STREAMFORGE_RESOLUTION", ""),
		FrameRate:  value("STREAMFORGE_FRAME_RATE", ""),
		Aspect:     value("STREAMFORGE_ASPECT", ""),
	}
	parent := value("STREAMFORGE_PLAYBACK_PARENT", "")
	exclude := value("STREAMFORGE_EXCLUDE", "")
	jobs := value("STREAMFORGE_JOBS", "")
	var target string
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
			profile.Codec = option
		case "--container":
			profile.Container = option
		case "--quality":
			profile.Quality = option
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
				profile.ConvertHDR = enabled
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
	if target == "" && len(seriesDirectories) == 0 {
		return errors.New("transcode requires a media path")
	}
	if parent != "" && (!filepath.IsAbs(parent) || strings.ContainsAny(parent, "\r\n")) {
		return errors.New("playback parent must be an absolute path")
	}
	if remove && !dryRun && !yes {
		return errors.New("deleting originals requires --yes")
	}
	capacity := max(runtime.GOMAXPROCS(0), 1)
	parallel := max(1, capacity/2)
	budget := parallel
	if jobs != "" {
		count, err := strconv.Atoi(jobs)
		if err != nil || count < 1 {
			return errors.New("transcode jobs must be a positive integer")
		}
		parallel = min(count, capacity)
		budget = capacity
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
	fromDownload := len(seriesDirectories) > 0
	requested := slices.Sorted(maps.Keys(seriesDirectories))
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
	type job struct {
		input, output, muxer string
		source               os.FileInfo
	}
	queue := make([]job, 0, len(files))
	seen := make(map[string]bool)
	var estimate uint64
	for _, input := range files {
		if !strings.HasPrefix(input, "input/") {
			return errors.New("transcode sources must be under /working/input")
		}
		info, err := root.Lstat(input)
		if err != nil {
			return err
		}
		estimate += uint64(info.Size())
		output, muxer := profile.Target(input)
		if directory := seriesDirectories[input]; directory != "" {
			output = filepath.Join("output", directory, filepath.Base(output))
		}
		output, err = media.SelectTarget(root, output, input, seen)
		if err != nil {
			return err
		}
		seen[output] = true
		queue = append(queue, job{input, output, muxer, info})
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
	if err := profile.Validate(); err != nil {
		return err
	}
	encoders, err := profile.EncoderInventory(ctx)
	if err != nil {
		return err
	}
	preferred := profile.PreferredEncoder(encoders)
	if preferred != "" {
		cap := 1
		if jobs != "" {
			cap = 2
		}
		parallel = min(parallel, cap)
	} else if profile.Hardware != "auto" && profile.Hardware != "cpu" && profile.Codec != "keep" {
		report("Requested hardware encoder unavailable; using CPU")
	}
	profile.Threads = max(1, budget/parallel)
	var mu sync.Mutex
	progress.setTotal(len(queue))
	var failures, postFailures []error
	var done, skipped, cancelled, retries int
	workers := make(chan struct{}, parallel)
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
			encoder, skip, attempts, err := profile.Convert(ctx, root, item.input, item.output, item.muxer, encoders)
			var companionErr, deleteErr error
			var companion string
			target := item.output
			if err == nil && skip && fromDownload && parent != "" {
				if info, statErr := root.Lstat(item.output); statErr != nil || !info.Mode().IsRegular() {
					target = item.input
				}
				companion, companionErr = media.WriteSTRMIn(ctx, root, target, item.input, parent, seriesDirectories[item.input], titles, false)
			}
			if err == nil && !skip {
				if parent != "" {
					companion, companionErr = media.WriteSTRMIn(ctx, root, item.output, item.input, parent, seriesDirectories[item.input], titles, false)
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
