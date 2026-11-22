// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package emby

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

var playlistAttribute = regexp.MustCompile(`([\w-]+)="([^"]*)"`)
var logoTypes = [...]string{"Logo", "LogoLight", "LogoLightColor"}

type logoIndex struct {
	byName  map[string][]string
	byTVGID map[string][]string
}

type logo struct {
	encoded string
	mime    string
}

func (c *Client) fetch(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	target, err := httpURL(rawURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", target.String(), nil)
	if err != nil {
		return nil, errors.New("could not construct media request")
	}
	req.Header.Set("User-Agent", c.agent)
	response, err := c.media.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("media request failed")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		return nil, fmt.Errorf("media server returned HTTP %d", response.StatusCode)
	}
	return response.Body, nil
}

func (c *Client) playlist(ctx context.Context, rawURL string) (logoIndex, error) {
	index := logoIndex{byName: make(map[string][]string), byTVGID: make(map[string][]string)}
	data, err := c.fetch(ctx, rawURL)
	if err != nil {
		return index, fmt.Errorf("fetch M3U playlist: %w", err)
	}
	defer data.Close()

	scanner := bufio.NewScanner(data)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := strings.TrimPrefix(scanner.Text(), "\uFEFF")
		if !strings.HasPrefix(line, "#EXTINF:") {
			continue
		}
		var logoURL, tvgID string
		for _, attr := range playlistAttribute.FindAllStringSubmatch(line, -1) {
			switch attr[1] {
			case "tvg-logo":
				logoURL = strings.TrimSpace(attr[2])
			case "tvg-id":
				tvgID = strings.ToLower(strings.TrimSpace(attr[2]))
			}
		}
		if logoURL == "" {
			continue
		}
		if name := normalize(line[strings.LastIndexByte(line, ',')+1:]); name != "" {
			index.byName[name] = append(index.byName[name], logoURL)
		}
		if tvgID != "" {
			index.byTVGID[tvgID] = append(index.byTVGID[tvgID], logoURL)
		}
	}
	if scanner.Err() != nil {
		return index, errors.New("could not read M3U playlist")
	}
	return index, nil
}

func normalize(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), " ")
}

func (index logoIndex) match(item channel) string {
	for _, key := range slices.Sorted(maps.Keys(item.ProviderIDs)) {
		if matches := index.byTVGID[strings.ToLower(item.ProviderIDs[key])]; len(matches) == 1 {
			return matches[0]
		}
	}
	if matches := index.byName[normalize(item.Name)]; len(matches) == 1 {
		return matches[0]
	}
	return ""
}

func (c *Client) downloadLogo(ctx context.Context, rawURL string) (logo, error) {
	const limit = 10 * 1024 * 1024
	response, err := c.fetch(ctx, rawURL)
	if err != nil {
		return logo{}, err
	}
	defer response.Close()
	data, err := io.ReadAll(io.LimitReader(response, limit+1))
	if err != nil {
		return logo{}, errors.New("could not read channel logo")
	}
	if len(data) > limit {
		return logo{}, errors.New("channel logo exceeds 10 MiB")
	}
	mime := http.DetectContentType(data)
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return logo{base64.StdEncoding.EncodeToString(data), mime}, nil
	default:
		return logo{}, errors.New("unsupported channel logo format")
	}
}

// RefreshLogos updates the three legacy Emby logo variants using unique playlist matches.
func (c *Client) RefreshLogos(ctx context.Context, playlistURL string, dryRun bool) (Counts, error) {
	var counts Counts
	channels, err := c.channels(ctx)
	if err != nil {
		return counts, err
	}
	index, err := c.playlist(ctx, playlistURL)
	if err != nil {
		return counts, err
	}
	type cachedLogo struct {
		image logo
		err   error
	}
	cache := make(map[string]cachedLogo)
	var failures []error
	for i, item := range channels {
		if ctx.Err() != nil {
			return counts, errors.Join(append(failures, ctx.Err())...)
		}
		source := index.match(item)
		if source == "" {
			counts.Unmatched++
			continue
		}
		id, err := itemPath(item.ID)
		if err == nil && !dryRun {
			entry, exists := cache[source]
			if !exists {
				entry.image, entry.err = c.downloadLogo(ctx, source)
				cache[source] = entry
			}
			err = entry.err
			if err == nil {
				var uploadFailures []error
				for _, kind := range logoTypes {
					if err := c.request(ctx, "POST", "Items/"+id+"/Images/"+kind, bytes.NewBufferString(entry.image.encoded), entry.image.mime, nil); err != nil {
						uploadFailures = append(uploadFailures, fmt.Errorf("%s: %w", kind, err))
					}
				}
				err = errors.Join(uploadFailures...)
			}
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
