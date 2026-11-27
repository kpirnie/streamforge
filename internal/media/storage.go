// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// DestinationSpace checks a managed destination's permissions and available bytes.
func DestinationSpace(root *os.Root, tree string, required bool) (uint64, error) {
	if tree != "input" && tree != "output" {
		return 0, errors.New("invalid media destination")
	}
	dir := filepath.Join(root.Name(), tree)
	info, err := root.Lstat(tree)
	if errors.Is(err, os.ErrNotExist) && !required {
		dir = root.Name()
	} else if err != nil || !info.IsDir() {
		return 0, errors.New("media destination must be a real directory")
	}
	if err := syscall.Access(dir, 3); err != nil {
		return 0, errors.New("media destination is not writable")
	}
	var stats syscall.Statfs_t
	if err := syscall.Statfs(dir, &stats); err != nil {
		return 0, errors.New("could not check media destination space")
	}
	return stats.Bavail * uint64(stats.Bsize), nil
}
