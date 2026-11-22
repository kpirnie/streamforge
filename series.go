// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kpirnie/streamforge/internal/media"
	"github.com/kpirnie/streamforge/internal/xtream"
)

type cachedProviders struct {
	titles    *media.Titles
	providers *xtream.Providers
	err       error
}
type seriesDiscovery struct {
	groups   []xtream.SearchGroup
	episodes []xtream.Episode
	provider string
	title    string
	seriesID string
}

type seriesSelection struct {
	provider    string
	argument    string
	episodes    []string
	allEpisodes bool
}

type seriesCommand struct {
	seriesSelection
	action     string
	redownload bool
	process    bool
	selections []seriesSelection
}

func parseSeries(args []string) (seriesCommand, error) {
	var command seriesCommand
	if len(args) == 0 {
		return command, errors.New("select series search, episodes, or download")
	}
	command.action = args[0]
	var positionals []string
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--provider":
			if i+1 >= len(args) || command.provider != "" {
				return command, errors.New("--provider requires one provider name")
			}
			i++
			command.provider = args[i]
		case strings.HasPrefix(arg, "--provider="):
			if command.provider != "" {
				return command, errors.New("specify a single provider")
			}
			command.provider = strings.TrimPrefix(arg, "--provider=")
		case strings.HasPrefix(arg, "--select="):
			pick, err := parseSelection(strings.TrimPrefix(arg, "--select="))
			if err != nil {
				return command, err
			}
			command.selections = append(command.selections, pick)
		case arg == "--all-episodes":
			command.allEpisodes = true
		case arg == "--process":
			command.process = true
		case arg == "--redownload":
			command.redownload = true
		case strings.HasPrefix(arg, "--episode="):
			command.episodes = append(command.episodes, strings.TrimPrefix(arg, "--episode="))
		case strings.HasPrefix(arg, "--"):
			return command, errors.New("unknown series option")
		default:
			positionals = append(positionals, arg)
		}
	}
	if command.action != "download" && (command.allEpisodes || command.redownload || command.process || len(command.episodes) > 0 || len(command.selections) > 0) {
		return command, errors.New("episode selection flags apply to series download")
	}
	switch command.action {
	case "search":
		command.argument = strings.TrimSpace(strings.Join(positionals, " "))
		if command.argument == "" {
			return command, errors.New("series search requires a title")
		}
	case "download":
		if len(command.selections) > 0 {
			if command.provider != "" || len(positionals) != 0 || command.allEpisodes || len(command.episodes) > 0 {
				return command, errors.New("use --select entries or a single provider selection")
			}
			return command, nil
		}
		if command.provider == "" || len(positionals) != 1 || command.allEpisodes == (len(command.episodes) > 0) {
			return command, errors.New("series download requires a provider, a series ID, and --all-episodes or --episode=ID")
		}
		command.argument = positionals[0]
		command.selections = []seriesSelection{command.seriesSelection}
	case "episodes":
		if command.provider == "" || len(positionals) != 1 {
			return command, errors.New("series episodes requires --provider and a series ID")
		}
		command.argument = positionals[0]
	default:
		return command, errors.New("select series search, episodes, or download")
	}
	return command, nil
}

func parseSelection(raw string) (seriesSelection, error) {
	parts := strings.Split(raw, "|")
	if len(parts) != 3 {
		return seriesSelection{}, errors.New("selection requires Provider|Series ID|Episode IDs or all")
	}
	selection := seriesSelection{provider: strings.TrimSpace(parts[0]), argument: strings.TrimSpace(parts[1]), allEpisodes: strings.EqualFold(strings.TrimSpace(parts[2]), "all")}
	if selection.provider == "" || selection.argument == "" || strings.TrimSpace(parts[2]) == "" {
		return seriesSelection{}, errors.New("selection contains a missing value")
	}
	if !selection.allEpisodes {
		for _, id := range strings.Split(parts[2], ",") {
			selection.episodes = append(selection.episodes, strings.TrimSpace(id))
		}
	}
	return selection, nil
}

func runSeries(ctx context.Context, providers *xtream.Providers, command seriesCommand, out io.Writer, warn func(string, ...any), discovery *seriesDiscovery) error {
	if command.action == "episodes" {
		title, episodes, skipped, err := providers.Episodes(ctx, command.provider, command.argument)
		if err != nil {
			return err
		}
		if discovery != nil {
			*discovery = seriesDiscovery{episodes: episodes, provider: command.provider, title: title, seriesID: command.argument}
		}
		fmt.Fprintf(out, "%s [%s]\n", title, command.provider)
		for _, episode := range episodes {
			fmt.Fprintf(out, "  %s\t%s.%s\n", episode.ID, episode.Name, episode.Extension)
		}
		if len(episodes) == 0 {
			fmt.Fprintln(out, "  No episodes found")
		}
		if skipped > 0 {
			warn("Skipped %d episodes with incomplete or invalid metadata", skipped)
		}
		return nil
	}
	groups, err := providers.Search(ctx, command.argument, command.provider)
	if err != nil {
		return err
	}
	if discovery != nil {
		discovery.groups = groups
	}
	failed := 0
	for _, group := range groups {
		if group.Err != nil {
			warn("Xtream provider %s: %s", group.Provider, group.Err)
			failed++
			continue
		}
		fmt.Fprintln(out, group.Provider)
		for _, item := range group.Series {
			fmt.Fprintf(out, "  %s\t%s", item.ID, item.Title)
			if item.Year != "" {
				fmt.Fprintf(out, " (%s)", item.Year)
			}
			if item.Seasons > 0 {
				fmt.Fprintf(out, " · %d seasons", item.Seasons)
			}
			fmt.Fprintln(out)
		}
		if len(group.Series) == 0 {
			fmt.Fprintln(out, "  No matches")
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if failed > 0 {
		return fmt.Errorf("series search failed for %d of %d providers", failed, len(groups))
	}
	return nil
}
