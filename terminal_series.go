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
	seriesProviders  = "Series providers"
	movieProviders   = "Movie providers"
	matchingSeries   = "Series search results"
	matchingMovies   = "Movie search results"
	matchingEpisodes = "Series episodes"
	downloadQueue    = "Download queue"
)

type seriesKey struct{ provider, id string }

type terminalSeries struct {
	groups      []xtream.SearchGroup
	movieGroups []xtream.MovieGroup
	provider    string
	episodeFrom string
	listing     seriesDiscovery
	selected    map[seriesKey]map[string]string
	movies      map[seriesKey]xtream.Movie
	redownload  bool
}

func (s *terminalSeries) choices(page string) []terminalChoice {
	switch page {
	case seriesProviders:
		var choices []terminalChoice
		for _, group := range s.groups {
			if len(group.Series) > 0 {
				choices = append(choices, providerChoice(group.Provider, len(group.Series), matchingSeries))
			}
		}
		return append(choices, terminalChoice{label: "Download queue", page: downloadQueue})
	case movieProviders:
		var choices []terminalChoice
		for _, group := range s.movieGroups {
			if len(group.Movies) > 0 {
				choices = append(choices, providerChoice(group.Provider, len(group.Movies), matchingMovies))
			}
		}
		return append(choices, terminalChoice{label: "Download queue", page: downloadQueue})
	case matchingMovies:
		var choices []terminalChoice
		for _, group := range s.movieGroups {
			if group.Provider != s.provider {
				continue
			}
			for _, movie := range group.Movies {
				key := seriesKey{group.Provider, movie.ID}
				marker := "[ ] "
				if _, queued := s.movies[key]; queued {
					marker = "[x] "
				}
				choices = append(choices, terminalChoice{label: marker + movieLabel(movie), mark: "movie", key: key, movie: movie})
			}
		}
		return append(choices, terminalChoice{label: "Download queue", page: downloadQueue})
	case matchingSeries:
		var choices []terminalChoice
		for _, group := range s.groups {
			if group.Provider != s.provider {
				continue
			}
			for _, item := range group.Series {
				label := item.Title
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
			if s.selected[key][episode.ID] != "" {
				label = "[x] " + label
			} else {
				label = "[ ] " + label
			}
			choices = append(choices, terminalChoice{label: label, mark: "episode", episode: episode.ID})
		}
		return choices
	case downloadQueue:
		selected := len(s.movies)
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
		var queued []terminalChoice
		for key, movie := range s.movies {
			label := fmt.Sprintf("Remove %s [%s · %s]", movieLabel(movie), key.provider, key.id)
			queued = append(queued, terminalChoice{label: label, mark: "remove-movie", key: key})
		}
		for key, ids := range s.selected {
			for id, title := range ids {
				label := fmt.Sprintf("Remove %s [%s · %s/%s]", title, key.provider, key.id, id)
				queued = append(queued, terminalChoice{label: label, mark: "remove-episode", key: key, episode: id})
			}
		}
		slices.SortFunc(queued, func(a, b terminalChoice) int {
			return cmp.Compare(a.label, b.label)
		})
		return append(choices, queued...)
	default:
		return terminalMenus[page]
	}
}

func providerChoice(provider string, count int, page string) terminalChoice {
	label := fmt.Sprintf("%s (%d matches)", provider, count)
	if count == 1 {
		label = provider + " (1 match)"
	}
	return terminalChoice{label: label, page: page, provider: provider}
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
		clear(s.movies)
		return nil
	case "movie":
		if _, queued := s.movies[choice.key]; queued {
			delete(s.movies, choice.key)
		} else {
			s.movies[choice.key] = choice.movie
		}
		return nil
	case "remove-movie":
		delete(s.movies, choice.key)
		return nil
	case "remove-episode":
		delete(s.selected[choice.key], choice.episode)
		return nil
	}
	key := seriesKey{s.listing.provider, s.listing.seriesID}
	ids := s.selected[key]
	if ids == nil {
		ids = make(map[string]string)
		s.selected[key] = ids
	}
	queueEpisode := func(item xtream.Episode) {
		ids[item.ID] = fmt.Sprintf("%s · S%02dE%02d", s.listing.title, item.Season, item.Number)
	}
	switch choice.mark {
	case "all":
		for _, item := range s.listing.episodes {
			queueEpisode(item)
		}
	case "none":
		clear(ids)
	case "episode":
		if ids[choice.episode] != "" {
			delete(ids, choice.episode)
		} else {
			for _, item := range s.listing.episodes {
				if item.ID == choice.episode {
					queueEpisode(item)
					break
				}
			}
		}
	case "season":
		all := true
		for _, item := range s.listing.episodes {
			if item.Season == choice.season && ids[item.ID] == "" {
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
				queueEpisode(item)
			}
		}
	}
	return nil
}

func (s *terminalSeries) arguments(process bool) []string {
	compare := func(a, b seriesKey) int {
		if order := cmp.Compare(a.provider, b.provider); order != 0 {
			return order
		}
		return cmp.Compare(a.id, b.id)
	}
	keys := slices.SortedFunc(maps.Keys(s.selected), compare)
	args := []string{"series", "download"}
	if process {
		args = append(args, "--process")
	}
	if s.redownload {
		args = append(args, "--redownload")
	}
	flags := len(args)
	for _, key := range keys {
		if len(s.selected[key]) == 0 {
			continue
		}
		ids := slices.Sorted(maps.Keys(s.selected[key]))
		args = append(args, "--select="+strings.Join([]string{key.provider, key.id, strings.Join(ids, ",")}, "|"))
	}
	for _, key := range slices.SortedFunc(maps.Keys(s.movies), compare) {
		args = append(args, "--movie="+key.provider+"|"+key.id)
	}
	if len(args) == flags {
		return nil
	}
	return args
}

func (s *terminalSeries) browse(discovery seriesDiscovery, from string) string {
	if len(discovery.episodes) != 0 {
		s.listing = discovery
		s.episodeFrom = from
		return matchingEpisodes
	}
	for _, group := range discovery.groups {
		if len(group.Series) > 0 {
			s.groups = discovery.groups
			return seriesProviders
		}
	}
	for _, group := range discovery.movies {
		if len(group.Movies) > 0 {
			s.movieGroups = discovery.movies
			return movieProviders
		}
	}
	return ""
}

func (s *terminalSeries) heading(page string) string {
	if page == matchingEpisodes {
		return s.listing.title + " [" + s.listing.provider + "]"
	}
	if page == matchingSeries || page == matchingMovies {
		return page + " [" + s.provider + "]"
	}
	return page
}

func (s *terminalSeries) previous(page string) string {
	switch page {
	case matchingEpisodes:
		return s.episodeFrom
	case matchingSeries:
		return seriesProviders
	case matchingMovies:
		return movieProviders
	case seriesProviders, movieProviders, downloadQueue:
		return "Downloads"
	default:
		return "Main"
	}
}
