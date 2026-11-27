// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

//go:build darwin || windows

package media

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
)

const mediaProtocols = "file,pipe,fd"

func mediaFileArgs(slot int, option ...string) []string {
	descriptor := slot + 3
	if runtime.GOOS == "windows" {
		descriptor = slot
	}
	args := []string{"-fd", strconv.Itoa(descriptor)}
	args = append(args, option...)
	return append(args, "fd:")
}

func configureMediaFiles(command *exec.Cmd, source, output, subtitle *os.File) error {
	// Duplicated handles share their file position with the parent.
	for _, file := range []*os.File{source, output, subtitle} {
		if file != nil {
			if _, err := file.Seek(0, 0); err != nil {
				return fmt.Errorf("rewind media file: %w", err)
			}
		}
	}
	if runtime.GOOS == "windows" {
		command.Stdin = source
		if output != nil {
			command.Stdout = output
		}
		if subtitle != nil {
			// Windows can inherit the third file in the stderr slot. FFmpeg
			// opens it read-only via the fd protocol.
			command.Stderr = subtitle
		}
		return nil
	}
	command.ExtraFiles = []*os.File{source}
	if output != nil {
		command.ExtraFiles = append(command.ExtraFiles, output)
	}
	if subtitle != nil {
		command.ExtraFiles = append(command.ExtraFiles, subtitle)
	}
	return nil
}
