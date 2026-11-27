// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

//go:build linux || darwin

package media

import (
	"errors"
	"os"
	"syscall"
)

func openMediaFile(root *os.Root, name string, nonblock bool) (*os.File, error) {
	flags := os.O_RDONLY | syscall.O_NOFOLLOW
	if nonblock {
		flags |= syscall.O_NONBLOCK
	}
	return root.OpenFile(name, flags, 0)
}

func preserveOwnership(output *os.File, source os.FileInfo) error {
	if stat, ok := source.Sys().(*syscall.Stat_t); ok {
		if err := output.Chown(int(stat.Uid), int(stat.Gid)); err != nil && !errors.Is(err, syscall.EPERM) {
			return err
		}
	}
	return nil
}

func temporaryCandidate(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func ownsTemporary(_ *os.File, info os.FileInfo) bool {
	return temporaryCandidate(info)
}

func destinationSpace(dir string) (uint64, error) {
	if err := syscall.Access(dir, 3); err != nil {
		return 0, errors.New("media destination is not writable")
	}
	var stats syscall.Statfs_t
	if err := syscall.Statfs(dir, &stats); err != nil {
		return 0, errors.New("could not check media destination space")
	}
	return uint64(stats.Bavail) * uint64(stats.Bsize), nil
}
