// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package app

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"unicode"
)

var globalOptions = map[string]string{
	"--timeout":         "STREAMFORGE_TIMEOUT",
	"--verify-tls":      "STREAMFORGE_VERIFY_TLS",
	"--user-agent":      "STREAMFORGE_USER_AGENT",
	"--start-channel":   "STREAMFORGE_START_CHANNEL",
	"--normalizer":      "STREAMFORGE_NORMALIZER",
	"--ffmpeg":          "STREAMFORGE_FFMPEG",
	"--ffprobe":         "STREAMFORGE_FFPROBE",
	"--playback-parent": "STREAMFORGE_PLAYBACK_PARENT",
}

var settingKeys = []string{
	"STREAMFORGE_TIMEOUT", "STREAMFORGE_VERIFY_TLS", "STREAMFORGE_USER_AGENT", "STREAMFORGE_CONFIRM",
	"STREAMFORGE_START_CHANNEL", "STREAMFORGE_NORMALIZER", "STREAMFORGE_FFMPEG", "STREAMFORGE_FFPROBE",
	"STREAMFORGE_PLAYBACK_PARENT", "STREAMFORGE_EXCLUDE", "STREAMFORGE_CODEC", "STREAMFORGE_CONTAINER",
	"STREAMFORGE_QUALITY", "STREAMFORGE_HARDWARE", "STREAMFORGE_JOBS",
	"STREAMFORGE_FORCE", "STREAMFORGE_CONVERT_HDR", "STREAMFORGE_DELETE_ORIGINAL",
	"STREAMFORGE_RESOLUTION", "STREAMFORGE_FRAME_RATE", "STREAMFORGE_ASPECT", "STREAMFORGE_WORK_DIR",
}

func unknownSettings(env map[string]string) []string {
	keys := make(map[string]bool)
	accept := func(key string) {
		if strings.HasPrefix(key, "STREAMFORGE_") && !slices.Contains(settingKeys, key) {
			keys[key] = true
		}
	}
	for key := range env {
		accept(key)
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "STREAMFORGE_RELEASE", "STREAMFORGE_BIN_DIR":
			continue
		}
		accept(key)
	}
	var warnings []string
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		if strings.IndexFunc(key, func(r rune) bool { return !unicode.IsPrint(r) || unicode.Is(unicode.Cf, r) }) >= 0 {
			warnings = append(warnings, "Unrecognized Stream Forge environment variable name")
		} else {
			warnings = append(warnings, fmt.Sprintf("Unrecognized environment variable %s", key))
		}
	}
	return warnings
}

func settingValue(key, fallback string, saved map[string]string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	if v, ok := saved[key]; ok {
		return v
	}
	return fallback
}
