// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package main

import (
	"os"
	"strings"
)

const sessionSettings = "Session Settings"

type terminalSetting struct {
	key, label, prompt string
	secret             bool
}

var availableSettings = []terminalSetting{
	{"STREAMFORGE_PLAYBACK_PARENT", "Playback parent", "Absolute player-visible path", false},
	{"STREAMFORGE_CODEC", "Video codec", "keep, hevc, or av1", false},
	{"STREAMFORGE_CONTAINER", "Container", "keep, mkv, or mp4", false},
	{"STREAMFORGE_QUALITY", "Quality", "balanced, high, or compression", false},
	{"STREAMFORGE_HARDWARE", "Encoder", "auto, cpu, nvidia, intel, or amd", false},
	{"STREAMFORGE_FORCE", "Reprocess existing", "true or false", false},
	{"STREAMFORGE_CONVERT_HDR", "Convert HDR to SDR", "true or false", false},
	{"STREAMFORGE_DELETE_ORIGINAL", "Delete successful source originals", "true or false", false},
	{"DISP_URL", "Dispatcharr URL", "Dispatcharr server URL", false},
	{"DISP_USER", "Dispatcharr username", "Dispatcharr username", false},
	{"DISP_PASS", "Dispatcharr password", "Dispatcharr password", true},
	{"EMBY_HOST", "Emby host", "Emby server URL", false},
	{"EMBY_API_KEY", "Emby API key", "Emby API key", true},
}

func terminalSettings(env, session map[string]string) []terminalChoice {
	choices := []terminalChoice{{label: "Continue with Defaults", mark: "accept-settings"}}
	for _, setting := range availableSettings {
		if _, ok := os.LookupEnv(setting.key); ok {
			continue
		}
		if _, ok := env[setting.key]; ok {
			continue
		}
		label := setting.label
		if session[setting.key] != "" {
			label += " (set)"
		}
		choices = append(choices, terminalChoice{label: label, prompt: setting.prompt, setting: setting.key, secret: setting.secret})
	}
	return choices
}

func sessionValue(key, fallback string, env, session map[string]string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	if v, ok := env[key]; ok {
		return v
	}
	if v, ok := session[key]; ok {
		return v
	}
	return fallback
}

func checkTerminalSetting(key, value string) bool {
	switch key {
	case "STREAMFORGE_PLAYBACK_PARENT":
		return strings.HasPrefix(value, "/")
	case "STREAMFORGE_CODEC":
		return value == "keep" || value == "hevc" || value == "av1"
	case "STREAMFORGE_CONTAINER":
		return value == "keep" || value == "mkv" || value == "mp4"
	case "STREAMFORGE_QUALITY":
		return value == "balanced" || value == "high" || value == "compression"
	case "STREAMFORGE_FORCE", "STREAMFORGE_CONVERT_HDR", "STREAMFORGE_DELETE_ORIGINAL":
		return value == "true" || value == "false"
	case "STREAMFORGE_HARDWARE":
		return value == "auto" || value == "cpu" || value == "nvidia" || value == "intel" || value == "amd"
	default:
		return value != ""
	}
}
