// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/kpirnie/streamforge/internal/media"
	"github.com/kpirnie/streamforge/internal/xtream"
)

func runDownload(ctx context.Context, p *xtream.Providers, command seriesCommand, parent string, titles *media.Titles, value func(string, string) string, dryRun, yes bool, report func(string, ...any)) error {
	if !filepath.IsAbs(parent) || strings.ContainsAny(parent, "\r\n") {
		return errors.New("download requires an absolute STREAMFORGE_PLAYBACK_PARENT")
	}
	_, episodes, invalid, err := p.Episodes(ctx, command.provider, command.argument)
	if err != nil {
		return err
	}
	wanted := make(map[string]bool, len(command.episodes))
	for _, id := range command.episodes {
		wanted[id] = true
	}
	var selected []xtream.Episode
	names := make(map[string]bool)
	for _, episode := range episodes {
		if !command.allEpisodes && !wanted[episode.ID] {
			continue
		}
		delete(wanted, episode.ID)
		name := episode.Name + "." + episode.Extension
		if names[name] {
			return errors.New("selected episodes have conflicting legacy filenames")
		}
		names[name] = true
		selected = append(selected, episode)
	}
	if len(wanted) != 0 || len(selected) == 0 {
		return errors.New("selected episodes were not found in provider metadata")
	}
	root, err := os.OpenRoot(media.Working)
	if err != nil {
		return err
	}
	defer root.Close()

	var stats syscall.Statfs_t
	destination := filepath.Join(media.Working, "input")
	if err := syscall.Statfs(destination, &stats); err != nil {
		return errors.New("could not check space in /working/input")
	}
	if stats.Bavail == 0 && !yes {
		return errors.New("no available space in /working/input; use --yes to attempt the download")
	}
	report("Space available in input: %d bytes", stats.Bavail*uint64(stats.Bsize))
	if dryRun {
		report("Would consider %d episodes for download, %d invalid metadata entries", len(selected), invalid)
		return nil
	}
	ffprobe := value("STREAMFORGE_FFPROBE", "/usr/local/bin/ffprobe")
	probe, err := os.Stat(ffprobe)
	if err != nil || !probe.Mode().IsRegular() || probe.Mode().Perm()&0111 == 0 {
		return errors.New("host FFprobe executable is required")
	}
	if command.redownload && !yes {
		return errors.New("replacing existing downloads requires --yes")
	}
	providerKey := sha256.Sum256([]byte(command.provider))
	folder := filepath.Join("input", fmt.Sprintf("provider-%x", providerKey[:8]), command.argument)
	var mu sync.Mutex
	var failures, companionFailures []error
	var done, skipped int
	var completed []string
	queueErr := p.DownloadBatch(ctx, command.provider, selected, func(episode xtream.Episode, transfer func(*os.File, func() error) error) {
		path := filepath.Join(folder, episode.Name+"."+episode.Extension)
		result := media.AtomicWrite(ctx, root, path, 0666, command.redownload, func(file *os.File) error {
			return transfer(file, func() error {
				_, err := media.Probe(ctx, ffprobe, file)
				return err
			})
		})
		var companionErr error
		if result == nil && !command.process {
			_, companionErr = media.WriteSTRM(ctx, root, path, parent, titles, false)
		}
		mu.Lock()
		defer mu.Unlock()
		switch {
		case errors.Is(result, os.ErrExist) && !command.redownload:
			skipped++
		case result != nil:
			failures = append(failures, fmt.Errorf("episode %s: %w", episode.ID, result))
		default:
			done++
			if command.process {
				completed = append(completed, path)
			}
			if companionErr != nil {
				companionFailures = append(companionFailures, fmt.Errorf("episode %s companion: %w", episode.ID, companionErr))
			}
		}
	})
	report("Downloaded %d episodes, %d skipped, %d failed, %d companion failures", done, skipped, len(failures), len(companionFailures))
	var processErr error
	if command.process && len(completed) > 0 && ctx.Err() == nil {
		processErr = runTranscode(ctx, nil, completed, titles, value, false, yes, report)
	}
	return errors.Join(queueErr, ctx.Err(), errors.Join(failures...), errors.Join(companionFailures...), processErr)
}
