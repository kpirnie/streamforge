// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/kpirnie/streamforge/internal/media"
)

func runTranscode(ctx context.Context, args []string, requested []string, titles *media.Titles, value func(string, string) string, dryRun, yes bool, report func(string, ...any)) error {
	profile := media.Conversion{
		FFmpeg:    value("STREAMFORGE_FFMPEG", "/usr/local/bin/ffmpeg"),
		FFprobe:   value("STREAMFORGE_FFPROBE", "/usr/local/bin/ffprobe"),
		Codec:     value("STREAMFORGE_CODEC", "keep"),
		Container: value("STREAMFORGE_CONTAINER", "keep"),
		Quality:   value("STREAMFORGE_QUALITY", "balanced"),
		Hardware:  value("STREAMFORGE_HARDWARE", "auto"),
	}
	parent := value("STREAMFORGE_PLAYBACK_PARENT", "")
	exclude := value("STREAMFORGE_EXCLUDE", "")
	jobs := value("STREAMFORGE_JOBS", "")
	var target string
	var remove bool
	for _, arg := range args {
		key, option, _ := strings.Cut(arg, "=")
		switch key {
		case "--codec":
			profile.Codec = option
		case "--container":
			profile.Container = option
		case "--quality":
			profile.Quality = option
		case "--hardware":
			profile.Hardware = option
		case "--jobs":
			jobs = option
		case "--exclude":
			exclude = option
		case "--playback-parent":
			parent = option
		case "--convert-hdr":
			profile.ConvertHDR = true
		case "--force":
			profile.Force = true
		case "--delete-original":
			remove = true
		default:
			if strings.HasPrefix(arg, "-") || target != "" {
				return errors.New("unexpected transcode argument")
			}
			target = arg
		}
	}
	if target == "" && len(requested) == 0 {
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
	if jobs != "" {
		count, err := strconv.Atoi(jobs)
		if err != nil || count < 1 {
			return errors.New("transcode jobs must be a positive integer")
		}
		parallel = min(count, capacity)
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
	fromDownload := len(requested) > 0
	if len(requested) == 0 {
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
		output, err = media.SelectTarget(root, output, input, seen)
		if err != nil {
			return err
		}
		seen[output] = true
		queue = append(queue, job{input, output, muxer, info})
	}
	var disk syscall.Statfs_t
	if err := syscall.Statfs(media.Working, &disk); err != nil {
		return errors.New("could not check output disk space")
	}
	available := disk.Bavail * uint64(disk.Bsize)
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
	var mu sync.Mutex
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
			encoder, skip, attempts, err := profile.Convert(ctx, root, item.input, item.output, item.muxer, encoders)
			var companionErr, deleteErr error
			if err == nil && skip && fromDownload && parent != "" {
				target := item.input
				if info, statErr := root.Lstat(item.output); statErr == nil && info.Mode().IsRegular() {
					target = item.output
				}
				_, companionErr = media.WriteSTRM(ctx, root, target, parent, titles, false)
			}
			if err == nil && !skip {
				if parent != "" {
					_, companionErr = media.WriteSTRM(ctx, root, item.output, parent, titles, false)
				}
				if remove && ctx.Err() == nil {
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
			retries += attempts
			switch {
			case err != nil && ctx.Err() != nil:
				cancelled++
			case err != nil:
				failures = append(failures, fmt.Errorf("media item %d: %w", index+1, err))
			case skip:
				if companionErr != nil {
					postFailures = append(postFailures, fmt.Errorf("media item %d companion: %w", index+1, companionErr))
				}
				skipped++
			default:
				done++
				report("Transcoded media item %d with %s", index+1, encoder)
				if companionErr != nil {
					postFailures = append(postFailures, fmt.Errorf("media item %d companion: %w", index+1, companionErr))
				}
				if deleteErr != nil {
					postFailures = append(postFailures, fmt.Errorf("media item %d source removal: %w", index+1, deleteErr))
				}
			}
		})
	}
	wait.Wait()
	report("Transcoded %d files, %d skipped, %d failed, %d cancelled, %d retries, %d post-processing failures", done, skipped, len(failures), cancelled, retries, len(postFailures))
	return errors.Join(ctx.Err(), errors.Join(failures...), errors.Join(postFailures...))
}
