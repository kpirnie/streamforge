// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kpirnie/streamforge/internal/media"
)

var configDir string

func initializeConfigDir() error {
	config, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locate user configuration directory: %w", err)
	}
	configDir = filepath.Join(config, "streamforge")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	return nil
}

func initializePaths(settings map[string]string) error {
	work := settingValue("STREAMFORGE_WORK_DIR", "", settings)
	if work == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("locate user home directory: %w", err)
		}
		work = filepath.Join(home, "StreamForge")
	}
	if !filepath.IsAbs(work) {
		return errors.New("STREAMFORGE_WORK_DIR must be an absolute path")
	}
	work = filepath.Clean(work)
	if err := os.MkdirAll(work, 0700); err != nil {
		return fmt.Errorf("create media workspace: %w", err)
	}
	root, err := os.OpenRoot(work)
	if err != nil {
		return fmt.Errorf("open media workspace: %w", err)
	}
	defer root.Close()
	for _, tree := range []string{"input", "output"} {
		if err := root.Mkdir(tree, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create media %s directory: %w", tree, err)
		}
		if info, err := root.Lstat(tree); err != nil || !info.IsDir() {
			return fmt.Errorf("media %s must be a real directory", tree)
		}
	}
	media.Working = work
	return nil
}

func applicationDirectories() []string {
	return []string{configDir, media.Working, filepath.Join(media.Working, "input"), filepath.Join(media.Working, "output")}
}

func mediaExecutable(value func(string, string) string, name string) string {
	if configured := value("STREAMFORGE_"+strings.ToUpper(name), ""); configured != "" {
		return configured
	}
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return name
}
