// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package app

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kpirnie/streamforge/internal/app/menu"
	"github.com/kpirnie/streamforge/internal/dispatcharr"
	"github.com/kpirnie/streamforge/internal/emby"
	"github.com/kpirnie/streamforge/internal/media"
	"github.com/kpirnie/streamforge/internal/xtream"
)

// Run starts the interactive terminal or runs a single command.
func Run(ctx context.Context, args []string, revision, date string) error {
	buildRevision, buildDate = revision, date
	if err := initializePaths(); err != nil {
		return err
	}
	if len(args) == 0 && terminalAvailable(os.Stdin) && terminalAvailable(os.Stdout) {
		return runTerminal(ctx)
	}
	return runCommand(ctx, args, nil, os.Stdout, os.Stderr, nil, nil, nil, nil)
}

func runCommand(ctx context.Context, args []string, logs *os.File, out, stderr io.Writer, discovery *menu.Discovery, progress *operationProgress, sessionValues map[string]string, providerCache *cachedProviders) (runErr error) {
	var command []string
	overrides := make(map[string]string)
	var dryRun, verbose, yes, help bool
	for _, arg := range args {
		if key, option, ok := strings.Cut(arg, "="); ok {
			if setting, exists := globalOptions[key]; exists {
				overrides[setting] = option
				continue
			}
		}
		switch arg {
		case "--help", "-h":
			help = true
		case "--dry-run":
			dryRun = true
		case "--verbose", "-v":
			verbose = true
		case "--yes", "-y":
			yes = true
		default:
			command = append(command, arg)
		}
	}
	if help || len(command) == 0 || (len(command) == 1 && (command[0] == "dispatcharr" || command[0] == "emby" || command[0] == "series" || command[0] == "movies")) {
		if len(command) == 0 || command[0] == "diagnostics" {
			fmt.Fprintln(out, "Usage: streamforge diagnostics [--json] [--save]")
		}
		if len(command) == 0 || command[0] == "movies" {
			fmt.Fprintln(out, "Usage: streamforge movies search [--provider NAME] TITLE")
			fmt.Fprintln(out, "       streamforge movies download --movie='Provider|Movie ID' [--process]")
			fmt.Fprintln(out, "Usage: streamforge movies transcode PATH [--codec=keep|hevc|av1] [--container=keep|mkv|mp4] [--force] [--convert-hdr]")
		}
		if len(command) == 0 || command[0] == "strm" {
			fmt.Fprintln(out, "Usage: streamforge strm PATH [--playback-parent=PATH] [--dry-run]")
		}
		if len(command) == 0 || command[0] == "dispatcharr" {
			fmt.Fprintln(out, "Usage: streamforge dispatcharr <export|refresh|create-update|prune|match|renumber|reorder|clear-epg|all> [--dry-run] [--yes] [--verbose]")
		}
		if len(command) == 0 || command[0] == "emby" {
			fmt.Fprintln(out, "Usage: streamforge emby <refresh|logos|numbers|all> [--dry-run] [--yes] [--verbose]")
		}
		if len(command) == 0 || command[0] == "series" {
			fmt.Fprintln(out, "Usage: streamforge series search [--provider NAME] TITLE")
			fmt.Fprintln(out, "       streamforge series episodes --provider NAME SERIES_ID")
			fmt.Fprintln(out, "       streamforge series transcode PATH [OPTIONS]")
			fmt.Fprintln(out, "       streamforge series download --provider NAME SERIES_ID [--all-episodes|--episode=ID] [--process]")
		}
		return nil
	}
	if command[0] == "diagnostics" {
		if logs == nil {
			file, _ := openLog()
			if file != nil {
				defer file.Close()
			}
			logs = file
		}
		return runDiagnostics(ctx, command[1:], out, logs, dryRun, sessionValues)
	}
	if len(command) < 2 {
		return errors.New("unknown command. Run streamforge --help")
	}
	service, action := command[0], command[1]
	var series seriesCommand
	if service != "series" && service != "strm" && service != "movies" && len(command) != 2 {
		return errors.New("unexpected command arguments. Run streamforge --help")
	}
	switch service {
	case "dispatcharr":
		switch action {
		case "export", "refresh", "create-update", "prune", "renumber", "reorder", "match", "clear-epg", "all":
		default:
			return errors.New("unknown Dispatcharr command. Run streamforge dispatcharr --help")
		}
	case "strm":
	case "movies", "series":
		if service == "movies" && action != "search" && action != "download" && action != "transcode" {
			return errors.New("select movies search, download, or transcode")
		}
		if action == "transcode" {
			break
		}
		parsed, err := parseSeries(command[1:])
		if err != nil {
			return err
		}
		series = parsed
		if service == "movies" && action == "download" && len(series.movies) == 0 {
			return errors.New("movies download requires --movie='Provider|Movie ID'")
		}
	case "emby":
		switch action {
		case "refresh", "logos", "numbers", "all":
		default:
			return errors.New("unknown Emby command. Run streamforge emby --help")
		}
	default:
		return errors.New("unknown command. Run streamforge --help")
	}
	if logs == nil {
		var err error
		logs, err = openLog()
		if err != nil {
			return fmt.Errorf("open session log: %w", err)
		}
		defer logs.Close()
	}
	message := func(level string, terminal io.Writer, format string, args ...any) {
		line := fmt.Sprintf("%s %s %s\n", time.Now().Format("2006-01-02 15:04:05"), level, fmt.Sprintf(format, args...))
		fmt.Fprint(logs, line)
		if terminal != nil {
			fmt.Fprint(terminal, line)
		}
	}
	defer func() {
		if runErr != nil {
			message("ERROR", nil, "%s", runErr)
		}
	}()
	if verbose {
		message("DEBUG", stderr, "%s action %s, dry run: %t", service, action, dryRun)
	}
	settings, err := readEnvironment()
	if err != nil {
		return err
	}
	for _, warning := range unknownSettings(settings) {
		message("WARN", stderr, "%s", warning)
	}
	value := func(key, fallback string) string {
		if option, ok := overrides[key]; ok {
			return option
		}
		return sessionValue(key, fallback, settings, sessionValues)
	}
	if strings.ContainsAny(value("STREAMFORGE_USER_AGENT", ""), "\r\n") {
		return errors.New("STREAMFORGE_USER_AGENT must be one line")
	}
	var findings []error
	timeout, err := time.ParseDuration(value("STREAMFORGE_TIMEOUT", "30s"))
	if err != nil || timeout <= 0 {
		findings = append(findings, errors.New("STREAMFORGE_TIMEOUT must be a positive duration"))
	}
	start, err := strconv.Atoi(value("STREAMFORGE_START_CHANNEL", "1"))
	if service == "dispatcharr" && (err != nil || start < 1) {
		findings = append(findings, errors.New("STREAMFORGE_START_CHANNEL must be a positive integer"))
	}
	verifyTLS, err := strconv.ParseBool(value("STREAMFORGE_VERIFY_TLS", "true"))
	if err != nil {
		findings = append(findings, errors.New("STREAMFORGE_VERIFY_TLS must be a boolean"))
	}
	confirm, err := strconv.ParseBool(value("STREAMFORGE_CONFIRM", "true"))
	if err != nil {
		findings = append(findings, errors.New("STREAMFORGE_CONFIRM must be a boolean"))
	}
	var normalizer *regexp.Regexp
	if pattern := value("STREAMFORGE_NORMALIZER", ""); pattern != "" && service == "dispatcharr" && (action == "all" || action == "create-update") {
		normalizer, err = regexp.Compile(pattern)
		if err != nil {
			findings = append(findings, errors.New("STREAMFORGE_NORMALIZER must be a valid Go regular expression"))
		}
	}
	base, user, password := value("DISP_URL", ""), value("DISP_USER", ""), value("DISP_PASS", "")
	if service == "dispatcharr" && (base == "" || user == "" || password == "") {
		findings = append(findings, errors.New("Dispatcharr requires DISP_URL, DISP_USER and DISP_PASS"))
	}
	embyHost, embyKey, playlist := value("EMBY_HOST", ""), value("EMBY_API_KEY", ""), value("DISP_M3U_URL", "")
	if service == "emby" {
		if embyHost == "" || embyKey == "" {
			findings = append(findings, errors.New("Emby requires EMBY_HOST and EMBY_API_KEY"))
		}
		if (action == "logos" || action == "all") && playlist == "" {
			findings = append(findings, errors.New("Emby logos require DISP_M3U_URL"))
		}
	}
	if err := errors.Join(findings...); err != nil {
		return err
	}
	if service != "series" && service != "strm" && service != "movies" && !(service == "series" && action == "transcode") && confirm && !dryRun && !yes && action != "export" {
		return errors.New("confirm changes with --yes, or preview with --dry-run")
	}
	if !verifyTLS {
		message("WARN", stderr, "TLS certificate verification is disabled")
	}
	transport := makeTransport(timeout, verifyTLS)
	defer transport.CloseIdleConnections()
	agent := value("STREAMFORGE_USER_AGENT", "VLC/3.0.20 LibVLC/3.0.20")
	report := func(format string, args ...any) {
		progress.status(fmt.Sprintf(format, args...))
		message("INFO", out, format, args...)
	}
	parent := value("STREAMFORGE_PLAYBACK_PARENT", "")
	if !dryRun && (service == "strm" || ((service == "series" || service == "movies") && (action == "download" || action == "transcode")) || (service == "dispatcharr" && action == "export")) {
		if root, err := os.OpenRoot(media.Working); err == nil {
			cleanupErr := media.CleanTemporary(root)
			root.Close()
			if cleanupErr != nil {
				message("WARN", stderr, "Some abandoned temporary files could not be cleaned")
			}
		}
	}
	titles := media.NewTitles(value("TMDB_API_KEY", ""), agent, timeout, transport, func(format string, args ...any) { message("WARN", stderr, format, args...) })
	if providerCache != nil {
		titles = providerCache.titles
	}
	if (service == "movies" || service == "series") && action == "transcode" {
		return runTranscode(ctx, command[2:], service == "series", nil, titles, value, dryRun, yes || !confirm, report, progress, nil)
	}
	if service == "strm" {
		return runSTRM(ctx, command[1:], parent, value("STREAMFORGE_EXCLUDE", ""), titles, dryRun, report)
	}
	if service == "series" || service == "movies" {
		var providers *xtream.Providers
		var err error
		if providerCache != nil {
			providers, err = providerCache.providers, providerCache.err
		} else {
			var warnings []error
			providers, warnings, err = xtream.LoadProviders(filepath.Join(configDir, "providers.json"), agent, timeout, transport)
			for _, warning := range warnings {
				message("WARN", stderr, "%s", warning)
			}
		}
		if err != nil {
			return err
		}
		if series.action == "download" {
			return runDownload(ctx, providers, series, parent, titles, value, dryRun, yes || !confirm, report, progress)
		}
		if service == "movies" {
			return runMovieSearch(ctx, providers, series, out, func(format string, args ...any) { message("WARN", stderr, format, args...) }, discovery)
		}
		return runSeries(ctx, providers, series, out, func(format string, args ...any) { message("WARN", stderr, format, args...) }, discovery)
	}
	if service == "emby" {
		client, err := emby.Connect(ctx, embyHost, embyKey, agent, timeout, transport)
		if err != nil {
			return err
		}
		return runEmby(ctx, client, action, playlist, dryRun, report, progress)
	}
	authContext, cancel := context.WithTimeout(ctx, timeout)
	client, err := dispatcharr.Connect(authContext, base, user, password, agent, timeout, transport)
	cancel()
	if err != nil {
		return err
	}
	actions := []string{action}
	if action == "all" {
		actions = []string{"refresh", "create-update", "prune", "match", "renumber", "reorder"}
	}
	var failures []error
	refreshFailed := false
	for _, step := range actions {
		progress.setTotal(0)
		progress.status(step)
		if ctx.Err() != nil {
			return errors.Join(append(failures, ctx.Err())...)
		}
		if step == "prune" && refreshFailed {
			message("WARN", stderr, "Skipped prune after refresh request failed")
			continue
		}
		var err error
		if action == "all" && step == "refresh" && !dryRun {
			err = client.RefreshAndWait(ctx, func() { report("Requested Dispatcharr M3U refresh; waiting for active accounts") })
			if err == nil {
				report("Dispatcharr M3U refresh complete")
			}
		} else {
			err = runDispatcharr(ctx, client, step, start, normalizer, dryRun, report, progress)
		}
		if err != nil {
			refreshFailed = step == "refresh"
			failures = append(failures, fmt.Errorf("%s: %w", step, err))
		}
	}
	return errors.Join(failures...)
}

func makeTransport(timeout time.Duration, verifyTLS bool) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport.DialContext = dialer.DialContext
	transport.TLSHandshakeTimeout = timeout
	transport.ResponseHeaderTimeout = timeout
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: !verifyTLS}
	return transport
}

func runEmby(ctx context.Context, client *emby.Client, action, playlist string, dryRun bool, report func(string, ...any), progress *operationProgress) error {
	actions := []string{action}
	if action == "all" {
		actions = []string{"refresh", "logos", "numbers"}
	}
	var failures []error
	for _, step := range actions {
		progress.setTotal(0)
		progress.status(step)
		if ctx.Err() != nil {
			return errors.Join(append(failures, ctx.Err())...)
		}
		var counts emby.Counts
		var err error
		switch step {
		case "refresh":
			if !dryRun {
				report("Waiting for Emby guide refresh")
			}
			err = client.RefreshGuide(ctx, dryRun)
			if err == nil {
				if dryRun {
					report("Would refresh the Emby guide")
				} else {
					report("Emby guide refresh completed")
				}
			}
		case "logos":
			counts, err = client.RefreshLogos(ctx, playlist, dryRun, progress.items)
		case "numbers":
			counts, err = client.ClearNumbers(ctx, dryRun, progress.items)
		}
		if step != "refresh" {
			if dryRun {
				report("Emby %s: would update %d channels, %d unmatched, %d failed", step, counts.Done, counts.Unmatched, counts.Failed)
			} else {
				report("Emby %s: updated %d channels, %d unmatched, %d failed", step, counts.Done, counts.Unmatched, counts.Failed)
			}
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("Emby %s: %w", step, err))
		}
	}
	return errors.Join(failures...)
}

func runDispatcharr(ctx context.Context, client *dispatcharr.Client, action string, start int, normalizer *regexp.Regexp, dryRun bool, report func(string, ...any), progress *operationProgress) error {
	switch action {
	case "refresh":
		if dryRun {
			report("Would request Dispatcharr M3U refresh")
			return nil
		}
		if err := client.Refresh(ctx); err != nil {
			return err
		}
		report("Requested Dispatcharr M3U refresh")
		return nil
	case "export":
		root, err := os.OpenRoot(media.Working)
		if err != nil {
			return err
		}
		defer root.Close()
		count, err := client.Export(ctx, root, dryRun)
		if err != nil {
			return err
		}
		if dryRun {
			report("Would export %d channels to %s", count, filepath.Join(media.Working, "channels.csv"))
		} else {
			report("Exported %d channels to %s", count, filepath.Join(media.Working, "channels.csv"))
		}
		return nil
	case "prune":
		result, err := client.Prune(ctx, dryRun)
		if dryRun {
			report("Would prune %d streams, %d channels and %d groups", result.Streams, result.Channels, result.Groups)
		} else {
			report("Pruned %d streams, %d channels and %d groups, %d failed", result.Streams, result.Channels, result.Groups, result.Failed)
		}
		return err
	}
	var done, failed int
	var err error
	if action == "create-update" {
		done, failed, err = client.SyncChannels(ctx, normalizer, dryRun, progress.items)
	} else {
		done, failed, err = client.UpdateChannels(ctx, action, start, dryRun, progress.items)
	}
	if dryRun {
		report("%s: would update %d channels", action, done)
	} else {
		report("%s: updated %d channels, %d failed", action, done, failed)
	}
	return err
}

func readEnvironment() (map[string]string, error) {
	settings := make(map[string]string)
	path := filepath.Join(configDir, ".env")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var findings []error
	for line := 1; scanner.Scan(); line++ {
		entry := strings.TrimSpace(scanner.Text())
		if entry == "" || strings.HasPrefix(entry, "#") {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimPrefix(entry, "export "), "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" || strings.ContainsAny(key, " \t") {
			findings = append(findings, fmt.Errorf("invalid configuration on line %d", line))
			continue
		}
		if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
			quote := value[0]
			end := strings.LastIndexByte(value, quote)
			if end == 0 {
				findings = append(findings, fmt.Errorf("unterminated quoted configuration on line %d", line))
				continue
			}
			value = value[1:end]
		} else if comment := strings.Index(value, " #"); comment >= 0 {
			value = strings.TrimSpace(value[:comment])
		}
		settings[key] = value
	}
	return settings, errors.Join(append(findings, scanner.Err())...)
}

func openLog() (*os.File, error) {
	stem := filepath.Join(media.Working, "streamforge-"+time.Now().Format("20060102-150405"))
	for i := 0; ; i++ {
		path := stem + ".log"
		if i > 0 {
			path = fmt.Sprintf("%s-%d.log", stem, i)
		}
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if !errors.Is(err, os.ErrExist) {
			return file, err
		}
	}
}
