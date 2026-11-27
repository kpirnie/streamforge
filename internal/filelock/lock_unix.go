// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

//go:build linux || darwin

package filelock

import (
	"errors"
	"os"
	"syscall"
)

// OpenQueue opens the workspace directory used to coordinate queue access.
func OpenQueue(root *os.Root) (*os.File, error) {
	return root.Open(".")
}

// Exclusive holds an exclusive lock until the file is closed.
func Exclusive(file *os.File, wait bool) error {
	flags := syscall.LOCK_EX
	if !wait {
		flags |= syscall.LOCK_NB
	}
	return syscall.Flock(int(file.Fd()), flags)
}

// Busy reports whether an exclusive lock is already held.
func Busy(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK)
}
