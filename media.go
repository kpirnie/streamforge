// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kpirnie/streamforge/internal/media"
)

func runSTRM(ctx context.Context, args []string, parent, exclusions string, titles *media.Titles, dryRun bool, report func(string, ...any)) error {
	var target string
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--playback-parent="):
			parent = strings.TrimPrefix(arg, "--playback-parent=")
		case strings.HasPrefix(arg, "--exclude="):
			exclusions = strings.TrimPrefix(arg, "--exclude=")
		case strings.HasPrefix(arg, "-") || target != "":
			return errors.New("usage: streamforge strm PATH [--playback-parent=PATH] [--exclude=GLOBS]")
		default:
			target = arg
		}
	}
	if target == "" {
		return errors.New("strm requires a media file or directory")
	}
	root, err := os.OpenRoot(media.Working)
	if err != nil {
		return err
	}
	defer root.Close()
	var patterns []string
	if exclusions != "" {
		patterns = strings.Split(exclusions, ",")
	}
	files, err := media.Files(root, target, patterns)
	if err != nil {
		return err
	}
	var failures []error
	count := 0
	for _, path := range files {
		if ctx.Err() != nil {
			return errors.Join(append(failures, ctx.Err())...)
		}
		if _, err := media.WriteSTRM(ctx, root, path, "", parent, titles, dryRun); err != nil {
			failures = append(failures, fmt.Errorf("companion %d: %w", count+len(failures)+1, err))
			continue
		}
		count++
	}
	if dryRun {
		report("Would write %d companions, %d failed", count, len(failures))
	} else {
		report("Wrote %d companions, %d failed", count, len(failures))
	}
	return errors.Join(failures...)
}
