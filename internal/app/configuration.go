// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "github.com/ncruces/go-sqlite3/driver"

	"github.com/kpirnie/streamforge/internal/xtream"
)

type configuration struct {
	db *sql.DB
}

type configurationKey struct{}

func configurationFor(ctx context.Context) *configuration {
	return ctx.Value(configurationKey{}).(*configuration)
}

func openConfiguration(ctx context.Context, dir string) (*configuration, error) {
	path := filepath.Join(dir, "settings.sqlite3")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	switch {
	case err == nil:
		if err := file.Close(); err != nil {
			return nil, err
		}
	case errors.Is(err, os.ErrExist):
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return nil, statErr
		}
		if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
			return nil, errors.New("configuration database must be a private regular file")
		}
	default:
		return nil, err
	}
	slashPath := filepath.ToSlash(path)
	if runtime.GOOS == "windows" && strings.HasPrefix(slashPath, "//") {
		// Keep the UNC server in the path instead of the URI authority.
		slashPath = "//" + slashPath
	}
	escaped := (&url.URL{Path: slashPath}).EscapedPath()
	db, err := sql.Open("sqlite3", "file:"+escaped+"?_txlock=immediate&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	cfg := &configuration{db: db}
	if err := cfg.importLegacy(ctx, dir); err != nil {
		db.Close()
		return nil, err
	}
	return cfg, nil
}

func (c *configuration) importLegacy(ctx context.Context, dir string) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == 1 {
		return tx.Commit()
	}
	if version != 0 {
		return fmt.Errorf("unsupported configuration database version %d", version)
	}
	settings, err := readLegacyEnvironment(filepath.Join(dir, ".env"))
	if err != nil {
		return err
	}
	providers, err := xtream.ReadProviderRecords(filepath.Join(dir, "providers.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID;
		CREATE TABLE providers (name TEXT PRIMARY KEY, document TEXT NOT NULL CHECK(json_valid(document))) WITHOUT ROWID;
	`); err != nil {
		return err
	}
	for key, value := range settings {
		if _, err := tx.ExecContext(ctx, "INSERT INTO settings VALUES (?, ?)", key, value); err != nil {
			return err
		}
	}
	for name, record := range providers {
		if _, err := tx.ExecContext(ctx, "INSERT INTO providers VALUES (?, ?)", name, string(record)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version = 1"); err != nil {
		return err
	}
	return tx.Commit()
}

func (c *configuration) settings(ctx context.Context) (map[string]string, error) {
	rows, err := c.db.QueryContext(ctx, "SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		settings[key] = value
	}
	return settings, rows.Err()
}

func (c *configuration) providers(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := c.db.QueryContext(ctx, "SELECT name, document FROM providers")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	providers := make(map[string]json.RawMessage)
	for rows.Next() {
		var name, document string
		if err := rows.Scan(&name, &document); err != nil {
			return nil, err
		}
		providers[name] = json.RawMessage(document)
	}
	return providers, rows.Err()
}

func (c *configuration) saveSetting(ctx context.Context, key, value string) error {
	if value == "" {
		_, err := c.db.ExecContext(ctx, "DELETE FROM settings WHERE key = ?", key)
		return err
	}
	_, err := c.db.ExecContext(ctx, "INSERT INTO settings VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value)
	return err
}

func (c *configuration) saveProvider(ctx context.Context, name string, before, after json.RawMessage) error {
	if before == nil {
		_, err := c.db.ExecContext(ctx, "INSERT INTO providers VALUES (?, ?)", name, string(after))
		return err
	}
	var result sql.Result
	var err error
	if after == nil {
		result, err = c.db.ExecContext(ctx, "DELETE FROM providers WHERE name = ? AND document = ?", name, string(before))
	} else {
		result, err = c.db.ExecContext(ctx, "UPDATE providers SET document = ? WHERE name = ? AND document = ?", string(after), name, string(before))
	}
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return errors.New("provider changed in another session. Reopen Reconfigure and try again")
	}
	return err
}
