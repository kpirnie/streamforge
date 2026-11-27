// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"

	"github.com/kpirnie/streamforge/internal/app/menu"
	"github.com/kpirnie/streamforge/internal/filelock"
	"github.com/kpirnie/streamforge/internal/media"
)

const queueFilename = "download-queue.yaml"

type downloadQueueStore struct {
	root  *os.Root
	lock  *os.File
	mu    sync.Mutex
	items []menu.QueueItem
}

func openDownloadQueue() (*downloadQueueStore, error) {
	root, err := os.OpenRoot(media.Working)
	if err != nil {
		return nil, err
	}
	lock, err := filelock.OpenQueue(root)
	if err != nil {
		root.Close()
		return nil, err
	}
	queue := &downloadQueueStore{root: root, lock: lock}
	if err := filelock.Exclusive(lock, false); err != nil {
		queue.close()
		return nil, fmt.Errorf("lock download queue: %w", err)
	}
	if err := queue.load(); err != nil {
		queue.close()
		return nil, err
	}
	return queue, nil
}

func (q *downloadQueueStore) close() {
	q.lock.Close()
	q.root.Close()
}

func (q *downloadQueueStore) load() error {
	info, err := q.root.Lstat(queueFilename)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("download queue must be a regular file")
	}
	data, err := q.root.ReadFile(queueFilename)
	if err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("[]")) {
		return nil
	}
	for line := range bytes.SplitSeq(bytes.TrimSpace(data), []byte("\n")) {
		record, ok := bytes.CutPrefix(bytes.TrimSpace(line), []byte("- "))
		var item menu.QueueItem
		if !ok || json.Unmarshal(record, &item) != nil || item.Provider == "" || item.ID == "" || item.Label == "" {
			return errors.New("invalid entry in download-queue.yaml")
		}
		q.items = append(q.items, item)
	}
	return nil
}

func (q *downloadQueueStore) snapshot() []menu.QueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.items)
}

func (q *downloadQueueStore) replace(items []menu.QueueItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.save(items)
}

func (q *downloadQueueStore) complete(provider, seriesID, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	items := slices.DeleteFunc(slices.Clone(q.items), func(item menu.QueueItem) bool {
		return item.Provider == provider && item.SeriesID == seriesID && item.ID == id
	})
	return q.save(items)
}

// save publishes the YAML sequence atomically before updating the in-memory copy.
func (q *downloadQueueStore) save(items []menu.QueueItem) error {
	if slices.Equal(q.items, items) {
		return nil
	}
	err := media.AtomicWrite(context.Background(), q.root, queueFilename, 0600, true, func(file *os.File) error {
		if len(items) == 0 {
			_, err := file.WriteString("[]\n")
			return err
		}
		// JSON objects are valid YAML flow mappings and safely quote provider text.
		encoder := json.NewEncoder(file)
		encoder.SetEscapeHTML(false)
		for _, item := range items {
			if _, err := file.WriteString("- "); err != nil {
				return err
			}
			if err := encoder.Encode(item); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		q.items = items
	}
	return err
}
