// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

var (
	ntdll     = syscall.NewLazyDLL("ntdll.dll")
	ntQueryEA = ntdll.NewProc("NtQueryEaFile")
	ntSetEA   = ntdll.NewProc("NtSetEaFile")
)

const (
	statusEAsNotSupported = 0xc000004f
	statusNonexistentEA   = 0xc0000051
	statusNoEAsOnFile     = 0xc0000052
	statusNotSupported    = 0xc00000bb
)

func readSource(file *os.File, value []byte) (int, error) {
	// FILE_GET_EA_INFORMATION selects the attribute on the already-open file.
	var request [5 + len(sourceAttribute) + 1]byte
	request[4] = byte(len(sourceAttribute))
	copy(request[5:], sourceAttribute)
	var data [256]byte
	var ioStatus [2]uintptr
	result, _, _ := ntQueryEA.Call(file.Fd(), uintptr(unsafe.Pointer(&ioStatus[0])), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 1, uintptr(unsafe.Pointer(&request[0])), uintptr(len(request)), 0, 1)
	runtime.KeepAlive(file)
	switch uint32(result) {
	case 0:
		nameLength := int(data[5])
		valueLength := int(binary.LittleEndian.Uint16(data[6:8]))
		offset := 8 + nameLength + 1
		if valueLength != len(value) || offset+valueLength > len(data) || uintptr(offset+valueLength) > ioStatus[1] {
			return 0, nil
		}
		if !strings.EqualFold(string(data[8:offset-1]), sourceAttribute) {
			return 0, nil
		}
		return copy(value, data[offset:offset+valueLength]), nil
	case statusEAsNotSupported, statusNonexistentEA, statusNoEAsOnFile, statusNotSupported:
		return 0, nil
	default:
		return 0, fmt.Errorf("read media ownership attribute: NTSTATUS %#x", uint32(result))
	}
}

func markSource(file *os.File, source string) error {
	// FILE_FULL_EA_INFORMATION follows the same layout queried above.
	const header = 8
	value := fingerprint(source)
	data := make([]byte, header+len(sourceAttribute)+1+len(value))
	data[5] = byte(len(sourceAttribute))
	binary.LittleEndian.PutUint16(data[6:8], uint16(len(value)))
	copy(data[header:], sourceAttribute)
	copy(data[header+len(sourceAttribute)+1:], value)
	var ioStatus [2]uintptr
	result, _, _ := ntSetEA.Call(file.Fd(), uintptr(unsafe.Pointer(&ioStatus[0])), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	runtime.KeepAlive(file)
	switch uint32(result) {
	case 0, statusEAsNotSupported, statusNotSupported:
		return nil
	default:
		return fmt.Errorf("mark media ownership: NTSTATUS %#x", uint32(result))
	}
}
