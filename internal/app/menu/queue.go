// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package menu

import (
	"cmp"
	"slices"
)

// QueueItem identifies a movie or series episode selected for download.
type QueueItem struct {
	Provider string `json:"provider"`
	SeriesID string `json:"series_id,omitempty"`
	ID       string `json:"id"`
	Label    string `json:"label"`
}

// Queue returns a snapshot of the current selections.
func (s *Browser) Queue() []QueueItem {
	var items []QueueItem
	for key, title := range s.movies {
		items = append(items, QueueItem{Provider: key.provider, ID: key.id, Label: title})
	}
	for key, episodes := range s.selected {
		for id, title := range episodes {
			items = append(items, QueueItem{Provider: key.provider, SeriesID: key.id, ID: id, Label: title})
		}
	}
	slices.SortFunc(items, func(a, b QueueItem) int {
		return cmp.Or(cmp.Compare(a.Provider, b.Provider), cmp.Compare(a.SeriesID, b.SeriesID), cmp.Compare(a.ID, b.ID))
	})
	return items
}

// RestoreQueue replaces the current selections with a saved snapshot.
func (s *Browser) RestoreQueue(items []QueueItem) {
	clear(s.movies)
	clear(s.selected)
	for _, item := range items {
		if item.SeriesID == "" {
			s.movies[seriesKey{item.Provider, item.ID}] = item.Label
			continue
		}
		key := seriesKey{item.Provider, item.SeriesID}
		if s.selected[key] == nil {
			s.selected[key] = make(map[string]string)
		}
		s.selected[key][item.ID] = item.Label
	}
}
