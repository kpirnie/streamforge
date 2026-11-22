// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"os"
	"path/filepath"
	"strings"
)

// PreferredEncoder returns the usable hardware encoder candidate for the selected codec.
func (c Conversion) PreferredEncoder(inventory string) string {
	return c.hardwareEncoder(c.Codec, inventory, false)
}

func hardwareDevice(backend string) string {
	if backend == "nvidia" {
		const path = "/dev/nvidiactl"
		if accessibleDevice(path) {
			return path
		}
		return ""
	}
	vendor := map[string]string{"intel": "0x8086", "amd": "0x1002"}[backend]
	paths, _ := filepath.Glob("/dev/dri/renderD*")
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join("/sys/class/drm", filepath.Base(path), "device/vendor"))
		if err == nil && strings.TrimSpace(string(data)) != vendor {
			continue
		}
		if accessibleDevice(path) {
			return path
		}
	}
	return ""
}

func accessibleDevice(path string) bool {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	file.Close()
	return true
}
