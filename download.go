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
	"slices"
	"strings"
	"sync"

	"github.com/kpirnie/streamforge/internal/media"
	"github.com/kpirnie/streamforge/internal/xtream"
)

func runDownload(ctx context.Context, p *xtream.Providers, command seriesCommand, parent string, titles *media.Titles, value func(string, string) string, dryRun, yes bool, report func(string, ...any), progress *operationProgress) error {
	if !filepath.IsAbs(parent) || strings.ContainsAny(parent, "\r\n") {
		return errors.New("download requires an absolute STREAMFORGE_PLAYBACK_PARENT")
	}
	grouped := make(map[string][]xtream.Episode)
	paths := make(map[string]bool)
	var providerOrder []string
	var invalid int
	var estimated uint64
	unknownSize := false
	var failures []error
	for _, pick := range command.selections {
		_, episodes, rejected, err := p.Episodes(ctx, pick.provider, pick.argument)
		invalid += rejected
		if err != nil {
			failures = append(failures, fmt.Errorf("provider selection: %w", err))
			continue
		}
		wanted := make(map[string]bool, len(pick.episodes))
		for _, id := range pick.episodes {
			wanted[id] = true
		}
		before := len(grouped[pick.provider])
		if before == 0 && !slices.Contains(providerOrder, pick.provider) {
			providerOrder = append(providerOrder, pick.provider)
		}
		for _, episode := range episodes {
			if !pick.allEpisodes && !wanted[episode.ID] {
				continue
			}
			delete(wanted, episode.ID)
			path := downloadPath(pick.provider, episode)
			if paths[path] {
				continue
			}
			paths[path] = true
			if episode.SizeBytes == 0 {
				unknownSize = true
			} else if episode.SizeBytes > ^uint64(0)-estimated {
				estimated = ^uint64(0)
			} else {
				estimated += episode.SizeBytes
			}
			grouped[pick.provider] = append(grouped[pick.provider], episode)
		}
		if len(wanted) != 0 || len(grouped[pick.provider]) == before {
			failures = append(failures, errors.New("some selected episodes were unavailable or duplicated"))
		}
	}
	if len(paths) == 0 {
		return errors.Join(errors.New("no episodes available for download"), errors.Join(failures...))
	}
	root, err := os.OpenRoot(media.Working)
	if err != nil {
		return err
	}
	defer root.Close()

	available, err := media.DestinationSpace(root, "input", true)
	if err != nil {
		return err
	}
	if available == 0 || estimated > available {
		if !yes && !dryRun {
			return errors.New("insufficient space in /working/input; use --yes to attempt the download")
		}
	}
	if unknownSize {
		report("Input space: %d bytes; known episode sizes total %d bytes", available, estimated)
	} else {
		report("Input space: %d bytes; estimated download %d bytes", available, estimated)
	}
	if _, err := media.DestinationSpace(root, "output", false); err != nil {
		return err
	}
	if dryRun {
		report("Would consider %d episodes for download, %d invalid metadata entries", len(paths), invalid)
		return errors.Join(failures...)
	}
	ffprobe := value("STREAMFORGE_FFPROBE", "/usr/local/bin/ffprobe")
	if !filepath.IsAbs(ffprobe) {
		return errors.New("FFprobe path must be absolute")
	}
	probe, err := os.Stat(ffprobe)
	if err != nil || !probe.Mode().IsRegular() || probe.Mode().Perm()&0111 == 0 {
		return errors.New("host FFprobe executable is required")
	}
	if command.redownload && !yes {
		return errors.New("replacing existing downloads requires --yes")
	}
	var mu sync.Mutex
	var companionFailures []error
	var done, skipped, cancelled int
	completed := make(map[string]string)
	progress.setTotal(len(paths))
	transferEpisode := func(provider string, episode xtream.Episode, transfer func(context.Context, *os.File, func() error) error) {
		path := downloadPath(provider, episode)
		directory := filepath.Join(episode.SeriesName, fmt.Sprintf("Season %02d", episode.Season))
		work, finish := progress.begin(ctx, fmt.Sprintf("%s · series %s · episode %s", provider, episode.SeriesID, episode.ID))
		result := media.AtomicWrite(work, root, path, 0666, command.redownload, func(file *os.File) error {
			return transfer(work, file, func() error {
				_, err := media.Probe(work, ffprobe, file)
				return err
			})
		})
		var companionErr error
		existing := errors.Is(result, os.ErrExist) && !command.redownload
		if !command.process && (result == nil || existing) && work.Err() == nil {
			_, companionErr = media.WriteSTRMIn(work, root, path, "", parent, directory, titles, false)
		}
		mu.Lock()
		defer mu.Unlock()
		if companionErr != nil {
			companionFailures = append(companionFailures, fmt.Errorf("episode %s companion: %w", episode.ID, companionErr))
		}
		switch {
		case result != nil && work.Err() != nil:
			cancelled++
			finish("cancelled")
		case existing:
			skipped++
			finish("skipped")
		case result != nil:
			failures = append(failures, fmt.Errorf("episode %s: %w", episode.ID, result))
			finish("failed")
		default:
			done++
			finish("done")
			if command.process {
				completed[path] = directory
			}
		}
	}
	var workers sync.WaitGroup
	for _, provider := range providerOrder {
		workers.Go(func() {
			err := p.DownloadBatch(ctx, provider, grouped[provider], func(episode xtream.Episode, transfer func(context.Context, *os.File, func() error) error) {
				transferEpisode(provider, episode, transfer)
			})
			if err != nil && ctx.Err() == nil {
				mu.Lock()
				failures = append(failures, err)
				mu.Unlock()
			}
		})
	}
	workers.Wait()
	report("Downloaded %d episodes, %d skipped, %d failed, %d cancelled, %d companion failures", done, skipped, len(failures), cancelled, len(companionFailures))
	var processErr error
	if command.process && len(completed) > 0 && ctx.Err() == nil {
		processErr = runTranscode(ctx, nil, completed, titles, value, false, yes, report, progress)
	}
	return errors.Join(ctx.Err(), errors.Join(failures...), errors.Join(companionFailures...), processErr)
}

func downloadPath(provider string, episode xtream.Episode) string {
	key := sha256.Sum256([]byte(provider))
	return filepath.Join("input", fmt.Sprintf("provider-%x", key[:8]), episode.SeriesID, episode.Name+"."+episode.Extension)
}
