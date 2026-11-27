// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package xtream

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"
)

// Providers holds a session's ordered Xtream providers.
type Providers struct {
	ordered []provider
	client  *http.Client
	agent   string
}

type provider struct {
	name       string
	url        *url.URL
	settings   providerConfig
	retryDelay time.Duration
}

type providerConfig struct {
	URL           string            `json:"url"`
	Username      string            `json:"username"`
	Password      string            `json:"password"`
	Priority      int               `json:"priority"`
	Concurrent    *int              `json:"concurrent"`
	Retries       *int              `json:"retries"`
	RetryDelay    string            `json:"retry_delay"`
	BandwidthKBPS int64             `json:"bandwidth_kbps"`
	Headers       map[string]string `json:"headers"`
}

// ReadProviderRecords reads the legacy provider configuration for migration.
func ReadProviderRecords(path string) (map[string]json.RawMessage, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, errors.New("could not open Xtream provider configuration")
	}
	defer file.Close()
	const limit = 8 << 20
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(data) > limit {
		return nil, errors.New("could not read Xtream provider configuration")
	}
	var records map[string]json.RawMessage
	if err := json.Unmarshal(data, &records); err != nil || records == nil {
		return nil, errors.New("Xtream provider configuration must be an object keyed by provider name")
	}
	return records, nil
}

// LoadProviders validates saved provider records and constructs their client.
func LoadProviders(records map[string]json.RawMessage, agent string, timeout time.Duration, transport http.RoundTripper) (*Providers, []error, error) {
	providers := &Providers{
		agent:  agent,
		client: &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
	names := make([]string, 0, len(records))
	for name := range records {
		names = append(names, name)
	}
	slices.Sort(names)
	var warnings []error
	for i, name := range names {
		entry, err := parseProvider(name, records[name])
		if err != nil {
			warnings = append(warnings, fmt.Errorf("Xtream provider entry %d: %w", i+1, err))
			continue
		}
		providers.ordered = append(providers.ordered, entry)
	}
	slices.SortStableFunc(providers.ordered, func(a, b provider) int {
		if order := cmp.Compare(a.settings.Priority, b.settings.Priority); order != 0 {
			return order
		}
		return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
	})
	if len(providers.ordered) == 0 {
		return nil, warnings, errors.New("no valid Xtream providers")
	}
	return providers, warnings, nil
}

// ValidateProviderRecord applies the same checks as loading the provider at runtime.
func ValidateProviderRecord(name string, record json.RawMessage) error {
	_, err := parseProvider(name, record)
	return err
}

func parseProvider(name string, raw json.RawMessage) (provider, error) {
	entry := provider{name: name, retryDelay: time.Second}
	if strings.TrimSpace(name) == "" || cleanDisplay(name) != name {
		return entry, errors.New("invalid provider name")
	}
	cfg := &entry.settings
	if err := json.Unmarshal(raw, cfg); err != nil {
		return entry, errors.New("invalid provider field type")
	}
	if cfg.URL == "" || cfg.Username == "" || cfg.Password == "" {
		return entry, errors.New("url, username and password are required")
	}
	target, err := url.Parse(cfg.URL)
	if err != nil || target.Hostname() == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" || (target.Scheme != "http" && target.Scheme != "https") {
		return entry, errors.New("invalid provider URL")
	}
	entry.url = target
	if (cfg.Concurrent != nil && *cfg.Concurrent <= 0) || (cfg.Retries != nil && *cfg.Retries < 0) || cfg.BandwidthKBPS < 0 {
		return entry, errors.New("invalid concurrency, retries or bandwidth limit")
	}
	if cfg.RetryDelay != "" {
		entry.retryDelay, err = time.ParseDuration(cfg.RetryDelay)
		if err != nil || entry.retryDelay <= 0 {
			return entry, errors.New("retry_delay must be a positive duration")
		}
	}
	headers := make(map[string]string, len(cfg.Headers))
	for key, value := range cfg.Headers {
		canonical := http.CanonicalHeaderKey(key)
		if canonical == "" || strings.IndexFunc(key, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r))
		}) >= 0 || strings.IndexFunc(value, func(r rune) bool { return r != '\t' && (r < 32 || r == 127) }) >= 0 {
			return entry, errors.New("invalid provider HTTP header")
		}
		if _, exists := headers[canonical]; exists {
			return entry, errors.New("duplicate provider HTTP header")
		}
		headers[canonical] = value
	}
	cfg.Headers = headers
	return entry, nil
}

func (p *Providers) selectProviders(name string) ([]provider, error) {
	if name == "" {
		return p.ordered, nil
	}
	for _, entry := range p.ordered {
		if entry.name == name {
			return []provider{entry}, nil
		}
	}
	return nil, errors.New("unknown Xtream provider")
}
