// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package dispatcharr

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
)

const groupsPath = "/api/channels/groups/"

// PruneResult contains the counts from a prune operation or preview.
type PruneResult struct {
	Streams  int
	Channels int
	Groups   int
	Failed   int
}

type channelGroup struct {
	ID       int  `json:"id"`
	Channels *int `json:"channel_count"`
	Accounts *int `json:"m3u_account_count"`
}

// Prune removes stale source streams and the channels and groups they leave empty.
func (c *Client) Prune(ctx context.Context, dryRun bool) (PruneResult, error) {
	var result PruneResult
	streams, err := list[streamRecord](ctx, c, streamsPath, nil)
	if err != nil {
		return result, err
	}
	stale := make(map[int]bool)
	var ids []int
	for _, stream := range streams {
		if stream.ID > 0 && stream.Account != nil && stream.Stale && !stale[stream.ID] {
			stale[stream.ID] = true
			ids = append(ids, stream.ID)
		}
	}
	if len(ids) == 0 {
		return result, nil
	}
	channels, err := list[channel](ctx, c, channelsPath, url.Values{"include_streams": {"true"}})
	if err != nil {
		return result, err
	}

	// Limit the cascade to channels that lose their last stream in this operation.
	var candidates []channel
	candidateCounts := make(map[int]int)
	for _, item := range channels {
		if item.ID <= 0 || len(item.Streams) == 0 {
			continue
		}
		if !slices.ContainsFunc(item.Streams, func(stream channelStream) bool { return !stale[stream.ID] }) {
			candidates = append(candidates, item)
			candidateCounts[item.groupID()]++
		}
	}
	if dryRun {
		result.Streams, result.Channels = len(ids), len(candidates)
		if len(candidates) == 0 {
			return result, nil
		}
		groups, err := list[channelGroup](ctx, c, groupsPath, nil)
		if err != nil {
			return result, err
		}
		for _, group := range groups {
			if count := candidateCounts[group.ID]; count > 0 && group.Channels != nil && group.Accounts != nil && *group.Channels == count && *group.Accounts == 0 {
				result.Groups++
			}
		}
		return result, nil
	}

	if err := c.request(ctx, "DELETE", streamsPath+"bulk-delete/", map[string][]int{"stream_ids": ids}, nil); err != nil {
		result.Failed = len(ids)
		return result, err
	}
	result.Streams = len(ids)
	current, err := list[channel](ctx, c, channelsPath, url.Values{"include_streams": {"true"}})
	if err != nil {
		return result, err
	}
	byID := make(map[int]channel, len(current))
	for _, item := range current {
		byID[item.ID] = item
	}
	var failures []error
	var groupIDs []int
	for _, candidate := range candidates {
		item, exists := byID[candidate.ID]
		// A missing stream list is not sufficient evidence that a channel is empty.
		if !exists || item.Streams == nil || len(item.Streams) != 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return result, errors.Join(append(failures, err)...)
		}
		if err := c.request(ctx, "DELETE", fmt.Sprintf("%s%d/", channelsPath, item.ID), nil, nil); err != nil {
			result.Failed++
			failures = append(failures, fmt.Errorf("delete channel %d: %w", item.ID, err))
			continue
		}
		result.Channels++
		if id := item.groupID(); id > 0 && !slices.Contains(groupIDs, id) {
			groupIDs = append(groupIDs, id)
		}
	}
	for _, id := range groupIDs {
		if err := ctx.Err(); err != nil {
			return result, errors.Join(append(failures, err)...)
		}
		path := fmt.Sprintf("%s%d/", groupsPath, id)
		var group channelGroup
		err := c.request(ctx, "GET", path, nil, &group)
		if err == nil && group.Channels != nil && group.Accounts != nil && *group.Channels == 0 && *group.Accounts == 0 {
			err = c.request(ctx, "DELETE", path, nil, nil)
			if err == nil {
				result.Groups++
			}
		}
		if err != nil {
			result.Failed++
			failures = append(failures, fmt.Errorf("check or delete group %d: %w", id, err))
		}
	}
	return result, errors.Join(failures...)
}
