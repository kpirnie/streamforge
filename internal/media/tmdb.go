// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var seriesFilename = regexp.MustCompile(`(?i)^(.+?)[ ._-]+(s\d+e\d+|\d+x\d+)(.*)$`)
var movieFilename = regexp.MustCompile(`^(.+?)[ ._(-]+((?:19|20)\d\d)(?:[ ._)\]-]|$)`)

// Titles resolves companion names through TMDB for a single session.
type Titles struct {
	client *http.Client
	key    string
	agent  string
	report func(string, ...any)
	mu     sync.Mutex
	cache  map[string]string
	next   time.Time
}

// NewTitles creates a session-only look-up cache if the operator supplied a key.
func NewTitles(key, agent string, timeout time.Duration, transport http.RoundTripper, report func(string, ...any)) *Titles {
	if key == "" {
		return nil
	}
	return &Titles{key: key, agent: agent, report: report, cache: make(map[string]string),
		client: &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (t *Titles) basename(ctx context.Context, path string, dryRun bool) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if t == nil || dryRun {
		return base
	}
	if match := seriesFilename.FindStringSubmatch(base); match != nil {
		title := t.lookup(ctx, "tv", normalizeTitle(match[1]), "")
		if title != "" {
			return title + " - " + strings.ToUpper(match[2]) + match[3]
		}
		return base
	}
	name, year := base, ""
	if match := movieFilename.FindStringSubmatch(base); match != nil {
		name, year = match[1], match[2]
	}
	if title := t.lookup(ctx, "movie", normalizeTitle(name), year); title != "" {
		return title
	}
	return base
}

func normalizeTitle(value string) string {
	return strings.Join(strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(strings.Trim(value, " -"))), " ")
}

func (t *Titles) lookup(ctx context.Context, kind, query, year string) string {
	if query == "" {
		return ""
	}
	key := kind + "/" + strings.ToLower(query) + "/" + year
	t.mu.Lock()
	if name, ok := t.cache[key]; ok {
		t.mu.Unlock()
		return name
	}
	t.mu.Unlock()
	params := url.Values{"query": {query}, "api_key": {t.key}}
	if year != "" {
		params.Set("year", year)
	}
	var name string
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		var retry bool
		retry, name, err = t.search(ctx, kind, params)
		if err == nil || !retry || ctx.Err() != nil {
			break
		}
		if attempt < 2 && !pause(ctx, time.Second<<attempt) {
			break
		}
	}
	if err != nil && ctx.Err() == nil && t.report != nil {
		t.report("TMDB lookup failed; using the media filename: %s", err)
	}
	t.mu.Lock()
	t.cache[key] = name
	t.mu.Unlock()
	return name
}

func (t *Titles) search(ctx context.Context, kind string, params url.Values) (bool, string, error) {
	t.mu.Lock()
	now := time.Now()
	if t.next.Before(now) {
		t.next = now
	}
	wait := time.Until(t.next)
	t.next = t.next.Add(250 * time.Millisecond)
	t.mu.Unlock()
	if !pause(ctx, wait) {
		return false, "", ctx.Err()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.themoviedb.org/3/search/"+kind+"?"+params.Encode(), nil)
	if err != nil {
		return false, "", errors.New("could not construct TMDB request")
	}
	req.Header.Set("User-Agent", t.agent)
	res, err := t.client.Do(req)
	if err != nil {
		return true, "", errors.New("TMDB request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		if date, parseErr := http.ParseTime(res.Header.Get("Retry-After")); parseErr == nil {
			t.mu.Lock()
			if t.next.Before(date) {
				t.next = date
			}
			t.mu.Unlock()
		}
		return res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500, "", fmt.Errorf("TMDB returned HTTP %d", res.StatusCode)
	}
	var result struct {
		Results []struct {
			Title, Name string
			ReleaseDate string `json:"release_date"`
		}
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&result); err != nil {
		return false, "", errors.New("invalid TMDB response")
	}
	if len(result.Results) == 0 {
		return false, "", nil
	}
	item := result.Results[0]
	name := item.Title
	if kind == "tv" {
		name = item.Name
	}
	name = strings.Trim(strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name), ". ")
	if name != "" && kind == "movie" && len(item.ReleaseDate) >= 4 {
		name += " (" + item.ReleaseDate[:4] + ")"
	}
	return false, name, nil
}

func pause(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
