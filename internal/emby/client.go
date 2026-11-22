// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package emby

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Client holds the Emby API connection and a separate unauthenticated media client.
type Client struct {
	base  string
	token string
	agent string
	api   *http.Client
	media *http.Client
}

type channel struct {
	ID          string
	Name        string
	ProviderIDs map[string]string
}

// Counts summarizes completed, unmatched and failed channel operations.
type Counts struct {
	Done      int
	Unmatched int
	Failed    int
}

// Connect validates the configured Emby connection before any modifications.
func Connect(ctx context.Context, base, token, agent string, timeout time.Duration, transport http.RoundTripper) (*Client, error) {
	root, err := httpURL(base)
	if err != nil || root.RawQuery != "" || root.Fragment != "" {
		return nil, errors.New("invalid EMBY_HOST URL")
	}
	client := &Client{
		base:  strings.TrimRight(root.String(), "/") + "/emby/",
		token: token, agent: agent,
		api:   &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		media: &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: mediaRedirect},
	}
	var info map[string]json.RawMessage
	if err := client.request(ctx, "GET", "LiveTv/Info", nil, "", &info); err != nil {
		return nil, fmt.Errorf("Emby authentication: %w", err)
	}
	if info == nil {
		return nil, errors.New("invalid Emby Live TV response")
	}
	return client, nil
}

func mediaRedirect(req *http.Request, previous []*http.Request) error {
	if len(previous) >= 10 {
		return errors.New("too many media redirects")
	}
	if _, err := httpURL(req.URL.String()); err != nil {
		return err
	}
	// A playlist URL can include credentials in its path or query.
	req.Header.Del("Referer")
	return nil
}

func httpURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, errors.New("invalid HTTP URL")
	}
	return parsed, nil
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader, contentType string, output any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return errors.New("could not construct Emby request")
	}
	req.Header.Set("User-Agent", c.agent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Emby-Token", c.token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := c.api.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("Emby request failed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Emby returned HTTP %d", res.StatusCode)
	}
	if output != nil {
		if err := json.NewDecoder(res.Body).Decode(output); err != nil {
			return errors.New("could not decode Emby response")
		}
	}
	return nil
}

func (c *Client) channels(ctx context.Context) ([]channel, error) {
	var channels []channel
	for {
		var page struct {
			Items            []channel
			TotalRecordCount *int
		}
		if err := c.request(ctx, "GET", fmt.Sprintf("LiveTv/Channels?StartIndex=%d&Limit=500", len(channels)), nil, "", &page); err != nil {
			return nil, err
		}
		if page.Items == nil {
			return nil, errors.New("Emby Live TV response is missing its channel list")
		}
		channels = append(channels, page.Items...)
		if len(page.Items) == 0 || (page.TotalRecordCount != nil && len(channels) >= *page.TotalRecordCount) || (page.TotalRecordCount == nil && len(page.Items) < 500) {
			return channels, nil
		}
	}
}

func itemPath(id string) (string, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "/\\") {
		return "", errors.New("invalid Emby item ID")
	}
	return url.PathEscape(id), nil
}

// ClearNumbers clears channel numbers and their related locks across Live TV.
func (c *Client) ClearNumbers(ctx context.Context, dryRun bool) (Counts, error) {
	var counts Counts
	channels, err := c.channels(ctx)
	if err != nil {
		return counts, err
	}
	var failures []error
	for i, item := range channels {
		if ctx.Err() != nil {
			return counts, errors.Join(append(failures, ctx.Err())...)
		}
		id, err := itemPath(item.ID)
		if err == nil {
			err = c.clearNumber(ctx, id, dryRun)
		}
		if err != nil {
			counts.Failed++
			failures = append(failures, fmt.Errorf("channel %d: %w", i+1, err))
			continue
		}
		counts.Done++
	}
	return counts, errors.Join(failures...)
}

func (c *Client) clearNumber(ctx context.Context, id string, dryRun bool) error {
	var item map[string]json.RawMessage
	if err := c.request(ctx, "GET", "LiveTv/Channels/"+id, nil, "", &item); err != nil {
		return err
	}
	if item == nil {
		return errors.New("Emby returned an empty channel record")
	}
	var locks []string
	if raw := item["LockedFields"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &locks); err != nil {
			return errors.New("invalid Emby channel field locks")
		}
	}
	if dryRun {
		return nil
	}
	locks = slices.DeleteFunc(locks, func(field string) bool { return field == "ChannelNumber" || field == "SortName" })
	if locks == nil {
		locks = []string{}
	}
	encodedLocks, err := json.Marshal(locks)
	if err != nil {
		return errors.New("could not encode Emby channel field locks")
	}
	item["LockedFields"] = encodedLocks
	item["ChannelNumber"] = json.RawMessage(`""`)
	item["Number"] = json.RawMessage(`""`)
	body, err := json.Marshal(item)
	if err != nil {
		return errors.New("could not encode Emby channel update")
	}
	return c.request(ctx, "POST", "Items/"+id, bytes.NewReader(body), "application/json", nil)
}
