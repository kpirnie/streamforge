// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"fmt"
	"os"
	"os/exec"
)

const mediaProtocols = "file,pipe"

func mediaFileArgs(slot int, option ...string) []string {
	return append(option, fmt.Sprintf("/proc/self/fd/%d", slot+3))
}

func configureMediaFiles(command *exec.Cmd, source, output, subtitle *os.File) error {
	command.ExtraFiles = []*os.File{source}
	if output != nil {
		command.ExtraFiles = append(command.ExtraFiles, output)
	}
	if subtitle != nil {
		command.ExtraFiles = append(command.ExtraFiles, subtitle)
	}
	return nil
}
