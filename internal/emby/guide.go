// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package emby

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
)

type taskResult struct {
	StartTimeUTC string
	EndTimeUTC   string
	Status       string
}

type task struct {
	ID                  string
	Name                string
	State               string
	LastExecutionResult taskResult
}

// RefreshGuide locates the guide task and waits for this execution to finish.
func (c *Client) RefreshGuide(ctx context.Context, dryRun bool) error {
	var tasks []task
	if err := c.request(ctx, "GET", "ScheduledTasks", nil, "", &tasks); err != nil {
		return err
	}
	index := slices.IndexFunc(tasks, func(item task) bool { return strings.Contains(strings.ToLower(item.Name), "guide") })
	if index < 0 {
		return errors.New("no Emby guide refresh task found")
	}
	selected := tasks[index]
	id, err := itemPath(selected.ID)
	if err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	path := "ScheduledTasks/" + id
	var baseline task
	if err := c.request(ctx, "GET", path, nil, "", &baseline); err != nil {
		return err
	}
	if strings.EqualFold(baseline.State, "Idle") {
		if err := c.request(ctx, "POST", "ScheduledTasks/Running/"+id, nil, "", nil); err != nil {
			return err
		}
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	for {
		var current task
		if err := c.request(ctx, "GET", path, nil, "", &current); err != nil {
			return err
		}
		result := current.LastExecutionResult
		if strings.EqualFold(current.State, "Idle") && result != baseline.LastExecutionResult && result.Status != "" {
			if !strings.EqualFold(result.Status, "Completed") {
				return errors.New("Emby guide refresh did not complete successfully")
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("timed out waiting for Emby guide refresh completion")
		case <-ticker.C:
		}
	}
}
