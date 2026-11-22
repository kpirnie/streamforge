// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package dispatcharr

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type refreshAccount struct {
	ID      int    `json:"id"`
	Active  *bool  `json:"is_active"`
	Status  string `json:"status"`
	Updated string `json:"updated_at"`
	Message string `json:"last_message"`
}

type refreshState struct {
	baseline     refreshAccount
	observedWork bool
}

// RefreshAndWait waits for tracked active accounts before dependent batch operations.
func (c *Client) RefreshAndWait(ctx context.Context, queued func()) error {
	const accountsPath = "/api/m3u/accounts/"
	accounts, err := list[refreshAccount](ctx, c, accountsPath, nil)
	if err != nil {
		return err
	}
	pending := make(map[int]refreshState)
	for _, account := range accounts {
		if account.ID > 0 && account.Active != nil && *account.Active && account.Status != "disabled" {
			pending[account.ID] = refreshState{baseline: account, observedWork: account.Status == "fetching" || account.Status == "parsing"}
		}
	}
	if err := c.Refresh(ctx); err != nil {
		return err
	}
	queued()
	if len(pending) == 0 {
		return nil
	}
	wait, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var failures []error
	for len(pending) != 0 {
		select {
		case <-wait.Done():
			return errors.Join(append(failures, errors.New("waiting for Dispatcharr M3U refresh: "+wait.Err().Error()))...)
		case <-ticker.C:
		}
		current, err := list[refreshAccount](wait, c, accountsPath, nil)
		if err != nil {
			return errors.Join(append(failures, err)...)
		}
		for _, account := range current {
			state, ok := pending[account.ID]
			if !ok {
				continue
			}
			switch account.Status {
			case "fetching", "parsing":
				state.observedWork = true
				pending[account.ID] = state
			case "success":
				if account.Updated != "" && (account.Updated != state.baseline.Updated || state.observedWork) {
					delete(pending, account.ID)
				}
			case "error":
				if state.observedWork || state.baseline.Status != "error" || account.Message != state.baseline.Message {
					failures = append(failures, fmt.Errorf("M3U account %d failed to refresh", account.ID))
					delete(pending, account.ID)
				}
			}
		}
	}
	return errors.Join(failures...)
}
