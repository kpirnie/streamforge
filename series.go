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

	"github.com/kpirnie/streamforge/internal/xtream"
)

type seriesCommand struct {
	action   string
	provider string
	argument string
}

func parseSeries(args []string) (seriesCommand, error) {
	var command seriesCommand
	if len(args) == 0 {
		return command, errors.New("select series search or series episodes")
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
		case strings.HasPrefix(arg, "--"):
			return command, errors.New("unknown series option")
		default:
			positionals = append(positionals, arg)
		}
	}
	switch command.action {
	case "search":
		command.argument = strings.TrimSpace(strings.Join(positionals, " "))
		if command.argument == "" {
			return command, errors.New("series search requires a title")
		}
	case "episodes":
		if command.provider == "" || len(positionals) != 1 {
			return command, errors.New("series episodes requires --provider and a series ID")
		}
		command.argument = positionals[0]
	default:
		return command, errors.New("select series search or series episodes")
	}
	return command, nil
}

func runSeries(ctx context.Context, providers *xtream.Providers, command seriesCommand, out io.Writer, warn func(string, ...any)) error {
	if command.action == "episodes" {
		title, episodes, skipped, err := providers.Episodes(ctx, command.provider, command.argument)
		if err != nil {
			return err
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
