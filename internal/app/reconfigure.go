// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package app

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/kpirnie/streamforge/internal/app/menu"
	"github.com/kpirnie/streamforge/internal/xtream"
)

const (
	reconfigurePage         = "Reconfigure"
	applicationSettingsPage = "Application Settings"
	providerListPage        = "Xtream Providers"
	providerEditorPage      = "Edit Xtream Provider"
)

type configurationEditor struct {
	store    *configuration
	settings map[string]string
	records  map[string]json.RawMessage
	name     string
	original json.RawMessage
	draft    map[string]json.RawMessage
	dirty    bool
}

var providerFields = []menu.Choice{
	{Label: "Server URL", Setting: "url", Prompt: "HTTP or HTTPS provider base URL"},
	{Label: "Username", Setting: "username", Prompt: "Provider username"},
	{Label: "Password", Setting: "password", Prompt: "Provider password", Secret: true},
	{Label: "Priority", Setting: "priority", Prompt: "Lower numbers appear first"},
	{Label: "Concurrent downloads", Setting: "concurrent", Prompt: "Positive integer. Leave blank for the default."},
	{Label: "Retries", Setting: "retries", Prompt: "Nonnegative integer. Leave blank for the default."},
	{Label: "Retry delay", Setting: "retry_delay", Prompt: "Positive duration such as 1s. Leave blank for the default."},
	{Label: "Bandwidth limit", Setting: "bandwidth_kbps", Prompt: "KB/s. Leave blank for unlimited."},
	{Label: "HTTP headers", Setting: "headers", Prompt: `JSON object, such as {"User-Agent":"StreamForge"}. Leave blank to clear.`, Secret: true},
}

func (e *configurationEditor) choices(page string) []menu.Choice {
	switch page {
	case reconfigurePage:
		return []menu.Choice{
			{Label: "Application settings", Page: applicationSettingsPage},
			{Label: "Xtream providers", Page: providerListPage},
			{Label: "Done", Mark: "config-done"},
		}
	case applicationSettingsPage:
		return menu.Settings(e.settings)
	case providerListPage:
		choices := []menu.Choice{{Label: "Add provider", Mark: "config-provider-new"}}
		for _, name := range slices.Sorted(maps.Keys(e.records)) {
			choices = append(choices, menu.Choice{Label: name, Setting: name, Mark: "config-provider-open"})
		}
		return choices
	case providerEditorPage:
		var choices []menu.Choice
		if e.original == nil {
			choices = append(choices, menu.Choice{Label: "Provider name", Prompt: "Name shown in search results", Setting: "name", Mark: "config-provider-field"})
		}
		for _, field := range providerFields {
			field.Mark = "config-provider-field"
			choices = append(choices, field)
		}
		choices = append(choices, menu.Choice{Label: "Save provider", Mark: "config-provider-save"})
		if e.original != nil {
			choices = append(choices, menu.Choice{Label: "Remove provider", Mark: "config-provider-remove"})
		}
		return choices
	}
	return nil
}

func reconfigurePrevious(page string) string {
	switch page {
	case reconfigurePage:
		return "Main"
	case applicationSettingsPage, providerListPage:
		return reconfigurePage
	case providerEditorPage:
		return providerListPage
	}
	return ""
}

func (e *configurationEditor) openProvider(name string) {
	e.name, e.original = name, e.records[name]
	e.draft = make(map[string]json.RawMessage)
	if e.original != nil {
		var fields map[string]json.RawMessage
		if json.Unmarshal(e.original, &fields) == nil && fields != nil {
			e.draft = fields
		}
	}
}

func (e *configurationEditor) input(choice menu.Choice) string {
	if choice.Mark == "config-setting" {
		return e.settings[choice.Setting]
	}
	if choice.Setting == "name" {
		return e.name
	}
	field := e.draft[choice.Setting]
	if len(field) == 0 || string(field) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(field, &value) == nil {
		return value
	}
	return string(field)
}

func (e *configurationEditor) edit(ctx context.Context, choice menu.Choice, value string) error {
	if !choice.Secret {
		value = strings.TrimSpace(value)
	}
	key := choice.Setting
	if choice.Mark == "config-setting" {
		if !menu.ValidSetting(key, value) {
			return errors.New("invalid setting value")
		}
		if previous, exists := e.settings[key]; (!exists && value == "") || (exists && previous == value && value != "") {
			return nil
		}
		if err := e.store.saveSetting(ctx, key, value); err != nil {
			return err
		}
		if value == "" {
			delete(e.settings, key)
		} else {
			e.settings[key] = value
		}
		e.dirty = true
		return nil
	}
	if key == "name" {
		e.name = value
		return nil
	}
	if value == "" {
		delete(e.draft, key)
		return nil
	}
	switch key {
	case "priority", "concurrent", "retries", "bandwidth_kbps":
		number, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return errors.New("enter a whole number")
		}
		e.draft[key] = json.RawMessage(strconv.FormatInt(number, 10))
	case "headers":
		var headers map[string]string
		if json.Unmarshal([]byte(value), &headers) != nil || headers == nil {
			return errors.New("enter a JSON object of HTTP headers")
		}
		e.draft[key] = json.RawMessage(value)
	default:
		e.draft[key], _ = json.Marshal(value)
	}
	return nil
}

func (e *configurationEditor) saveProvider(ctx context.Context) error {
	if e.original == nil && e.records[e.name] != nil {
		return errors.New("a provider with that name already exists")
	}
	record, err := json.Marshal(e.draft)
	if err != nil {
		return err
	}
	if err := xtream.ValidateProviderRecord(e.name, record); err != nil {
		return err
	}
	if err := e.store.saveProvider(ctx, e.name, e.original, record); err != nil {
		return err
	}
	e.records[e.name] = record
	e.dirty = true
	return nil
}

func (e *configurationEditor) removeProvider(ctx context.Context) error {
	if err := e.store.saveProvider(ctx, e.name, e.original, nil); err != nil {
		return err
	}
	delete(e.records, e.name)
	e.dirty = true
	return nil
}
