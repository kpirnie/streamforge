// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const sourceAttribute = "user.streamforge.source"

func fingerprint(path string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(path)))
}

func ownership(root *os.Root, destination, input string) (bool, bool, error) {
	file, err := openMediaFile(root, destination, true)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if errors.Is(err, syscall.ELOOP) {
		return true, false, nil
	}
	if err != nil {
		return false, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, false, err
	}
	if !info.Mode().IsRegular() {
		return true, false, nil
	}
	var value [64]byte
	size, err := readSource(file, value[:])
	if err != nil {
		return false, false, err
	}
	return true, string(value[:size]) == fingerprint(input), nil
}

// SelectTarget keeps a corresponding output or allocates a collision-safe filename.
func SelectTarget(root *os.Root, output, input string, reserved map[string]bool) (string, error) {
	exists, owned, err := ownership(root, output, input)
	if err != nil {
		return "", err
	}
	if !reserved[output] && (!exists || owned) {
		return output, nil
	}
	ext := filepath.Ext(output)
	stem := strings.TrimSuffix(filepath.Base(output), ext) + "-"
	dir := filepath.Dir(output)
	entries, err := fs.ReadDir(root.FS(), filepath.ToSlash(dir))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	for _, entry := range entries {
		candidate := filepath.Join(dir, entry.Name())
		if reserved[candidate] || len(entry.Name()) != len(stem)+8+len(ext) || !strings.HasPrefix(entry.Name(), stem) || !strings.HasSuffix(entry.Name(), ext) {
			continue
		}
		_, owned, err := ownership(root, candidate, input)
		if err != nil {
			return "", err
		}
		if owned {
			return candidate, nil
		}
	}
	for {
		candidate := filepath.Join(dir, stem+rand.Text()[:8]+ext)
		if reserved[candidate] {
			continue
		}
		if _, err := root.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
}

func previousCompanion(root *os.Root, destination, source string) bool {
	file, err := openMediaFile(root, destination, true)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(file, 8<<10))
	if err != nil {
		return false
	}
	target := strings.TrimSuffix(string(data), "\n")
	return ValidPlaybackParent(target) && strings.HasSuffix(filepath.ToSlash(target), "/"+filepath.ToSlash(source))
}
