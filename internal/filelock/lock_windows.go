// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package filelock

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

var lockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

// OpenQueue opens the regular file used to coordinate queue access.
func OpenQueue(root *os.Root) (*os.File, error) {
	const name = ".streamforge-queue.lock"
	file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if !errors.Is(err, os.ErrExist) {
		return file, err
	}
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errors.New("download queue lock must be a regular file")
	}
	file, err = root.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	after, err := root.Lstat(name)
	if err != nil {
		file.Close()
		return nil, err
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, opened) || !os.SameFile(opened, after) {
		file.Close()
		return nil, errors.New("download queue lock changed while opening")
	}
	return file, nil
}

// Exclusive holds an exclusive lock until the file is closed.
func Exclusive(file *os.File, wait bool) error {
	const (
		exclusiveLock   = 2
		failImmediately = 1
	)
	flags := uintptr(exclusiveLock)
	if !wait {
		flags |= failImmediately
	}
	// Windows byte locks are mandatory. Lock a byte beyond the usable file
	// range so the encoder can still write through its inherited handle.
	overlapped := syscall.Overlapped{Offset: 0xfffffffe, OffsetHigh: 0x7fffffff}
	result, _, err := lockFileEx.Call(file.Fd(), flags, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	runtime.KeepAlive(file)
	if result == 0 {
		return err
	}
	return nil
}

// Busy reports whether an exclusive lock is already held.
func Busy(err error) bool {
	const errorLockViolation syscall.Errno = 33
	return errors.Is(err, errorLockViolation)
}
