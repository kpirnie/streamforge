// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/kpirnie/streamforge/internal/app/menu"
	"github.com/kpirnie/streamforge/internal/media"
	"github.com/kpirnie/streamforge/internal/xtream"
)

func runDownload(ctx context.Context, p *xtream.Providers, command seriesCommand, parent string, titles *media.Titles, value func(string, string) string, dryRun, yes bool, report func(string, ...any), progress *operationProgress) error {
	if !media.ValidPlaybackParent(parent) {
		return errors.New("download requires an absolute STREAMFORGE_PLAYBACK_PARENT")
	}
	grouped := make(map[string][]xtream.Episode)
	paths := make(map[string]bool)
	var providerOrder []string
	var invalid int
	var estimated uint64
	unknownSize := false
	var failures []error
	add := func(provider string, episode xtream.Episode) {
		path := downloadPath(provider, episode)
		if paths[path] {
			return
		}
		paths[path] = true
		if len(grouped[provider]) == 0 {
			providerOrder = append(providerOrder, provider)
		}
		if episode.SizeBytes == 0 {
			unknownSize = true
		} else if episode.SizeBytes > ^uint64(0)-estimated {
			estimated = ^uint64(0)
		} else {
			estimated += episode.SizeBytes
		}
		grouped[provider] = append(grouped[provider], episode)
	}
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
		for _, episode := range episodes {
			if !pick.allEpisodes && !wanted[episode.ID] {
				continue
			}
			delete(wanted, episode.ID)
			add(pick.provider, episode)
		}
		if len(wanted) != 0 || len(grouped[pick.provider]) == before {
			failures = append(failures, errors.New("some selected episodes were unavailable or duplicated"))
		}
	}
	for _, provider := range slices.Sorted(maps.Keys(command.movies)) {
		wanted := maps.Clone(command.movies[provider])
		movies, err := p.Movies(ctx, provider, wanted)
		if err != nil {
			failures = append(failures, fmt.Errorf("movie selection [%s]: %w", provider, err))
			continue
		}
		for _, movie := range movies {
			delete(wanted, movie.ID)
			add(provider, movie)
		}
		if len(wanted) != 0 {
			failures = append(failures, fmt.Errorf("%d selected movies unavailable from %s", len(wanted), provider))
		}
	}
	if len(paths) == 0 {
		return errors.Join(errors.New("no media available for download"), errors.Join(failures...))
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
			return fmt.Errorf("insufficient space in %s; use --yes to attempt the download", filepath.Join(media.Working, "input"))
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
		report("Would consider %d items for download, %d invalid metadata entries", len(paths), invalid)
		return errors.Join(failures...)
	}
	ffprobe := mediaExecutable(value, "ffprobe")
	if !filepath.IsAbs(ffprobe) {
		return errors.New("ffprobe is required on PATH or at an absolute STREAMFORGE_FFPROBE path")
	}
	if !media.IsMediaExecutable(ffprobe) {
		return errors.New("host FFprobe executable is required")
	}
	if command.redownload && !yes {
		return errors.New("replacing existing downloads requires --yes")
	}
	var mu sync.Mutex
	var companionFailures []error
	var done, skipped, cancelled int
	completed := make(map[string]downloadedMedia)
	queued := make(map[string]menu.QueueItem)
	tracked := progress != nil && progress.complete != nil
	progress.setTotal(len(paths))
	transferEpisode := func(provider string, episode xtream.Episode, transfer func(context.Context, *os.File, func() error) error) {
		path := downloadPath(provider, episode)
		directory := filepath.Join(episode.SeriesName, fmt.Sprintf("Season %02d", episode.Season))
		label := fmt.Sprintf("%s · series %s · episode %s", provider, episode.SeriesID, episode.ID)
		if episode.Movie {
			directory = filepath.Join("Movies", episode.Name)
			label = fmt.Sprintf("%s · %s · movie %s", provider, episode.Name, episode.ID)
		}
		work, finish := progress.begin(ctx, label)
		result := media.AtomicWrite(work, root, path, 0666, command.redownload, func(file *os.File) error {
			return transfer(work, file, func() error {
				_, err := media.Probe(work, ffprobe, file)
				return err
			})
		})
		var companionErr error
		var companion string
		existing := errors.Is(result, os.ErrExist) && !command.redownload
		if !command.process && (result == nil || existing) && work.Err() == nil {
			companion, companionErr = media.WriteSTRMIn(work, root, path, "", parent, directory, titles, false)
		}
		mu.Lock()
		defer mu.Unlock()
		if companionErr != nil {
			companionFailures = append(companionFailures, fmt.Errorf("%s companion: %w", label, companionErr))
		}
		if companion != "" {
			reportDownloadPaths(report, path, companion, parent)
			if tracked {
				if err := progress.complete(provider, episode.SeriesID, episode.ID); err != nil {
					failures = append(failures, fmt.Errorf("update download queue: %w", err))
				}
			}
		}
		if command.process && (result == nil || (existing && tracked && work.Err() == nil)) {
			completed[path] = downloadedMedia{directory: directory, movie: episode.Movie}
			if tracked {
				queued[path] = menu.QueueItem{Provider: provider, SeriesID: episode.SeriesID, ID: episode.ID}
			}
		}
		switch {
		case result != nil && work.Err() != nil:
			cancelled++
			finish("cancelled")
		case existing:
			skipped++
			finish("skipped")
		case result != nil:
			failures = append(failures, fmt.Errorf("%s: %w", label, result))
			finish("failed")
		default:
			done++
			finish("done")
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
	report("Downloaded %d items, %d skipped, %d failed, %d cancelled, %d companion failures", done, skipped, len(failures), cancelled, len(companionFailures))
	var processErr error
	if command.process && len(completed) > 0 && ctx.Err() == nil {
		var finishItem func(string) error
		if tracked {
			finishItem = func(input string) error {
				item := queued[input]
				return progress.complete(item.Provider, item.SeriesID, item.ID)
			}
		}
		processErr = runTranscode(ctx, nil, false, completed, titles, value, false, yes, report, progress, finishItem)
	}
	return errors.Join(ctx.Err(), errors.Join(failures...), errors.Join(companionFailures...), processErr)
}

func reportDownloadPaths(report func(string, ...any), video, companion, parent string) {
	report("Video: %s", filepath.Join(media.Working, video))
	if companion != "" {
		report("Playback link: %s", filepath.Join(media.Working, companion))
		report("Player path: %s", media.PlaybackPath(parent, video))
	}
}

func downloadPath(provider string, episode xtream.Episode) string {
	key := sha256.Sum256([]byte(provider))
	directory := filepath.Join("input", fmt.Sprintf("provider-%x", key[:8]))
	if episode.Movie {
		return filepath.Join(directory, "movies", episode.ID, episode.Name+"."+episode.Extension)
	}
	return filepath.Join(directory, episode.SeriesID, episode.Name+"."+episode.Extension)
}
