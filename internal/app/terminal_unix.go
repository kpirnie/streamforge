// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

//go:build linux || darwin

package app

import (
	"context"
	"os"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

func terminalAvailable(file *os.File) bool {
	var settings syscall.Termios
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), getTermios, uintptr(unsafe.Pointer(&settings)))
	return err == 0
}

func terminalDimensions() (int, int) {
	var size struct{ Rows, Columns, X, Y uint16 }
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&size)))
	if err != 0 || size.Columns == 0 || size.Rows == 0 {
		return 80, 24
	}
	return int(size.Columns), int(size.Rows)
}

func terminalInput(ctx context.Context, file *os.File) <-chan string {
	keys := make(chan string, 32)
	go func() {
		defer close(keys)
		buffer := make([]byte, 128)
		pending := ""
		for ctx.Err() == nil {
			count, err := syscall.Read(int(file.Fd()), buffer)
			if err != nil && err != syscall.EINTR {
				return
			}
			if count == 0 && pending == "" {
				continue
			}
			if count > 0 {
				pending += string(buffer[:count])
			}
			for pending != "" {
				size := 1
				var key string
				switch {
				case strings.HasPrefix(pending, "\x1b[A"):
					key, size = "up", 3
				case strings.HasPrefix(pending, "\x1b[B"):
					key, size = "down", 3
				case strings.HasPrefix(pending, "\x1b[") && len(pending) >= 3:
					key, size = "", 3
				case pending[0] == '\x1b':
					if count > 0 && (len(pending) == 1 || pending == "\x1b[") {
						break
					}
					key = "escape"
				case pending[0] == '\r' || pending[0] == '\n':
					key = "enter"
				case pending[0] == 127 || pending[0] == 8:
					key = "backspace"
				default:
					r, n := utf8.DecodeRuneInString(pending)
					if count > 0 && !utf8.FullRuneInString(pending) {
						break
					}
					if n == 0 && count > 0 {
						break
					}
					size = max(1, n)
					if unicode.IsPrint(r) {
						key = "text:" + string(r)
					}
				}
				if key == "" && (pending == "\x1b" || pending == "\x1b[") {
					break
				}
				pending = pending[size:]
				if key == "" {
					continue
				}
				select {
				case keys <- key:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return keys
}

func terminalMode() (func(), error) {
	fd := os.Stdin.Fd()
	var saved syscall.Termios
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, getTermios, uintptr(unsafe.Pointer(&saved))); err != 0 {
		return nil, err
	}
	raw := saved
	raw.Lflag &^= syscall.ECHO | syscall.ICANON
	raw.Cc[syscall.VMIN], raw.Cc[syscall.VTIME] = 0, 1
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, setTermios, uintptr(unsafe.Pointer(&raw))); err != 0 {
		return nil, err
	}
	return func() { syscall.Syscall(syscall.SYS_IOCTL, fd, setTermios, uintptr(unsafe.Pointer(&saved))) }, nil
}
