// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package main

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/kpirnie/streamforge/internal/media"
	"github.com/kpirnie/streamforge/internal/xtream"
)

const (
	matchingSeries   = "Series search results"
	matchingEpisodes = "Series episodes"
	downloadQueue    = "Download queue"
)

type seriesKey struct{ provider, id string }

type terminalSeries struct {
	groups     []xtream.SearchGroup
	listing    seriesDiscovery
	selected   map[seriesKey]map[string]bool
	redownload bool
}

func (s *terminalSeries) choices(page string) []terminalChoice {
	switch page {
	case matchingSeries:
		var choices []terminalChoice
		for _, group := range s.groups {
			for _, item := range group.Series {
				label := item.Title + " [" + group.Provider + "]"
				if item.Year != "" {
					label += " (" + item.Year + ")"
				}
				if item.Seasons > 0 {
					label += fmt.Sprintf(" · %d seasons", item.Seasons)
				}
				choices = append(choices, terminalChoice{label: label, args: []string{"series", "episodes", "--provider=" + group.Provider, item.ID}})
			}
		}
		return append(choices, terminalChoice{label: "Download queue", page: downloadQueue})
	case matchingEpisodes:
		choices := []terminalChoice{
			{label: "Add All Episodes to Download Queue", mark: "all"},
			{label: "Deselect All Episodes", mark: "none"},
			{label: "Download queue", page: downloadQueue},
		}
		key := seriesKey{s.listing.provider, s.listing.seriesID}
		lastSeason := -1
		root, err := os.OpenRoot(media.Working)
		if err == nil {
			defer root.Close()
		}
		for _, episode := range s.listing.episodes {
			if episode.Season != lastSeason {
				choices = append(choices, terminalChoice{label: fmt.Sprintf("Toggle all in Season %d", episode.Season), mark: "season", season: episode.Season})
				lastSeason = episode.Season
			}
			label := fmt.Sprintf("S%02dE%02d · %s", episode.Season, episode.Number, episode.ID)
			if root != nil {
				if info, err := root.Lstat(downloadPath(key.provider, episode)); err == nil && info.Mode().IsRegular() {
					label += " · downloaded"
				}
			}
			if s.selected[key][episode.ID] {
				label = "[x] " + label
			} else {
				label = "[ ] " + label
			}
			choices = append(choices, terminalChoice{label: label, mark: "episode", episode: episode.ID})
		}
		return choices
	case downloadQueue:
		selected := 0
		for _, ids := range s.selected {
			selected += len(ids)
		}
		state := "off"
		if s.redownload {
			state = "on"
		}
		choices := []terminalChoice{
			{label: fmt.Sprintf("Download Only (%d selected)", selected), mark: "download", confirm: true},
			{label: fmt.Sprintf("Download and Process (%d selected)", selected), mark: "process", confirm: true},
			{label: "Replace existing downloads (" + state + ")", mark: "replace"},
			{label: "Clear selections", mark: "clear"},
		}
		return choices
	default:
		return terminalMenus[page]
	}
}

func (s *terminalSeries) mark(choice terminalChoice) []string {
	switch choice.mark {
	case "download", "process":
		return s.arguments(choice.mark == "process")
	case "replace":
		s.redownload = !s.redownload
		return nil
	case "clear":
		clear(s.selected)
		return nil
	}
	key := seriesKey{s.listing.provider, s.listing.seriesID}
	ids := s.selected[key]
	if ids == nil {
		ids = make(map[string]bool)
		s.selected[key] = ids
	}
	switch choice.mark {
	case "all":
		for _, item := range s.listing.episodes {
			ids[item.ID] = true
		}
	case "none":
		clear(ids)
	case "episode":
		if ids[choice.episode] {
			delete(ids, choice.episode)
		} else {
			ids[choice.episode] = true
		}
	case "season":
		all := true
		for _, item := range s.listing.episodes {
			if item.Season == choice.season && !ids[item.ID] {
				all = false
				break
			}
		}
		for _, item := range s.listing.episodes {
			if item.Season != choice.season {
				continue
			}
			if all {
				delete(ids, item.ID)
			} else {
				ids[item.ID] = true
			}
		}
	}
	return nil
}

func (s *terminalSeries) arguments(process bool) []string {
	if len(s.selected) == 0 {
		return nil
	}
	keys := slices.SortedFunc(maps.Keys(s.selected), func(a, b seriesKey) int {
		if order := cmp.Compare(a.provider, b.provider); order != 0 {
			return order
		}
		return cmp.Compare(a.id, b.id)
	})
	args := []string{"series", "download"}
	if process {
		args = append(args, "--process")
	}
	if s.redownload {
		args = append(args, "--redownload")
	}
	for _, key := range keys {
		if len(s.selected[key]) == 0 {
			continue
		}
		ids := slices.Sorted(maps.Keys(s.selected[key]))
		args = append(args, "--select="+strings.Join([]string{key.provider, key.id, strings.Join(ids, ",")}, "|"))
	}
	if len(args) == 2 || !strings.HasPrefix(args[len(args)-1], "--select=") {
		return nil
	}
	return args
}

func (s *terminalSeries) browse(discovery seriesDiscovery) string {
	if len(discovery.episodes) != 0 {
		s.listing = discovery
		return matchingEpisodes
	}
	for _, group := range discovery.groups {
		if len(group.Series) > 0 {
			s.groups = discovery.groups
			return matchingSeries
		}
	}
	return ""
}

func (s *terminalSeries) heading(page string) string {
	if page == matchingEpisodes {
		return s.listing.title + " [" + s.listing.provider + "]"
	}
	return page
}

func terminalPrevious(page string) string {
	switch page {
	case matchingEpisodes:
		return matchingSeries
	case matchingSeries, downloadQueue:
		return "Series"
	default:
		return "Main"
	}
}
