// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package app

import (
	"context"
	"os"
	"syscall"
	"unicode"
	"unicode/utf16"
	"unsafe"
)

var (
	consoleKernel    = syscall.NewLazyDLL("kernel32.dll")
	setConsoleMode   = consoleKernel.NewProc("SetConsoleMode")
	getConsoleSize   = consoleKernel.NewProc("GetConsoleScreenBufferInfo")
	readConsoleInput = consoleKernel.NewProc("ReadConsoleInputW")
)

func terminalAvailable(file *os.File) bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(file.Fd()), &mode) == nil
}

func terminalDimensions() (int, int) {
	var info struct {
		columns, rows            int16
		cursorX, cursorY         int16
		attributes               uint16
		left, top, right, bottom int16
		maxColumns, maxRows      int16
	}
	result, _, _ := getConsoleSize.Call(os.Stdout.Fd(), uintptr(unsafe.Pointer(&info)))
	if result == 0 || info.right < info.left || info.bottom < info.top {
		return 80, 24
	}
	return int(info.right) - int(info.left) + 1, int(info.bottom) - int(info.top) + 1
}

func terminalMode() (func(), error) {
	const (
		lineInput             = 0x2
		echoInput             = 0x4
		quickEdit             = 0x40
		extendedFlags         = 0x80
		virtualTerminalOutput = 0x4
	)
	input, output := os.Stdin.Fd(), os.Stdout.Fd()
	var inputMode, outputMode uint32
	if err := syscall.GetConsoleMode(syscall.Handle(input), &inputMode); err != nil {
		return nil, err
	}
	if err := syscall.GetConsoleMode(syscall.Handle(output), &outputMode); err != nil {
		return nil, err
	}
	result, _, err := setConsoleMode.Call(input, uintptr(inputMode&^(lineInput|echoInput|quickEdit)|extendedFlags))
	if result == 0 {
		return nil, err
	}
	result, _, err = setConsoleMode.Call(output, uintptr(outputMode|virtualTerminalOutput))
	if result == 0 {
		setConsoleMode.Call(input, uintptr(inputMode))
		return nil, err
	}
	return func() {
		setConsoleMode.Call(output, uintptr(outputMode))
		setConsoleMode.Call(input, uintptr(inputMode))
	}, nil
}

type consoleInputRecord struct {
	event                                   uint16
	_                                       uint16
	down                                    int32
	repeat, virtualKey, scanCode, character uint16
	control                                 uint32
}

func terminalInput(ctx context.Context, file *os.File) <-chan string {
	keys := make(chan string, 32)
	go func() {
		defer close(keys)
		handle := syscall.Handle(file.Fd())
		var surrogate rune
		for ctx.Err() == nil {
			state, err := syscall.WaitForSingleObject(handle, 100)
			if err != nil {
				return
			}
			if state != syscall.WAIT_OBJECT_0 {
				continue
			}
			var input consoleInputRecord
			var count uint32
			result, _, _ := readConsoleInput.Call(uintptr(handle), uintptr(unsafe.Pointer(&input)), 1, uintptr(unsafe.Pointer(&count)))
			if result == 0 {
				return
			}
			if count == 0 || input.event != 1 || input.down == 0 {
				continue
			}
			var key string
			switch input.virtualKey {
			case 0x08:
				key = "backspace"
			case 0x0d:
				key = "enter"
			case 0x1b:
				key = "escape"
			case 0x26:
				key = "up"
			case 0x28:
				key = "down"
			default:
				character := rune(input.character)
				if character >= 0xd800 && character <= 0xdbff {
					surrogate = character
					continue
				}
				if surrogate != 0 {
					character, surrogate = utf16.DecodeRune(surrogate, character), 0
				}
				if unicode.IsPrint(character) {
					key = "text:" + string(character)
				}
			}
			if key == "" {
				continue
			}
			for range max(1, input.repeat) {
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
