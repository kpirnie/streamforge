// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package dispatcharr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const channelsPath = "/api/channels/channels/"

// Client holds the Dispatcharr connection and the session token.
type Client struct {
	base  *url.URL
	http  *http.Client
	agent string
	token string
}

// Connect authenticates against Dispatcharr without sending credentials across redirects.
func Connect(ctx context.Context, base, username, password, agent string, timeout time.Duration, transport http.RoundTripper) (*Client, error) {
	root, err := url.Parse(base)
	if err != nil || root.Hostname() == "" || (root.Scheme != "https" && root.Scheme != "http") || root.User != nil || root.RawQuery != "" || root.Fragment != "" {
		return nil, errors.New("invalid Dispatcharr URL")
	}
	client := &Client{base: root, agent: agent, http: &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}}
	var token struct {
		Access string `json:"access"`
	}
	err = client.request(ctx, "POST", "/api/accounts/token/", map[string]string{"username": username, "password": password}, &token)
	if err != nil {
		return nil, fmt.Errorf("Dispatcharr authentication: %w", err)
	}
	if token.Access == "" {
		return nil, errors.New("Dispatcharr returned an empty access token")
	}
	client.token = token.Access
	return client, nil
}

// Refresh requests the legacy asynchronous M3U source refresh.
func (c *Client) Refresh(ctx context.Context) error {
	return c.request(ctx, "POST", "/api/m3u/refresh/", nil, nil)
}

func (c *Client) request(ctx context.Context, method, path string, payload, output any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return errors.New("could not encode Dispatcharr request")
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.base.String(), "/")+path, body)
	if err != nil {
		return errors.New("could not construct Dispatcharr request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.agent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("Dispatcharr request failed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Dispatcharr returned HTTP %d", res.StatusCode)
	}
	if output == nil {
		return nil
	}
	if err := json.NewDecoder(res.Body).Decode(output); err != nil {
		return errors.New("could not decode Dispatcharr response")
	}
	return nil
}

func list[T any](ctx context.Context, c *Client, path string, query url.Values) ([]T, error) {
	if query == nil {
		query = make(url.Values)
	}
	query.Set("page_size", "2500")
	var items []T
	for page := 1; ; page++ {
		query.Set("page", fmt.Sprint(page))
		var raw json.RawMessage
		if err := c.request(ctx, "GET", path+"?"+query.Encode(), nil, &raw); err != nil {
			return nil, err
		}
		if len(raw) > 0 && raw[0] == '[' {
			var records []T
			if err := json.Unmarshal(raw, &records); err != nil {
				return nil, errors.New("invalid Dispatcharr list response")
			}
			return append(items, records...), nil
		}
		var result struct {
			Results []T     `json:"results"`
			Next    *string `json:"next"`
		}
		if err := json.Unmarshal(raw, &result); err != nil || result.Results == nil {
			return nil, errors.New("invalid Dispatcharr paginated response")
		}
		items = append(items, result.Results...)
		if result.Next == nil || *result.Next == "" {
			return items, nil
		}
	}
}
