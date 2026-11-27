// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

// WithCPUCap rejects CPU budgets the macOS scheduler cannot enforce.
func WithCPUCap(ctx context.Context, limit int) (context.Context, func(), error) {
	available := runtime.NumCPU()
	if limit < available {
		return nil, nil, fmt.Errorf("macOS cannot enforce a hard CPU cap below the host's %d logical CPUs; set --jobs=%d to use the host capacity", available, available)
	}
	return context.WithValue(ctx, cpuLimitKey{}, true), func() {}, nil
}

func mediaCommand(ctx context.Context, binary string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, binary, args...)
	if ctx.Value(cpuLimitKey{}) != nil {
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		command.Cancel = func() error {
			err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
			if err == syscall.ESRCH {
				return os.ErrProcessDone
			}
			return err
		}
	}
	return command
}

// RunCPULaunch is used by platforms that re-execute the main binary.
func RunCPULaunch(_ []string) {}
