// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"
)

var (
	createJob                    = kernel32.NewProc("CreateJobObjectW")
	setJobInformation            = kernel32.NewProc("SetInformationJobObject")
	assignProcessToJob           = kernel32.NewProc("AssignProcessToJobObject")
	getActiveProcessorCount      = kernel32.NewProc("GetActiveProcessorCount")
	getActiveProcessorGroupCount = kernel32.NewProc("GetActiveProcessorGroupCount")
	getProcessAffinityMask       = kernel32.NewProc("GetProcessAffinityMask")
)

type windowsCPUCap struct {
	executable string
	job        syscall.Handle
	affinity   uintptr
}

// WithCPUCap creates a Windows job that enforces the shared CPU budget.
func WithCPUCap(ctx context.Context, limit int) (context.Context, func(), error) {
	const (
		allProcessorGroups    = 0xffff
		jobCPURateInformation = 15
		jobCPUEnable          = 0x1
		jobCPUHardCap         = 0x4
	)
	total, _, err := getActiveProcessorCount.Call(allProcessorGroups)
	if total == 0 {
		return nil, nil, fmt.Errorf("read logical processor count: %w", err)
	}
	executable, execErr := os.Executable()
	if execErr != nil {
		return nil, nil, execErr
	}
	job, jobErr := newWindowsJob(0)
	if jobErr != nil {
		return nil, nil, jobErr
	}
	rate := uint32(uint64(min(limit, int(total))) * 10000 / uint64(total))
	settings := [2]uint32{jobCPUEnable | jobCPUHardCap, rate}
	result, _, err := setJobInformation.Call(uintptr(job), jobCPURateInformation, uintptr(unsafe.Pointer(&settings[0])), unsafe.Sizeof(settings))
	var affinity uintptr
	if result == 0 {
		syscall.CloseHandle(job)
		affinity, jobErr = chooseWindowsAffinity(limit)
		if jobErr == nil {
			job, jobErr = newWindowsJob(affinity)
		}
		if jobErr != nil {
			return nil, nil, errors.Join(fmt.Errorf("set media CPU rate: %w", err), jobErr)
		}
	}
	if err := syscall.SetHandleInformation(job, syscall.HANDLE_FLAG_INHERIT, syscall.HANDLE_FLAG_INHERIT); err != nil {
		syscall.CloseHandle(job)
		return nil, nil, err
	}
	cap := windowsCPUCap{executable: executable, job: job, affinity: affinity}
	return context.WithValue(ctx, cpuLimitKey{}, cap), func() { syscall.CloseHandle(job) }, nil
}

func newWindowsJob(affinity uintptr) (syscall.Handle, error) {
	const (
		jobExtendedLimitInformation = 9
		killOnJobClose              = 0x2000
		jobLimitAffinity            = 0x10
	)
	job, _, err := createJob.Call(0, 0)
	if job == 0 {
		return 0, fmt.Errorf("create media job: %w", err)
	}
	// JOBOBJECT_EXTENDED_LIMIT_INFORMATION varies with pointer width.
	info := make([]byte, 80+8*int(unsafe.Sizeof(uintptr(0))))
	flags := uint32(killOnJobClose)
	if affinity != 0 {
		flags |= jobLimitAffinity
		if strconv.IntSize == 64 {
			binary.LittleEndian.PutUint64(info[48:], uint64(affinity))
		} else {
			binary.LittleEndian.PutUint32(info[32:], uint32(affinity))
		}
	}
	binary.LittleEndian.PutUint32(info[16:20], flags)
	result, _, err := setJobInformation.Call(job, jobExtendedLimitInformation, uintptr(unsafe.Pointer(&info[0])), uintptr(len(info)))
	if result == 0 {
		syscall.CloseHandle(syscall.Handle(job))
		return 0, fmt.Errorf("configure media job cleanup: %w", err)
	}
	return syscall.Handle(job), nil
}

func chooseWindowsAffinity(limit int) (uintptr, error) {
	groups, _, _ := getActiveProcessorGroupCount.Call()
	if groups != 1 {
		return 0, errors.New("a Windows affinity cap requires a single processor group")
	}
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, err
	}
	allowed, err := windowsAffinity(process)
	if err != nil {
		return 0, err
	}
	var selected uintptr
	for range min(limit, bits.OnesCount(uint(allowed))) {
		cpu := uintptr(1) << bits.TrailingZeros(uint(allowed))
		selected |= cpu
		allowed &^= cpu
	}
	if selected == 0 {
		return 0, errors.New("no CPUs available for the media job")
	}
	return selected, nil
}

func windowsAffinity(process syscall.Handle) (uintptr, error) {
	var allowed, system uintptr
	result, _, err := getProcessAffinityMask.Call(uintptr(process), uintptr(unsafe.Pointer(&allowed)), uintptr(unsafe.Pointer(&system)))
	if result == 0 {
		return 0, fmt.Errorf("read Windows CPU affinity: %w", err)
	}
	return allowed, nil
}

func mediaCommand(ctx context.Context, binary string, args ...string) *exec.Cmd {
	cap, limited := ctx.Value(cpuLimitKey{}).(windowsCPUCap)
	if !limited {
		return exec.CommandContext(ctx, binary, args...)
	}
	command := exec.CommandContext(ctx, cap.executable, append([]string{cpuLaunchMode, strconv.FormatUint(uint64(cap.job), 10), strconv.FormatUint(uint64(cap.affinity), 10), binary}, args...)...)
	command.SysProcAttr = &syscall.SysProcAttr{AdditionalInheritedHandles: []syscall.Handle{cap.job}}
	return command
}

// RunCPULaunch assigns the launcher and its descendants to the shared CPU job.
func RunCPULaunch(args []string) {
	if len(args) == 0 || args[0] != cpuLaunchMode {
		return
	}
	code, err := launchWindowsMedia(args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s%v\n", cpuLaunchErrorPrefix, err)
	}
	os.Exit(code)
}

func launchWindowsMedia(args []string) (int, error) {
	if len(args) < 3 {
		return cpuLaunchFailure, errCPUEnforcement
	}
	value, err := strconv.ParseUint(args[0], 10, strconv.IntSize)
	if err != nil {
		return cpuLaunchFailure, err
	}
	affinity, err := strconv.ParseUint(args[1], 10, strconv.IntSize)
	if err != nil {
		return cpuLaunchFailure, err
	}
	shared := syscall.Handle(value)
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		return cpuLaunchFailure, err
	}
	result, _, err := assignProcessToJob.Call(uintptr(shared), uintptr(process))
	syscall.CloseHandle(shared)
	if result == 0 {
		return cpuLaunchFailure, fmt.Errorf("join media CPU job: %w", err)
	}
	// The launcher's own handle closes on cancellation and kills its descendants.
	job, err := newWindowsJob(0)
	if err != nil {
		return cpuLaunchFailure, err
	}
	result, _, err = assignProcessToJob.Call(uintptr(job), uintptr(process))
	if result == 0 {
		syscall.CloseHandle(job)
		return cpuLaunchFailure, fmt.Errorf("join media cleanup job: %w", err)
	}
	if affinity != 0 {
		allowed, err := windowsAffinity(process)
		if err != nil {
			return cpuLaunchFailure, err
		}
		if allowed == 0 || allowed&^uintptr(affinity) != 0 {
			return cpuLaunchFailure, errors.New("Windows did not apply the media job affinity limit")
		}
	}
	command := exec.Command(args[2], args[3:]...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), nil
		}
		return cpuLaunchFailure, err
	}
	return 0, nil
}
