// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

//go:build linux || darwin

package app

import "os"

func applyUpdate(staged, installed string, _ os.FileInfo) (bool, error) {
	return false, os.Rename(staged, installed)
}
