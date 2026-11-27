// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
	getSecurityInfo    = syscall.NewLazyDLL("advapi32.dll").NewProc("GetSecurityInfo")
	equalSid           = syscall.NewLazyDLL("advapi32.dll").NewProc("EqualSid")
)

func openMediaFile(root *os.Root, name string, _ bool) (*os.File, error) {
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &info); err != nil {
		file.Close()
		return nil, err
	}
	if info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		file.Close()
		return nil, &os.PathError{Op: "open", Path: name, Err: syscall.ELOOP}
	}
	return file, nil
}

func preserveOwnership(_ *os.File, _ os.FileInfo) error {
	// New files inherit the destination directory's Windows security descriptor.
	return nil
}

func temporaryCandidate(_ os.FileInfo) bool {
	return true
}

func ownsTemporary(file *os.File, _ os.FileInfo) bool {
	const (
		seFileObject             = 1
		ownerSecurityInformation = 1
	)
	var owner *syscall.SID
	var descriptor syscall.Handle
	result, _, _ := getSecurityInfo.Call(file.Fd(), seFileObject, ownerSecurityInformation, uintptr(unsafe.Pointer(&owner)), 0, 0, 0, uintptr(unsafe.Pointer(&descriptor)))
	runtime.KeepAlive(file)
	if result != 0 {
		return false
	}
	defer syscall.LocalFree(descriptor)
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		return false
	}
	var token syscall.Token
	if err := syscall.OpenProcessToken(process, syscall.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil || owner == nil {
		return false
	}
	match, _, _ := equalSid.Call(uintptr(unsafe.Pointer(owner)), uintptr(unsafe.Pointer(user.User.Sid)))
	runtime.KeepAlive(user)
	return match != 0
}

func destinationSpace(dir string) (uint64, error) {
	const (
		fileAddFile         = 0x0002
		fileAddSubdirectory = 0x0004
	)
	name, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	handle, err := syscall.CreateFile(name, fileAddFile|fileAddSubdirectory, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return 0, errors.New("media destination is not writable")
	}
	syscall.CloseHandle(handle)
	var available uint64
	result, _, err := getDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&available)), 0, 0)
	if result == 0 {
		return 0, errors.New("could not check media destination space")
	}
	return available, nil
}
