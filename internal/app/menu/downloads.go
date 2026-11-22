// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package menu

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
	// DownloadQueue is the page for reviewing and running queued downloads.
	DownloadQueue = "Download queue"
)

type seriesKey struct{ provider, id string }

// Browser tracks provider navigation and queued movies and episodes.
type Browser struct {
	downloadPath func(string, xtream.Episode) string
	groups       []xtream.SearchGroup
	movieGroups  []xtream.MovieGroup
	provider     string
	episodeFrom  string
	listing      Discovery
	selected     map[seriesKey]map[string]string
	movies       map[seriesKey]xtream.Movie
	redownload   bool
}

// Choices returns the entries and navigation actions for a page.
func (s *Browser) Choices(page string) []Choice {
	choices := s.choices(page)
	if page == "Main" || page == SessionSettings {
		return choices
	}
	return append(slices.Clone(choices),
		Choice{Label: "Back to Previous", Page: s.Previous(page)},
		Choice{Label: "Back to Main", Page: "Main"},
		Choice{Label: "Exit"},
	)
}

func (s *Browser) choices(page string) []Choice {
	switch page {
	case seriesProviders:
		var choices []Choice
		for _, group := range s.groups {
			if len(group.Series) > 0 {
				choices = append(choices, providerChoice(group.Provider, len(group.Series), matchingSeries))
			}
		}
		return append(choices, Choice{Label: "Download queue", Page: DownloadQueue})
	case movieProviders:
		var choices []Choice
		for _, group := range s.movieGroups {
			if len(group.Movies) > 0 {
				choices = append(choices, providerChoice(group.Provider, len(group.Movies), matchingMovies))
			}
		}
		return append(choices, Choice{Label: "Download queue", Page: DownloadQueue})
	case matchingMovies:
		var choices []Choice
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
				choices = append(choices, Choice{Label: marker + MovieLabel(movie), Mark: "movie", key: key, movie: movie})
			}
		}
		return append(choices, Choice{Label: "Download queue", Page: DownloadQueue})
	case matchingSeries:
		var choices []Choice
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
				choices = append(choices, Choice{Label: label, Args: []string{"series", "episodes", "--provider=" + group.Provider, item.ID}})
			}
		}
		return append(choices, Choice{Label: "Download queue", Page: DownloadQueue})
	case matchingEpisodes:
		choices := []Choice{
			{Label: "Add All Episodes to Download Queue", Mark: "all"},
			{Label: "Deselect All Episodes", Mark: "none"},
			{Label: "Download queue", Page: DownloadQueue},
		}
		key := seriesKey{s.listing.Provider, s.listing.SeriesID}
		lastSeason := -1
		root, err := os.OpenRoot(media.Working)
		if err == nil {
			defer root.Close()
		}
		for _, episode := range s.listing.Episodes {
			if episode.Season != lastSeason {
				choices = append(choices, Choice{Label: fmt.Sprintf("Toggle all in Season %d", episode.Season), Mark: "season", season: episode.Season})
				lastSeason = episode.Season
			}
			label := fmt.Sprintf("S%02dE%02d · %s", episode.Season, episode.Number, episode.ID)
			if root != nil {
				if info, err := root.Lstat(s.downloadPath(key.provider, episode)); err == nil && info.Mode().IsRegular() {
					label += " · downloaded"
				}
			}
			if s.selected[key][episode.ID] != "" {
				label = "[x] " + label
			} else {
				label = "[ ] " + label
			}
			choices = append(choices, Choice{Label: label, Mark: "episode", episode: episode.ID})
		}
		return choices
	case DownloadQueue:
		selected := len(s.movies)
		for _, ids := range s.selected {
			selected += len(ids)
		}
		state := "off"
		if s.redownload {
			state = "on"
		}
		choices := []Choice{
			{Label: fmt.Sprintf("Download Only (%d selected)", selected), Mark: "download", Confirm: true},
			{Label: fmt.Sprintf("Download and Process (%d selected)", selected), Mark: "process", Confirm: true},
			{Label: "Replace existing downloads (" + state + ")", Mark: "replace"},
			{Label: "Clear selections", Mark: "clear"},
		}
		var queued []Choice
		for key, movie := range s.movies {
			label := fmt.Sprintf("Remove %s [%s · %s]", MovieLabel(movie), key.provider, key.id)
			queued = append(queued, Choice{Label: label, Mark: "remove-movie", key: key})
		}
		for key, ids := range s.selected {
			for id, title := range ids {
				label := fmt.Sprintf("Remove %s [%s · %s/%s]", title, key.provider, key.id, id)
				queued = append(queued, Choice{Label: label, Mark: "remove-episode", key: key, episode: id})
			}
		}
		slices.SortFunc(queued, func(a, b Choice) int {
			return cmp.Compare(a.Label, b.Label)
		})
		return append(choices, queued...)
	default:
		return static[page]
	}
}

func providerChoice(provider string, count int, page string) Choice {
	label := fmt.Sprintf("%s (%d matches)", provider, count)
	if count == 1 {
		label = provider + " (1 match)"
	}
	return Choice{Label: label, Page: page, provider: provider}
}

// Mark applies a queue action and returns command arguments if it starts a batch.
func (s *Browser) Mark(choice Choice) []string {
	switch choice.Mark {
	case "download", "process":
		return s.arguments(choice.Mark == "process")
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
	key := seriesKey{s.listing.Provider, s.listing.SeriesID}
	ids := s.selected[key]
	if ids == nil {
		ids = make(map[string]string)
		s.selected[key] = ids
	}
	queueEpisode := func(item xtream.Episode) {
		ids[item.ID] = fmt.Sprintf("%s · S%02dE%02d", s.listing.Title, item.Season, item.Number)
	}
	switch choice.Mark {
	case "all":
		for _, item := range s.listing.Episodes {
			queueEpisode(item)
		}
	case "none":
		clear(ids)
	case "episode":
		if ids[choice.episode] != "" {
			delete(ids, choice.episode)
		} else {
			for _, item := range s.listing.Episodes {
				if item.ID == choice.episode {
					queueEpisode(item)
					break
				}
			}
		}
	case "season":
		all := true
		for _, item := range s.listing.Episodes {
			if item.Season == choice.season && ids[item.ID] == "" {
				all = false
				break
			}
		}
		for _, item := range s.listing.Episodes {
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

func (s *Browser) arguments(process bool) []string {
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

// Browse stores the latest results and returns the page that displays them.
func (s *Browser) Browse(discovery Discovery, from string) string {
	if len(discovery.Episodes) != 0 {
		s.listing = discovery
		s.episodeFrom = from
		return matchingEpisodes
	}
	for _, group := range discovery.Groups {
		if len(group.Series) > 0 {
			s.groups = discovery.Groups
			return seriesProviders
		}
	}
	for _, group := range discovery.Movies {
		if len(group.Movies) > 0 {
			s.movieGroups = discovery.Movies
			return movieProviders
		}
	}
	return ""
}

// Heading returns the title displayed above a page.
func (s *Browser) Heading(page string) string {
	if page == matchingEpisodes {
		return s.listing.Title + " [" + s.listing.Provider + "]"
	}
	if page == matchingSeries || page == matchingMovies {
		return page + " [" + s.provider + "]"
	}
	return page
}

// Previous returns the parent page for back navigation.
func (s *Browser) Previous(page string) string {
	switch page {
	case matchingEpisodes:
		return s.episodeFrom
	case matchingSeries:
		return seriesProviders
	case matchingMovies:
		return movieProviders
	case seriesProviders, movieProviders, DownloadQueue:
		return "Downloads"
	default:
		return "Main"
	}
}

// NewBrowser creates the state for browsing and queueing downloads.
func NewBrowser(downloadPath func(string, xtream.Episode) string) *Browser {
	return &Browser{
		downloadPath: downloadPath,
		selected:     make(map[seriesKey]map[string]string),
		movies:       make(map[seriesKey]xtream.Movie),
	}
}

// Open records the provider selected for a results page.
func (s *Browser) Open(choice Choice) {
	if choice.provider != "" {
		s.provider = choice.provider
	}
}
