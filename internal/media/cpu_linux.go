// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"
)

// WithCPUCap selects a shared set of allowed logical CPUs for the batch's media commands.
func WithCPUCap(ctx context.Context, limit int) (context.Context, func(), error) {
	mask, err := currentCPUMask()
	if err != nil {
		return nil, nil, fmt.Errorf("read allowed CPUs: %w", err)
	}
	for index, allowed := range mask {
		for bit := byte(1); bit != 0; bit <<= 1 {
			if allowed&bit == 0 {
				continue
			}
			if limit > 0 {
				limit--
			} else {
				mask[index] &^= bit
			}
		}
	}
	return context.WithValue(ctx, cpuLimitKey{}, hex.EncodeToString(mask)), func() {}, nil
}

func currentCPUMask() ([]byte, error) {
	for size := 128; ; size *= 2 {
		mask := make([]byte, size)
		count, _, errno := syscall.RawSyscall(syscall.SYS_SCHED_GETAFFINITY, 0, uintptr(size), uintptr(unsafe.Pointer(&mask[0])))
		if errno == 0 {
			return mask[:count], nil
		}
		if errno != syscall.EINVAL {
			return nil, errno
		}
	}
}

func mediaCommand(ctx context.Context, binary string, args ...string) *exec.Cmd {
	mask, limited := ctx.Value(cpuLimitKey{}).(string)
	if !limited {
		return exec.CommandContext(ctx, binary, args...)
	}
	command := exec.CommandContext(ctx, "/proc/self/exe", append([]string{cpuLaunchMode, mask, binary}, args...)...)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	return command
}

// RunCPULaunch handles the private entry point that applies CPU affinity before exec.
func RunCPULaunch(args []string) {
	if len(args) == 0 || args[0] != cpuLaunchMode {
		return
	}
	runtime.LockOSThread()
	err := launchWithCPUCap(args[1:])
	fmt.Fprintf(os.Stderr, "%s%v\n", cpuLaunchErrorPrefix, err)
	os.Exit(cpuLaunchFailure)
}

func launchWithCPUCap(args []string) error {
	if len(args) < 2 {
		return errCPUEnforcement
	}
	selected, err := hex.DecodeString(args[0])
	if err != nil {
		return err
	}
	mask, err := currentCPUMask()
	if err != nil {
		return fmt.Errorf("read inherited CPU affinity: %w", err)
	}
	for index := range mask {
		if index < len(selected) {
			mask[index] &= selected[index]
		} else {
			mask[index] = 0
		}
	}
	_, _, errno := syscall.RawSyscall(syscall.SYS_SCHED_SETAFFINITY, 0, uintptr(len(mask)), uintptr(unsafe.Pointer(&mask[0])))
	if errno != 0 {
		return fmt.Errorf("set media CPU affinity: %w", errno)
	}
	if err := lockCPUAffinity(); err != nil {
		return fmt.Errorf("lock media CPU affinity: %w", err)
	}
	return syscall.Exec(args[1], args[1:], os.Environ())
}

func lockCPUAffinity() error {
	const (
		prSetNoNewPrivs   = 38
		prSetSeccomp      = 22
		seccompModeFilter = 2
		seccompRetErrno   = 0x00050000
		seccompRetAllow   = 0x7fff0000
	)
	// Encoder libraries can change their own affinity. Prevent them from
	// expanding the inherited CPU set in the executable or any descendant.
	filter := [...]syscall.SockFilter{
		{Code: syscall.BPF_LD | syscall.BPF_W | syscall.BPF_ABS},
		{Code: syscall.BPF_JMP | syscall.BPF_JEQ | syscall.BPF_K, Jf: 1, K: syscall.SYS_SCHED_SETAFFINITY},
		{Code: syscall.BPF_RET | syscall.BPF_K, K: seccompRetErrno | uint32(syscall.EPERM)},
		{Code: syscall.BPF_RET | syscall.BPF_K, K: seccompRetAllow},
	}
	program := syscall.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	_, _, errno := syscall.RawSyscall6(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0, 0, 0, 0)
	if errno != 0 {
		return errno
	}
	_, _, errno = syscall.RawSyscall6(syscall.SYS_PRCTL, prSetSeccomp, seccompModeFilter, uintptr(unsafe.Pointer(&program)), 0, 0, 0)
	runtime.KeepAlive(filter)
	if errno != 0 {
		return errno
	}
	return nil
}
