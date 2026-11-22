// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package dispatcharr

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
)

const streamsPath = "/api/channels/streams/"

type streamRecord struct {
	ID      int     `json:"id"`
	Name    *string `json:"name"`
	Account *int    `json:"m3u_account"`
	TVGID   *string `json:"tvg_id"`
	Group   *int    `json:"channel_group"`
	Stale   bool    `json:"is_stale"`
}

type channelFields struct {
	Name    string  `json:"name"`
	Streams []int   `json:"streams"`
	TVGID   *string `json:"tvg_id"`
	Group   *int    `json:"channel_group_id,omitempty"`
}

// SyncChannels creates missing channels and updates exact name matches.
func (c *Client) SyncChannels(ctx context.Context, normalizer *regexp.Regexp, dryRun bool, progress func(int, int, int, int)) (int, int, error) {
	streams, err := list[streamRecord](ctx, c, streamsPath, nil)
	if err != nil {
		return 0, 0, err
	}
	channels, err := list[channel](ctx, c, channelsPath, nil)
	if err != nil {
		return 0, 0, err
	}

	ids := make(map[string]int, len(channels))
	for _, item := range channels {
		if _, exists := ids[item.Name]; !exists {
			ids[item.Name] = item.ID
		}
	}
	grouped := make(map[string][]streamRecord)
	var order []string
	for _, stream := range streams {
		if stream.ID <= 0 || stream.Name == nil {
			continue
		}
		name := *stream.Name
		if normalizer != nil {
			name = normalizer.ReplaceAllString(name, "")
		}
		if _, exists := grouped[name]; !exists {
			order = append(order, name)
		}
		grouped[name] = append(grouped[name], stream)
	}
	if dryRun {
		return len(order), 0, nil
	}

	done := 0
	if progress != nil {
		progress(len(order), 0, 0, 0)
	}
	var failures []error
	for _, name := range order {
		if err := ctx.Err(); err != nil {
			return done, len(failures), errors.Join(append(failures, err)...)
		}
		id := ids[name]
		if err := c.syncChannel(ctx, id, name, grouped[name]); err != nil {
			failures = append(failures, fmt.Errorf("channel group %d: %w", done+len(failures)+1, err))
			if progress != nil {
				progress(len(order), done, len(failures), 0)
			}
			continue
		}
		done++
		if progress != nil {
			progress(len(order), done, len(failures), 0)
		}
	}
	return done, len(failures), errors.Join(failures...)
}

func (c *Client) syncChannel(ctx context.Context, id int, name string, streams []streamRecord) error {
	slices.SortStableFunc(streams, func(a, b streamRecord) int {
		switch {
		case a.Account == nil && b.Account == nil:
			return 0
		case a.Account == nil:
			return 1
		case b.Account == nil:
			return -1
		default:
			return cmp.Compare(*a.Account, *b.Account)
		}
	})
	fields := channelFields{Name: name, Streams: make([]int, 0, len(streams))}
	for _, stream := range streams {
		fields.Streams = append(fields.Streams, stream.ID)
		if fields.TVGID == nil && stream.TVGID != nil && *stream.TVGID != "" {
			fields.TVGID = stream.TVGID
		}
		if fields.Group == nil && stream.Group != nil {
			fields.Group = stream.Group
		}
	}
	if id == 0 {
		var created struct {
			ID int `json:"id"`
		}
		body := map[string]any{"name": name, "stream_id": fields.Streams[0]}
		if err := c.request(ctx, "POST", channelsPath+"from-stream/", body, &created); err != nil {
			return err
		}
		if created.ID <= 0 {
			return errors.New("created channel response is missing its ID")
		}
		id = created.ID
	}
	if err := c.request(ctx, "PUT", fmt.Sprintf("%s%d/", channelsPath, id), fields, nil); err != nil {
		return fmt.Errorf("update channel %d: %w", id, err)
	}
	return nil
}
