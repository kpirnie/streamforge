// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: go run ./internal/winicon ARCH OUTPUT.syso")
	}
	var machine, relocation uint16
	switch args[0] {
	case "386":
		machine, relocation = pe.IMAGE_FILE_MACHINE_I386, 7
	case "amd64":
		machine, relocation = pe.IMAGE_FILE_MACHINE_AMD64, 3
	case "arm64":
		machine, relocation = pe.IMAGE_FILE_MACHINE_ARM64, 2
	default:
		return fmt.Errorf("unsupported Windows architecture %q", args[0])
	}
	file, err := os.Open("docs/assets/streamforge-app-icon.png")
	if err != nil {
		return err
	}
	defer file.Close()
	source, err := png.Decode(file)
	if err != nil {
		return err
	}
	// The Windows icon directory encodes 256 pixels as zero.
	icon := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	bounds := source.Bounds()
	for y := range 256 {
		for x := range 256 {
			icon.Set(x, y, source.At(bounds.Min.X+x*bounds.Dx()/256, bounds.Min.Y+y*bounds.Dy()/256))
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, icon); err != nil {
		return err
	}
	resource, offsets := iconResource(encoded.Bytes())
	return os.WriteFile(args[1], coffResource(machine, relocation, resource, offsets), 0600)
}

func iconResource(icon []byte) ([]byte, [2]uint32) {
	const directoryBit = 1 << 31
	u16, u32 := binary.LittleEndian.PutUint16, binary.LittleEndian.PutUint32
	data := make([]byte, 160)
	// Each resource has a type, identifier and language directory.
	u16(data[14:], 2)
	u32(data[16:], 3) // RT_ICON
	u32(data[20:], directoryBit|32)
	u32(data[24:], 14) // RT_GROUP_ICON
	u32(data[28:], directoryBit|80)
	for _, start := range []int{32, 80} {
		u16(data[start+14:], 1)
		u32(data[start+16:], 1)
		u32(data[start+20:], directoryBit|uint32(start+24))
		u16(data[start+38:], 1)
		u32(data[start+40:], 0x409) // English (United States)
	}
	u32(data[76:], 128)
	u32(data[124:], 144)

	u32(data[128:], uint32(len(data)))
	u32(data[132:], uint32(len(icon)))
	data = append(data, icon...)
	for len(data)%4 != 0 {
		data = append(data, 0)
	}
	u32(data[144:], uint32(len(data)))
	u32(data[148:], 20)

	// The group icon advertises one PNG image stored as RT_ICON resource 1.
	var group [20]byte
	u16(group[2:], 1)
	u16(group[4:], 1)
	u16(group[10:], 1)
	u16(group[12:], 32)
	u32(group[14:], uint32(len(icon)))
	u16(group[18:], 1)
	return append(data, group[:]...), [2]uint32{128, 144}
}

func coffResource(machine, relocation uint16, resource []byte, offsets [2]uint32) []byte {
	const headerSize = 20 + 40
	u16, u32 := binary.LittleEndian.PutUint16, binary.LittleEndian.PutUint32
	data := make([]byte, headerSize)
	relocations := headerSize + len(resource)
	symbols := relocations + len(offsets)*10
	u16(data[0:], machine)
	u16(data[2:], 1)
	u32(data[8:], uint32(symbols))
	u32(data[12:], 1)
	copy(data[20:], ".rsrc")
	u32(data[36:], uint32(len(resource)))
	u32(data[40:], headerSize)
	u32(data[44:], uint32(relocations))
	u16(data[52:], uint16(len(offsets)))
	u32(data[56:], pe.IMAGE_SCN_CNT_INITIALIZED_DATA|pe.IMAGE_SCN_MEM_READ)
	data = append(data, resource...)
	for _, offset := range offsets {
		var entry [10]byte
		u32(entry[0:], offset)
		u16(entry[8:], relocation)
		data = append(data, entry[:]...)
	}
	var symbol [18]byte
	copy(symbol[:], ".rsrc")
	u16(symbol[12:], 1)
	symbol[16] = 3 // Static section symbol.
	data = append(data, symbol[:]...)
	return append(data, 4, 0, 0, 0) // Empty COFF string table.
}
