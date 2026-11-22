// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package xtream

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// Series describes a result returned by a single provider.
type Series struct {
	ID      string
	Title   string
	Year    string
	Seasons int
}

// SearchGroup contains the provider's results or its individual failure.
type SearchGroup struct {
	Provider string
	Series   []Series
	Err      error
}

// Episode identifies a downloadable provider item and its legacy output name.
type Episode struct {
	SizeBytes  uint64
	SeriesID   string
	ID         string
	SeriesName string
	Name       string
	Extension  string
	Season     int
	Number     int
}

// Search queries the selected providers concurrently and groups exact matches before partial matches.
func (p *Providers) Search(ctx context.Context, query, providerName string) ([]SearchGroup, error) {
	selected, err := p.selectProviders(providerName)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, errors.New("series search requires a title")
	}
	groups := make([]SearchGroup, len(selected))
	var wait sync.WaitGroup
	for i, entry := range selected {
		wait.Go(func() {
			groups[i].Provider = entry.name
			groups[i].Series, groups[i].Err = p.searchProvider(ctx, entry, query)
		})
	}
	wait.Wait()
	return groups, nil
}

func (p *Providers) searchProvider(ctx context.Context, entry provider, query string) ([]Series, error) {
	var catalog []struct {
		ID               scalar `json:"series_id"`
		Name             string `json:"name"`
		Year             scalar `json:"year"`
		ReleaseDate      string `json:"releaseDate"`
		ReleaseDateSnake string `json:"release_date"`
		SeasonCount      scalar `json:"season_count"`
		Seasons          scalar `json:"seasons"`
	}
	if err := p.metadata(ctx, entry, "get_series", "", &catalog); err != nil {
		return nil, err
	}
	if catalog == nil {
		return nil, errors.New("invalid Xtream series catalog")
	}
	var exact, partial []Series
	for _, item := range catalog {
		title := strings.TrimSpace(item.Name)
		folded := strings.ToLower(title)
		if !validID(string(item.ID)) || !strings.Contains(folded, query) {
			continue
		}
		year := yearPrefix(string(item.Year))
		if year == "" {
			year = yearPrefix(item.ReleaseDate)
		}
		if year == "" {
			year = yearPrefix(item.ReleaseDateSnake)
		}
		seasons := item.SeasonCount.number()
		if seasons == 0 {
			seasons = item.Seasons.number()
		}
		record := Series{ID: string(item.ID), Title: cleanDisplay(title), Year: year, Seasons: seasons}
		if folded == query {
			exact = append(exact, record)
		} else {
			partial = append(partial, record)
		}
	}
	return append(exact, partial...), nil
}

func yearPrefix(value string) string {
	if len(value) < 4 {
		return ""
	}
	year, err := strconv.Atoi(value[:4])
	if err != nil || year < 1000 {
		return ""
	}
	return value[:4]
}

func (p *Providers) metadata(ctx context.Context, entry provider, action, id string, out any) error {
	target := entry.url.JoinPath("player_api.php")
	query := url.Values{"username": {entry.settings.Username}, "password": {entry.settings.Password}, "action": {action}}
	if id != "" {
		query.Set("series_id", id)
	}
	target.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", target.String(), nil)
	if err != nil {
		return errors.New("could not construct Xtream request")
	}
	req.Header.Set("User-Agent", p.agent)
	for key, value := range entry.settings.Headers {
		req.Header.Set(key, value)
	}
	response, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return errors.New("Xtream request timed out")
		}
		return errors.New("Xtream request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Xtream returned HTTP %d", response.StatusCode)
	}
	const limit = 128 << 20
	reader := &io.LimitedReader{R: response.Body, N: limit + 1}
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(out); err != nil {
		return errors.New("could not decode Xtream metadata")
	}
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) || reader.N == 0 {
		return errors.New("invalid or oversized Xtream metadata")
	}
	return nil
}

type scalar string

func (s *scalar) UnmarshalJSON(data []byte) error {
	*s = ""
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*s = scalar(value)
		return nil
	}
	if data[0] >= '0' && data[0] <= '9' {
		*s = scalar(data)
	}
	return nil
}

func (s scalar) number() int {
	n, err := strconv.Atoi(strings.TrimSpace(string(s)))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func validID(id string) bool {
	return id != "" && strings.IndexFunc(id, func(r rune) bool { return r < '0' || r > '9' }) < 0
}

var numbering = regexp.MustCompile(`(?i)\b(?:s(?:eason\s*)?(\d+)\s*e(?:pisode\s*)?(\d+)|(\d+)x(\d+))\b`)
var extensionPattern = regexp.MustCompile(`^[a-zA-Z0-9]{1,12}$`)

// Episodes loads metadata for one explicitly selected provider and series.
func (p *Providers) Episodes(ctx context.Context, providerName, seriesID string) (string, []Episode, int, error) {
	if providerName == "" || !validID(seriesID) {
		return "", nil, 0, errors.New("episode listing requires a provider and numeric series ID")
	}
	selected, err := p.selectProviders(providerName)
	if err != nil {
		return "", nil, 0, err
	}
	var details struct {
		Info struct {
			Name string `json:"name"`
		} `json:"info"`
		Episodes json.RawMessage `json:"episodes"`
	}
	if err := p.metadata(ctx, selected[0], "get_series_info", seriesID, &details); err != nil {
		return "", nil, 0, err
	}
	title := strings.TrimSpace(details.Info.Name)
	if title == "" {
		return "", nil, 0, errors.New("Xtream series details are missing the series name")
	}
	var seasons map[string][]episodeRecord
	if len(details.Episodes) > 0 && details.Episodes[0] == '[' {
		var groups [][]episodeRecord
		if err := json.Unmarshal(details.Episodes, &groups); err != nil {
			return "", nil, 0, errors.New("invalid Xtream episode list")
		}
		seasons = make(map[string][]episodeRecord, len(groups))
		for i, group := range groups {
			seasons["array-"+strconv.Itoa(i)] = group
		}
	} else if len(details.Episodes) != 0 {
		if err := json.Unmarshal(details.Episodes, &seasons); err != nil {
			return "", nil, 0, errors.New("invalid Xtream episode list")
		}
	}
	var episodes []Episode
	skipped := 0
	filenameTitle := safeFilename(title)
	for _, key := range slices.Sorted(maps.Keys(seasons)) {
		for _, record := range seasons[key] {
			season, episode := record.numbers(scalar(key).number())
			extension := strings.TrimSpace(record.Extension)
			if extension == "" {
				extension = "mkv"
			}
			if season <= 0 || episode <= 0 || !validID(string(record.ID)) || !extensionPattern.MatchString(extension) {
				skipped++
				continue
			}
			name := fmt.Sprintf("%s - S%02dE%02d", filenameTitle, season, episode)
			size, _ := strconv.ParseUint(string(record.Size), 10, 64)
			if size == 0 {
				size, _ = strconv.ParseUint(string(record.Info.Size), 10, 64)
			}
			episodes = append(episodes, Episode{SizeBytes: size, SeriesID: seriesID, ID: string(record.ID), SeriesName: filenameTitle, Name: name, Extension: extension, Season: season, Number: episode})
		}
	}
	slices.SortStableFunc(episodes, func(a, b Episode) int {
		if order := cmp.Compare(a.Season, b.Season); order != 0 {
			return order
		}
		return cmp.Compare(a.Number, b.Number)
	})
	return cleanDisplay(title), episodes, skipped, nil
}

type episodeRecord struct {
	Size scalar `json:"filesize"`
	Info struct {
		Size scalar `json:"filesize"`
	} `json:"info"`
	ID        scalar `json:"id"`
	Season    scalar `json:"season"`
	Episode   scalar `json:"episode_num"`
	Title     string `json:"title"`
	Extension string `json:"container_extension"`
}

func (item episodeRecord) numbers(group int) (int, int) {
	season, episode := item.Season.number(), item.Episode.number()
	if season == 0 {
		season = group
	}
	if season == 0 || episode == 0 {
		match := numbering.FindStringSubmatch(item.Title)
		if match != nil {
			if season == 0 {
				season = max(scalar(match[1]).number(), scalar(match[3]).number())
			}
			if episode == 0 {
				episode = max(scalar(match[2]).number(), scalar(match[4]).number())
			}
		}
	}
	return season, episode
}

func cleanDisplay(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, value)
}

func safeFilename(value string) string {
	name := strings.Trim(strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || unicode.IsControl(r) {
			return '_'
		}
		return r
	}, cleanDisplay(value)), ". ")
	if name == "" {
		return "Series"
	}
	return name
}
