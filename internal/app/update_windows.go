// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package app

import (
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"unicode/utf16"
)

func applyUpdate(staged, installed string, info os.FileInfo) (bool, error) {
	system := os.Getenv("SystemRoot")
	if !filepath.IsAbs(system) {
		return false, errors.New("Windows system directory is unavailable")
	}
	// Windows keeps the running image open. Replace it after this process exits.
	const script = `
$ErrorActionPreference = 'Stop'
try {
    $parent = Get-Process -Id ([int]$env:STREAMFORGE_UPDATE_PID) -ErrorAction SilentlyContinue
    if ($parent -and !$parent.WaitForExit(15000)) { throw 'StreamForge is still running.' }
    $share = [IO.FileShare]::ReadWrite -bor [IO.FileShare]::Delete
    $lock = [IO.File]::Open($env:STREAMFORGE_UPDATE_TARGET, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, $share)
    try {
        $lock.Lock(([long]::MaxValue - 1), 1)
        $installed = Get-Item -LiteralPath $env:STREAMFORGE_UPDATE_TARGET -Force
        if (($installed.Attributes -band [IO.FileAttributes]::ReparsePoint) -or
            $installed.Length -ne [long]$env:STREAMFORGE_UPDATE_SIZE -or
            $installed.LastWriteTimeUtc.ToFileTimeUtc() -ne [long]$env:STREAMFORGE_UPDATE_MTIME) {
            throw 'The installed executable changed. Restart StreamForge to check again.'
        }
        [IO.File]::Replace($env:STREAMFORGE_UPDATE_SOURCE, $env:STREAMFORGE_UPDATE_TARGET, [NullString]::Value)
    } finally {
        $lock.Dispose()
    }
    Write-Host 'StreamForge updated. Launch it again to use the new version.'
} catch {
    [Console]::Error.WriteLine("StreamForge update failed: " + $_.Exception.Message)
    exit 1
} finally {
    Remove-Item -LiteralPath $env:STREAMFORGE_UPDATE_SOURCE -Force -ErrorAction SilentlyContinue
}
`
	units := utf16.Encode([]rune(script))
	encoded := make([]byte, len(units)*2)
	for i, unit := range units {
		encoded[2*i], encoded[2*i+1] = byte(unit), byte(unit>>8)
	}
	powershell := filepath.Join(system, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	command := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	command.Env = append(os.Environ(),
		"STREAMFORGE_UPDATE_PID="+strconv.Itoa(os.Getpid()),
		"STREAMFORGE_UPDATE_SOURCE="+staged,
		"STREAMFORGE_UPDATE_TARGET="+installed,
		"STREAMFORGE_UPDATE_SIZE="+strconv.FormatInt(info.Size(), 10),
		"STREAMFORGE_UPDATE_MTIME="+strconv.FormatInt(info.ModTime().UnixNano()/100+116444736000000000, 10),
	)
	if err := command.Start(); err != nil {
		return false, err
	}
	_ = command.Process.Release()
	return true, nil
}
