// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package menu

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kpirnie/streamforge/internal/media"
)

type terminalSetting struct {
	key, label, prompt string
	secret             bool
}

var availableSettings = []terminalSetting{
	{"DISP_URL", "Dispatcharr URL", "Dispatcharr server URL", false},
	{"DISP_USER", "Dispatcharr username", "Dispatcharr username", false},
	{"DISP_PASS", "Dispatcharr password", "Dispatcharr password", true},
	{"DISP_M3U_URL", "Dispatcharr playlist URL", "HTTP or HTTPS playlist URL", true},
	{"EMBY_HOST", "Emby host", "Emby server URL", false},
	{"EMBY_API_KEY", "Emby API key", "Emby API key", true},
	{"TMDB_API_KEY", "TMDB API key", "TMDB API key", true},
	{"STREAMFORGE_TIMEOUT", "HTTP timeout", "Positive duration, such as 30s", false},
	{"STREAMFORGE_VERIFY_TLS", "Verify TLS certificates", "true or false", false},
	{"STREAMFORGE_USER_AGENT", "HTTP user agent", "User agent string", false},
	{"STREAMFORGE_CONFIRM", "Confirm protected actions", "true or false", false},
	{"STREAMFORGE_START_CHANNEL", "First channel number", "Positive integer", false},
	{"STREAMFORGE_NORMALIZER", "Channel normalizer", "Go regular expression", false},
	{"STREAMFORGE_WORK_DIR", "Media workspace", "Absolute directory path. Changes apply after restart.", false},
	{"STREAMFORGE_PLAYBACK_PARENT", "Playback parent", "Absolute player-visible path", false},
	{"STREAMFORGE_FFMPEG", "FFmpeg executable", "Absolute executable path", false},
	{"STREAMFORGE_FFPROBE", "FFprobe executable", "Absolute executable path", false},
	{"STREAMFORGE_EXCLUDE", "Excluded media names", "Comma-separated filename globs", false},
	{"STREAMFORGE_CODEC", "Video codec", "keep, hevc, or av1", false},
	{"STREAMFORGE_CONTAINER", "Container", "keep, mkv, or mp4", false},
	{"STREAMFORGE_QUALITY", "Quality", "script, balanced, high, or compression", false},
	{"STREAMFORGE_HARDWARE", "Encoder", "auto, cpu, nvidia, intel, or amd", false},
	{"STREAMFORGE_JOBS", "Transcode jobs", "Positive integer", false},
	{"STREAMFORGE_FORCE", "Reprocess existing", "true or false", false},
	{"STREAMFORGE_CONVERT_HDR", "Convert HDR to SDR", "true or false", false},
	{"STREAMFORGE_DELETE_ORIGINAL", "Delete successful source originals", "true or false", false},
	{"STREAMFORGE_RESOLUTION", "Resolution", "Width and height, such as 1920x1080", false},
	{"STREAMFORGE_FRAME_RATE", "Frame rate", "Frames per second, such as 24 or 24000/1001", false},
	{"STREAMFORGE_ASPECT", "Aspect ratio", "Aspect ratio, such as 16:9", false},
}

// Settings lists the saved application settings that can be reconfigured.
func Settings(saved map[string]string) []Choice {
	choices := make([]Choice, 0, len(availableSettings))
	for _, setting := range availableSettings {
		label := setting.label
		if _, set := os.LookupEnv(setting.key); set {
			label += " (environment override)"
		} else if _, set := saved[setting.key]; set {
			label += " (saved)"
		}
		choices = append(choices, Choice{Label: label, Prompt: setting.prompt + " Leave blank to clear the saved value.", Setting: setting.key, Secret: setting.secret, Mark: "config-setting"})
	}
	return choices
}

// ValidSetting reports whether the menu accepts the entered value.
func ValidSetting(key, value string) bool {
	if value == "" {
		return true
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	switch key {
	case "STREAMFORGE_PLAYBACK_PARENT":
		return media.ValidPlaybackParent(value)
	case "STREAMFORGE_WORK_DIR", "STREAMFORGE_FFMPEG", "STREAMFORGE_FFPROBE":
		return filepath.IsAbs(value)
	case "STREAMFORGE_CODEC":
		return value == "keep" || value == "hevc" || value == "av1"
	case "STREAMFORGE_CONTAINER":
		return value == "keep" || value == "mkv" || value == "mp4"
	case "STREAMFORGE_QUALITY":
		return value == "script" || value == "balanced" || value == "high" || value == "compression"
	case "STREAMFORGE_FORCE", "STREAMFORGE_CONVERT_HDR", "STREAMFORGE_DELETE_ORIGINAL", "STREAMFORGE_VERIFY_TLS", "STREAMFORGE_CONFIRM":
		return value == "true" || value == "false"
	case "STREAMFORGE_HARDWARE":
		return value == "auto" || value == "cpu" || value == "nvidia" || value == "intel" || value == "amd"
	case "STREAMFORGE_JOBS", "STREAMFORGE_START_CHANNEL":
		n, err := strconv.Atoi(value)
		return err == nil && n > 0
	case "STREAMFORGE_TIMEOUT":
		duration, err := time.ParseDuration(value)
		return err == nil && duration > 0
	case "STREAMFORGE_NORMALIZER":
		_, err := regexp.Compile(value)
		return err == nil
	case "DISP_URL", "DISP_M3U_URL", "EMBY_HOST":
		address, err := url.Parse(value)
		return err == nil && address.Hostname() != "" && (address.Scheme == "http" || address.Scheme == "https")
	case "STREAMFORGE_EXCLUDE":
		for _, pattern := range strings.Split(value, ",") {
			if _, err := filepath.Match(pattern, ""); err != nil {
				return false
			}
		}
	case "STREAMFORGE_RESOLUTION", "STREAMFORGE_FRAME_RATE", "STREAMFORGE_ASPECT":
		return media.ValidVideoSetting(key, value)
	}
	return true
}
