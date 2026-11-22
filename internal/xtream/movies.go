// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package xtream

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
)

// Movie describes an available movie in a provider catalog.
type Movie struct {
	ID, Title, Year, Quality, Extension string
	SizeBytes                           uint64
}

// MovieGroup contains one provider's movie search results.
type MovieGroup struct {
	Provider string
	Movies   []Movie
	Err      error
}

// SearchMovies finds title matches across the selected providers.
func (p *Providers) SearchMovies(ctx context.Context, query, providerName string) ([]MovieGroup, error) {
	selected, err := p.selectProviders(providerName)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, errors.New("movie search requires a title")
	}
	groups := make([]MovieGroup, len(selected))
	var wait sync.WaitGroup
	for i, entry := range selected {
		wait.Go(func() {
			groups[i].Provider = entry.name
			catalog, err := p.movieCatalog(ctx, entry)
			groups[i].Err = err
			var exact, partial []Movie
			for _, movie := range catalog {
				title := strings.ToLower(movie.Title)
				if title == query {
					exact = append(exact, movie)
				} else if strings.Contains(title, query) {
					partial = append(partial, movie)
				}
			}
			groups[i].Movies = append(exact, partial...)
		})
	}
	wait.Wait()
	return groups, ctx.Err()
}

// Movies resolves selected IDs against the provider's current catalog.
func (p *Providers) Movies(ctx context.Context, providerName string, wanted map[string]bool) ([]Episode, error) {
	if providerName == "" {
		return nil, errors.New("movie selection requires a provider")
	}
	selected, err := p.selectProviders(providerName)
	if err != nil {
		return nil, err
	}
	catalog, err := p.movieCatalog(ctx, selected[0])
	if err != nil {
		return nil, err
	}
	var movies []Episode
	for _, movie := range catalog {
		if wanted[movie.ID] {
			name := movie.Title
			year := " (" + movie.Year + ")"
			if movie.Year != "" && !strings.Contains(name, year) {
				name += year
			}
			movies = append(movies, Episode{ID: movie.ID, Name: safeFilename(name), Extension: movie.Extension, SizeBytes: movie.SizeBytes, Movie: true})
		}
	}
	return movies, nil
}

func (p *Providers) movieCatalog(ctx context.Context, entry provider) ([]Movie, error) {
	var catalog []struct {
		ID        scalar `json:"stream_id"`
		Name      string `json:"name"`
		Year      scalar `json:"year"`
		Release   string `json:"release_date"`
		Quality   scalar `json:"quality"`
		Extension string `json:"container_extension"`
		Size      scalar `json:"size"`
	}
	if err := p.metadata(ctx, entry, "get_vod_streams", "", &catalog); err != nil {
		return nil, err
	}
	if catalog == nil {
		return nil, errors.New("invalid Xtream movie catalog")
	}
	var movies []Movie
	for _, item := range catalog {
		title := cleanDisplay(item.Name)
		extension := strings.TrimSpace(item.Extension)
		if title == "" || !validID(string(item.ID)) || !extensionPattern.MatchString(extension) {
			continue
		}
		year := yearPrefix(string(item.Year))
		if year == "" {
			year = yearPrefix(item.Release)
		}
		size, _ := strconv.ParseUint(string(item.Size), 10, 64)
		movies = append(movies, Movie{ID: string(item.ID), Title: title, Year: year, Quality: cleanDisplay(string(item.Quality)), Extension: extension, SizeBytes: size})
	}
	return movies, nil
}
