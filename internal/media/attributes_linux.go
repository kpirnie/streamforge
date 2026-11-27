// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func descriptor(file *os.File) string {
	return fmt.Sprintf("/proc/self/fd/%d", file.Fd())
}

func readSource(file *os.File, value []byte) (int, error) {
	size, err := syscall.Getxattr(descriptor(file), sourceAttribute, value)
	if errors.Is(err, syscall.ENODATA) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.ERANGE) {
		return 0, nil
	}
	return size, err
}

func markSource(file *os.File, source string) error {
	err := syscall.Setxattr(descriptor(file), sourceAttribute, []byte(fingerprint(source)), 0)
	if errors.Is(err, syscall.ENOTSUP) {
		return nil
	}
	return err
}
