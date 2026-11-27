// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// Working is the media and operational output directory selected at startup.
var Working string

// Files resolves an input or output media path and enumerates regular media files.
func Files(root *os.Root, target string, exclusions []string) ([]string, error) {
	if filepath.IsAbs(target) {
		var err error
		target, err = filepath.Rel(root.Name(), target)
		if err != nil {
			return nil, err
		}
	} else if target != "input" && target != "output" && !strings.HasPrefix(target, "input/") && !strings.HasPrefix(target, "output/") {
		target = filepath.Join("input", target)
	}
	target = filepath.Clean(target)
	if !filepath.IsLocal(target) || (target != "input" && target != "output" && !strings.HasPrefix(target, "input/") && !strings.HasPrefix(target, "output/")) {
		return nil, fmt.Errorf("media paths must be inside %s or %s", filepath.Join(root.Name(), "input"), filepath.Join(root.Name(), "output"))
	}
	for _, pattern := range exclusions {
		if _, err := filepath.Match(pattern, ""); err != nil {
			return nil, errors.New("invalid media exclusion glob")
		}
	}
	for path := target; path != "."; path = filepath.Dir(path) {
		entry, err := root.Lstat(path)
		if err != nil {
			return nil, err
		}
		if entry.Mode()&os.ModeSymlink != 0 || excluded(entry.Name(), exclusions) {
			return nil, errors.New("media path is hidden, excluded or symbolic")
		}
	}
	extensions := []string{".avi", ".flv", ".m2ts", ".m4v", ".mkv", ".mov", ".mp4", ".mpeg", ".mpg", ".ts", ".vob", ".webm", ".wmv"}
	var files []string
	err := fs.WalkDir(root.FS(), target, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if excluded(entry.Name(), exclusions) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() && slices.Contains(extensions, strings.ToLower(filepath.Ext(path))) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func excluded(name string, patterns []string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(strings.ToLower(pattern), strings.ToLower(name)); matched {
			return true
		}
	}
	return false
}

// WriteSTRM writes a companion for an existing media file using the player's mount root.
func WriteSTRM(ctx context.Context, root *os.Root, mediaPath, source, parent string, titles *Titles, dryRun bool) (string, error) {
	return WriteSTRMIn(ctx, root, mediaPath, source, parent, "", titles, dryRun)
}

// WriteSTRMIn writes a companion in a directory relative to the output tree.
func WriteSTRMIn(ctx context.Context, root *os.Root, mediaPath, source, parent, directory string, titles *Titles, dryRun bool) (string, error) {
	if !filepath.IsAbs(parent) || strings.ContainsAny(parent, "\r\n") {
		return "", errors.New("playback parent must be an absolute path")
	}
	tree, relative, ok := strings.Cut(mediaPath, "/")
	if !ok || (tree != "input" && tree != "output") || !filepath.IsLocal(relative) || strings.ContainsAny(relative, "\r\n") {
		return "", errors.New("invalid media location for companion")
	}
	info, err := root.Lstat(mediaPath)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("companion target must be a regular media file")
	}
	if directory == "" {
		directory = filepath.Dir(relative)
	}
	if !filepath.IsLocal(directory) {
		return "", errors.New("invalid output directory for companion")
	}
	destination := filepath.Join("output", directory, titles.basename(ctx, relative, dryRun)+".strm")
	if dryRun {
		return destination, nil
	}
	data := filepath.Join(parent, mediaPath) + "\n"
	if source == "" {
		source = mediaPath
	}
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		target := destination
		exists, owned, err := ownership(root, target, source)
		if err != nil {
			return "", err
		}
		if exists && !owned {
			owned = previousCompanion(root, target, mediaPath) || previousCompanion(root, target, source)
			if !owned {
				target, err = SelectTarget(root, destination, source, nil)
				if err != nil {
					return "", err
				}
				exists, owned, err = ownership(root, target, source)
				if err != nil {
					return "", err
				}
				if exists && !owned {
					continue
				}
			}
		}
		err = AtomicWrite(ctx, root, target, 0666, exists && owned, func(file *os.File) error {
			if _, err := io.WriteString(file, data); err != nil {
				return err
			}
			return markSource(file, source)
		})
		if errors.Is(err, os.ErrExist) && !exists {
			continue
		}
		if err != nil {
			return "", err
		}
		return target, nil
	}
}

// AtomicWrite publishes a completed regular file without replacing it on failure.
func AtomicWrite(ctx context.Context, root *os.Root, destination string, mode fs.FileMode, replace bool, write func(*os.File) error) error {
	if !filepath.IsLocal(destination) || destination == "." {
		return errors.New("invalid output path")
	}
	dir := filepath.Dir(destination)
	// Check each parent before creating anything below it.
	current := "."
	for _, part := range strings.Split(dir, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err = root.Mkdir(current, 0777); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			info, err = root.Lstat(current)
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("output parent must be a real directory")
		}
	}
	if existing, err := root.Lstat(destination); err == nil {
		if !existing.Mode().IsRegular() || !replace {
			return fmt.Errorf("destination already exists: %w", os.ErrExist)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temp := filepath.Join(dir, ".streamforge-"+rand.Text()+filepath.Ext(destination))
	file, err := root.OpenFile(temp, os.O_RDWR|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	err = errors.Join(write(file), ctx.Err())
	if err == nil {
		err = file.Sync()
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if replace {
		return root.Rename(temp, destination)
	}
	return root.Link(temp, destination)
}
