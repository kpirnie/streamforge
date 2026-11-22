// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package app

import (
	"context"
	"maps"
	"slices"
	"sync"
)

type operationProgress struct {
	mu        sync.Mutex
	total     int
	done      int
	failed    int
	skipped   int
	cancelled int
	note      string
	active    map[string]context.CancelFunc
}

type progressSnapshot struct {
	total, done, failed, skipped, cancelled int
	note                                    string
	active                                  []string
}

func (p *operationProgress) setTotal(total int) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.total = total
	p.done, p.failed, p.skipped, p.cancelled = 0, 0, 0, 0
	p.mu.Unlock()
}

func (p *operationProgress) status(note string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.note = note
	p.mu.Unlock()
}

func (p *operationProgress) begin(parent context.Context, label string) (context.Context, func(string)) {
	if p == nil {
		return parent, func(string) {}
	}
	ctx, cancel := context.WithCancel(parent)
	p.mu.Lock()
	if label != "" {
		p.active[label] = cancel
	}
	p.mu.Unlock()
	return ctx, func(outcome string) {
		cancel()
		p.mu.Lock()
		delete(p.active, label)
		switch outcome {
		case "done":
			p.done++
		case "failed":
			p.failed++
		case "skipped":
			p.skipped++
		case "cancelled":
			p.cancelled++
		}
		p.mu.Unlock()
	}
}

func (p *operationProgress) snapshot() progressSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return progressSnapshot{p.total, p.done, p.failed, p.skipped, p.cancelled, p.note, slices.Sorted(maps.Keys(p.active))}
}

func (p *operationProgress) cancel(label string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if cancel := p.active[label]; cancel != nil {
		cancel()
	}
}

func (p *operationProgress) items(total, done, failed, skipped int) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.total, p.done, p.failed, p.skipped, p.cancelled = total, done, failed, skipped, 0
	p.mu.Unlock()
}
