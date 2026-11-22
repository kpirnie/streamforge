// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package menu

import (
	"errors"
	"slices"
	"strings"

	"github.com/kpirnie/streamforge/internal/xtream"
)

// Choice represents a menu action or a link to another page.
type Choice struct {
	Label    string
	Args     []string
	Page     string
	Prompt   string
	Confirm  bool
	Mark     string
	season   int
	episode  string
	provider string
	key      seriesKey
	movie    xtream.Movie
	Setting  string
	Secret   bool
}

var static = map[string][]Choice{
	"Main": {
		{Label: "Dispatcharr", Page: "Dispatcharr"},
		{Label: "Emby", Page: "Emby"},
		{Label: "Downloads", Page: "Downloads"},
		{Label: "Transcode media", Args: []string{"movies", "transcode"}, Prompt: "Media path", Confirm: true},
		{Label: "Generate .strm companions", Args: []string{"strm"}, Prompt: "Media path", Confirm: true},
		{Label: "System Status", Page: "System Status"},
		{Label: "Exit"},
	},
	"Dispatcharr": {
		{Label: "Refresh", Args: []string{"dispatcharr", "refresh"}, Confirm: true},
		{Label: "Create/Update", Args: []string{"dispatcharr", "create-update"}, Confirm: true},
		{Label: "Prune", Args: []string{"dispatcharr", "prune"}, Confirm: true},
		{Label: "Match", Args: []string{"dispatcharr", "match"}, Confirm: true},
		{Label: "Renumber", Args: []string{"dispatcharr", "renumber"}, Confirm: true},
		{Label: "Reorder", Args: []string{"dispatcharr", "reorder"}, Confirm: true},
		{Label: "Clear EPG Mappings", Args: []string{"dispatcharr", "clear-epg"}, Confirm: true},
		{Label: "Export", Args: []string{"dispatcharr", "export"}},
		{Label: "All", Args: []string{"dispatcharr", "all"}, Confirm: true},
	},
	"Emby": {
		{Label: "Refresh Guide", Args: []string{"emby", "refresh"}, Confirm: true},
		{Label: "Logos", Args: []string{"emby", "logos"}, Confirm: true},
		{Label: "Numbers", Args: []string{"emby", "numbers"}, Confirm: true},
		{Label: "All", Args: []string{"emby", "all"}, Confirm: true},
	},
	"Downloads": {
		{Label: "Search series", Args: []string{"series", "search"}, Prompt: "Title"},
		{Label: "Search movies", Args: []string{"movies", "search"}, Prompt: "Title"},
		{Label: "Download queue", Page: DownloadQueue},
		{Label: "List episodes", Args: []string{"series", "episodes"}, Prompt: "Provider name | Series ID"},
		{Label: "Download series", Args: []string{"series", "download"}, Prompt: "Provider name | Series ID | Episode IDs (comma separated or all)", Confirm: true},
		{Label: "Download and process series", Args: []string{"series", "download", "--process"}, Prompt: "Provider name | Series ID | Episode IDs (comma separated or all)", Confirm: true},
		{Label: "Transcode media", Args: []string{"movies", "transcode"}, Prompt: "Media path", Confirm: true},
	},
	"System Status": {
		{Label: "Show status", Args: []string{"diagnostics"}},
		{Label: "Retry Connections", Args: []string{"diagnostics"}, Page: ""},
		{Label: "Save diagnostics", Args: []string{"diagnostics", "--save"}},
		{Label: "Copy Diagnostics", Args: []string{"diagnostics"}},
	},
}

// Arguments builds command arguments from a choice and its prompt input.
func Arguments(choice Choice, input string) ([]string, error) {
	args := slices.Clone(choice.Args)
	if choice.Prompt == "" {
		return args, nil
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, errors.New("an input value is required")
	}
	if len(args) < 2 || args[0] != "series" || (args[1] != "episodes" && args[1] != "download") {
		return append(args, input), nil
	}
	values := strings.Split(input, "|")
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}
	size := 2
	if args[1] == "download" {
		size = 3
	}
	if len(values) != size || values[0] == "" || values[1] == "" {
		return nil, errors.New("enter the requested values separated by |")
	}
	args = append(args, "--provider="+values[0], values[1])
	if size == 3 {
		if strings.EqualFold(values[2], "all") {
			args = append(args, "--all-episodes")
		} else {
			for _, id := range strings.Split(values[2], ",") {
				args = append(args, "--episode="+strings.TrimSpace(id))
			}
		}
	}
	return args, nil
}

// Discovery holds the search results or episodes returned by a command.
type Discovery struct {
	Movies   []xtream.MovieGroup
	Groups   []xtream.SearchGroup
	Episodes []xtream.Episode
	Provider string
	Title    string
	SeriesID string
}

// MovieLabel formats the title and available release information.
func MovieLabel(movie xtream.Movie) string {
	title := movie.Title
	if movie.Year != "" && !strings.Contains(title, "("+movie.Year+")") {
		title += " (" + movie.Year + ")"
	}
	if movie.Quality != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(movie.Quality)) {
		title += " · " + movie.Quality
	}
	return title + " · " + strings.ToUpper(movie.Extension)
}
