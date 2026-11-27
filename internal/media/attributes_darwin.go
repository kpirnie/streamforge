// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

func readSource(file *os.File, value []byte) (int, error) {
	name, err := syscall.BytePtrFromString(sourceAttribute)
	if err != nil {
		return 0, err
	}
	size, _, errno := syscall.Syscall6(syscall.SYS_FGETXATTR, file.Fd(), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&value[0])), uintptr(len(value)), 0, 0)
	runtime.KeepAlive(file)
	switch errno {
	case 0:
		return int(size), nil
	case syscall.ENOATTR, syscall.ENOTSUP, syscall.ERANGE:
		return 0, nil
	default:
		return 0, errno
	}
}

func markSource(file *os.File, source string) error {
	name, err := syscall.BytePtrFromString(sourceAttribute)
	if err != nil {
		return err
	}
	value := []byte(fingerprint(source))
	_, _, errno := syscall.Syscall6(syscall.SYS_FSETXATTR, file.Fd(), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&value[0])), uintptr(len(value)), 0, 0)
	runtime.KeepAlive(file)
	if errno != 0 && errno != syscall.ENOTSUP {
		return errno
	}
	return nil
}
