// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"errors"
	"io/fs"
	"os"
	"regexp"
	"time"

	"github.com/kpirnie/streamforge/internal/filelock"
)

var tempName = regexp.MustCompile(`^\.streamforge-(?:export-)?[A-Z2-7]{26}(?:\.[a-zA-Z0-9]{1,12})?$`)

// CleanTemporary removes abandoned application temporary files from managed trees.
func CleanTemporary(root *os.Root) error {
	var failures []error
	for _, tree := range []string{".", "input", "output"} {
		err := fs.WalkDir(root.FS(), tree, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if errors.Is(walkErr, os.ErrNotExist) {
					return nil
				}
				return walkErr
			}
			if path != tree && entry.IsDir() && (tree == "." || excluded(entry.Name(), nil)) {
				return fs.SkipDir
			}
			if !entry.Type().IsRegular() || !tempName.MatchString(entry.Name()) {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return nil
				}
				return err
			}
			// A short grace period protects a file between exclusive creation and locking.
			if !temporaryCandidate(info) || time.Since(info.ModTime()) < time.Minute {
				return nil
			}
			file, err := openMediaFile(root, path, false)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			defer file.Close()
			if !ownsTemporary(file, info) {
				return nil
			}
			err = filelock.Exclusive(file, false)
			if filelock.Busy(err) {
				return nil
			}
			if err != nil {
				return err
			}
			current, err := root.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			if !os.SameFile(current, info) {
				return nil
			}
			err = root.Remove(path)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		})
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
