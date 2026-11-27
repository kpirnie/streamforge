// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/version"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/kpirnie/streamforge/internal/filelock"
)

const (
	latestReleaseURL   = "https://api.github.com/repos/kpirnie/streamforge/releases/latest"
	releaseDownloadURL = "https://github.com/kpirnie/streamforge/releases/download/"
	maxUpdateBytes     = 128 << 20
)

var stableTag = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type availableUpdate struct {
	tag, asset, digest string
	size               int64
	executable         string
	original           os.FileInfo
}

func checkForUpdate(ctx context.Context, current string) *availableUpdate {
	if !stableTag.MatchString(current) {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return nil
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil
	}
	installed, err := os.Open(executable)
	if err != nil {
		return nil
	}
	original, err := installed.Stat()
	installed.Close()
	if err != nil || !original.Mode().IsRegular() {
		return nil
	}
	body, err := updateDownload(ctx, latestReleaseURL, 3*time.Second)
	if err != nil {
		return nil
	}
	defer body.Close()
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
			Size   int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 1<<20)).Decode(&release); err != nil {
		return nil
	}
	if release.Draft || release.Prerelease || !stableTag.MatchString(release.Tag) ||
		version.Compare("go"+release.Tag[1:], "go"+current[1:]) <= 0 {
		return nil
	}
	assetName := "streamforge-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	for _, asset := range release.Assets {
		if asset.Name != assetName || asset.Size <= 0 || asset.Size > maxUpdateBytes {
			continue
		}
		digest, ok := strings.CutPrefix(asset.Digest, "sha256:")
		decoded, err := hex.DecodeString(digest)
		if !ok || err != nil || len(decoded) != sha256.Size {
			return nil
		}
		return &availableUpdate{
			tag: release.Tag, asset: assetName, digest: strings.ToLower(digest), size: asset.Size,
			executable: executable, original: original,
		}
	}
	return nil
}

func updateDownload(ctx context.Context, address string, timeout time.Duration) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "StreamForge")
	client := &http.Client{Timeout: timeout, CheckRedirect: func(req *http.Request, previous []*http.Request) error {
		if req.URL.Scheme != "https" || len(previous) >= 5 {
			return errors.New("invalid release download redirect")
		}
		return nil
	}}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("GitHub returned HTTP %d", response.StatusCode)
	}
	return response.Body, nil
}

func (update *availableUpdate) install(ctx context.Context) (bool, error) {
	file, err := os.CreateTemp(filepath.Dir(update.executable), ".streamforge-update-*")
	if err != nil {
		return false, fmt.Errorf("stage update next to the installed binary: %w", err)
	}
	defer file.Close()
	queued := false
	defer func() {
		if !queued {
			os.Remove(file.Name())
		}
	}()
	body, err := updateDownload(ctx, releaseDownloadURL+update.tag+"/"+update.asset, 2*time.Minute)
	if err != nil {
		return false, err
	}
	defer body.Close()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(body, update.size+1))
	if err != nil {
		return false, err
	}
	if size != update.size || hex.EncodeToString(hash.Sum(nil)) != update.digest {
		return false, errors.New("the downloaded binary did not match the release checksum")
	}
	if err := file.Chmod(update.original.Mode().Perm()); err != nil {
		return false, err
	}
	if err := file.Sync(); err != nil {
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	installed, err := os.Open(update.executable)
	if err != nil {
		return false, err
	}
	defer installed.Close()
	if err := filelock.Exclusive(installed, false); err != nil {
		return false, fmt.Errorf("lock installed binary for update: %w", err)
	}
	// Reject an executable replaced while this download was in progress.
	current, err := os.Lstat(update.executable)
	if err != nil {
		return false, err
	}
	if !os.SameFile(current, update.original) || current.Size() != update.original.Size() || !current.ModTime().Equal(update.original.ModTime()) {
		return false, errors.New("the installed executable changed. Restart StreamForge to check again")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	queued, err = applyUpdate(file.Name(), update.executable, current)
	return queued, err
}
