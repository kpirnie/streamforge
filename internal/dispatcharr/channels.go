// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package dispatcharr

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/kpirnie/streamforge/internal/media"
)

type channel struct {
	ID             int             `json:"id"`
	Name           string          `json:"name"`
	TVGID          string          `json:"tvg_id"`
	EPGID          int             `json:"epg_data_id"`
	Group          int             `json:"channel_group_id"`
	EffectiveGroup *int            `json:"effective_channel_group_id"`
	Streams        []channelStream `json:"streams"`
}

type channelStream struct {
	ID      int `json:"id"`
	Account int `json:"m3u_account"`
}

// Dispatcharr sends IDs by default and stream objects with include_streams.
func (s *channelStream) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '{' {
		type wireStream channelStream
		return json.Unmarshal(data, (*wireStream)(s))
	}
	return json.Unmarshal(data, &s.ID)
}

func (c channel) groupID() int {
	if c.EffectiveGroup != nil {
		return *c.EffectiveGroup
	}
	return c.Group
}

type update struct {
	id     int
	fields map[string]any
}

// Export fetches every channel and atomically publishes channels.csv.
func (c *Client) Export(ctx context.Context, root *os.Root, dryRun bool) (int, error) {
	channels, err := list[channel](ctx, c, channelsPath, nil)
	if err != nil {
		return 0, err
	}
	if dryRun {
		return len(channels), nil
	}
	alphabetical(channels)
	err = media.AtomicWrite(ctx, root, "channels.csv", 0666, true, func(file *os.File) error {
		writer := csv.NewWriter(file)
		if err := writer.Write([]string{"Channel Name", "TVG ID"}); err != nil {
			return err
		}
		for _, record := range channels {
			if err := writer.Write([]string{record.Name, record.TVGID}); err != nil {
				return err
			}
		}
		writer.Flush()
		return writer.Error()
	})
	if err != nil {
		return 0, err
	}
	return len(channels), nil
}

func alphabetical(channels []channel) {
	slices.SortStableFunc(channels, func(a, b channel) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
}

// UpdateChannels plans or applies the selected channel updates.
func (c *Client) UpdateChannels(ctx context.Context, action string, start int, dryRun bool, progress func(int, int, int, int)) (int, int, error) {
	query := make(url.Values)
	if action == "renumber" || action == "reorder" {
		query.Set("include_streams", "true")
	}
	channels, err := list[channel](ctx, c, channelsPath, query)
	if err != nil {
		return 0, 0, err
	}
	var changes []update
	switch action {
	case "renumber":
		alphabetical(channels)
		for _, item := range channels {
			if len(item.Streams) == 0 {
				continue
			}
			changes = append(changes, update{item.ID, map[string]any{"channel_number": start}})
			start++
		}
	case "clear-epg":
		for _, item := range channels {
			if item.EPGID != 0 {
				changes = append(changes, update{item.ID, map[string]any{"epg_data_id": nil}})
			}
		}
	case "match":
		records, err := list[struct {
			ID    int    `json:"id"`
			TVGID string `json:"tvg_id"`
		}](ctx, c, "/api/epg/epgdata/", nil)
		if err != nil {
			return 0, 0, err
		}
		identifiers := make(map[int]string, len(records))
		for _, record := range records {
			identifiers[record.ID] = record.TVGID
		}
		for _, item := range channels {
			if id, ok := identifiers[item.EPGID]; ok && item.TVGID != id {
				changes = append(changes, update{item.ID, map[string]any{"tvg_id": id}})
			}
		}
	case "reorder":
		records, err := list[struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}](ctx, c, "/api/m3u/accounts/", nil)
		if err != nil {
			return 0, 0, err
		}
		names := make(map[int]string, len(records))
		for _, record := range records {
			names[record.ID] = strings.ToLower(record.Name)
		}
		for _, item := range channels {
			ids := make([]int, len(item.Streams))
			order := make([]int, len(item.Streams))
			for i, stream := range item.Streams {
				ids[i] = stream.ID
				order[i] = i
			}
			slices.SortStableFunc(order, func(a, b int) int {
				left, lok := names[item.Streams[a].Account]
				right, rok := names[item.Streams[b].Account]
				if lok && !rok {
					return -1
				}
				if !lok && rok {
					return 1
				}
				return strings.Compare(left, right)
			})
			sorted := make([]int, len(ids))
			for i, index := range order {
				sorted[i] = ids[index]
			}
			if !slices.Equal(ids, sorted) {
				changes = append(changes, update{item.ID, map[string]any{"streams": sorted}})
			}
		}
	default:
		return 0, 0, errors.New("unknown Dispatcharr action")
	}
	if dryRun {
		return len(changes), 0, nil
	}
	if progress != nil {
		progress(len(changes), 0, 0, 0)
	}
	var failures []error
	done := 0
	for _, change := range changes {
		if ctx.Err() != nil {
			return done, len(failures), errors.Join(append(failures, ctx.Err())...)
		}
		if err := c.request(ctx, "PATCH", fmt.Sprintf("%s%d/", channelsPath, change.id), change.fields, nil); err != nil {
			failures = append(failures, fmt.Errorf("channel %d: %w", change.id, err))
			if progress != nil {
				progress(len(changes), done, len(failures), 0)
			}
			continue
		}
		done++
		if progress != nil {
			progress(len(changes), done, len(failures), 0)
		}
	}
	return done, len(failures), errors.Join(failures...)
}
