// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package main

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
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kpirnie/streamforge/internal/dispatcharr"
)

const workDir = "/working"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) (runErr error) {
	var command []string
	var dryRun, verbose, yes, help bool
	for _, arg := range args {
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
	if help || len(command) == 0 || (len(command) == 1 && command[0] == "dispatcharr") {
		fmt.Println("Usage: streamforge dispatcharr <export|refresh|create-update|prune|match|renumber|reorder|clear-epg|all> [--dry-run] [--yes] [--verbose]")
		return nil
	}
	if len(command) != 2 || command[0] != "dispatcharr" {
		return errors.New("unknown command. Run streamforge --help")
	}
	action := command[1]
	switch action {
	case "export", "refresh", "create-update", "prune", "renumber", "reorder", "match", "clear-epg", "all":
	default:
		return errors.New("unknown Dispatcharr command. Run streamforge dispatcharr --help")
	}
	logs, err := openLog()
	if err != nil {
		return fmt.Errorf("open session log: %w", err)
	}
	defer logs.Close()
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
		message("DEBUG", os.Stderr, "Dispatcharr action %s, dry run: %t", action, dryRun)
	}
	settings, err := readEnvironment()
	if err != nil {
		return err
	}
	value := func(key, fallback string) string {
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
		if v, ok := settings[key]; ok {
			return v
		}
		return fallback
	}
	var findings []error
	timeout, err := time.ParseDuration(value("STREAMFORGE_TIMEOUT", "30s"))
	if err != nil || timeout <= 0 {
		findings = append(findings, errors.New("STREAMFORGE_TIMEOUT must be a positive duration"))
	}
	start, err := strconv.Atoi(value("STREAMFORGE_START_CHANNEL", "1"))
	if err != nil || start < 1 {
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
	if pattern := value("STREAMFORGE_NORMALIZER", ""); pattern != "" && (action == "all" || action == "create-update") {
		normalizer, err = regexp.Compile(pattern)
		if err != nil {
			findings = append(findings, errors.New("STREAMFORGE_NORMALIZER must be a valid Go regular expression"))
		}
	}
	base, user, password := value("DISP_URL", ""), value("DISP_USER", ""), value("DISP_PASS", "")
	if base == "" || user == "" || password == "" {
		findings = append(findings, errors.New("Dispatcharr requires DISP_URL, DISP_USER and DISP_PASS"))
	}
	if err := errors.Join(findings...); err != nil {
		return err
	}
	if confirm && !dryRun && !yes && action != "export" {
		return errors.New("confirm Dispatcharr changes with --yes, or preview with --dry-run")
	}
	if !verifyTLS {
		message("WARN", os.Stderr, "TLS certificate verification is disabled")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport.DialContext = dialer.DialContext
	transport.TLSHandshakeTimeout = timeout
	transport.ResponseHeaderTimeout = timeout
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: !verifyTLS}
	authContext, cancel := context.WithTimeout(ctx, timeout)
	client, err := dispatcharr.Connect(authContext, base, user, password, value("STREAMFORGE_USER_AGENT", "VLC/3.0.20 LibVLC/3.0.20"), timeout, transport)
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
		if ctx.Err() != nil {
			return errors.Join(append(failures, ctx.Err())...)
		}
		if step == "prune" && refreshFailed {
			message("WARN", os.Stderr, "Skipped prune after refresh request failed")
			continue
		}
		err := runDispatcharr(ctx, client, step, start, normalizer, dryRun, func(format string, args ...any) { message("INFO", os.Stdout, format, args...) })
		if err != nil {
			refreshFailed = step == "refresh"
			failures = append(failures, fmt.Errorf("%s: %w", step, err))
		}
	}
	return errors.Join(failures...)
}

func runDispatcharr(ctx context.Context, client *dispatcharr.Client, action string, start int, normalizer *regexp.Regexp, dryRun bool, report func(string, ...any)) error {
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
		root, err := os.OpenRoot(workDir)
		if err != nil {
			return err
		}
		defer root.Close()
		count, err := client.Export(ctx, root, dryRun)
		if err != nil {
			return err
		}
		if dryRun {
			report("Would export %d channels to /working/channels.csv", count)
		} else {
			report("Exported %d channels to /working/channels.csv", count)
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
		done, failed, err = client.SyncChannels(ctx, normalizer, dryRun)
	} else {
		done, failed, err = client.UpdateChannels(ctx, action, start, dryRun)
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
	file, err := os.Open("/config/.env")
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open /config/.env: %w", err)
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
	stem := workDir + "/streamforge-" + time.Now().Format("20060102-150405")
	for i := 0; ; i++ {
		path := stem + ".log"
		if i > 0 {
			path = fmt.Sprintf("%s-%d.log", stem, i)
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if !errors.Is(err, os.ErrExist) {
			return file, err
		}
	}
}
