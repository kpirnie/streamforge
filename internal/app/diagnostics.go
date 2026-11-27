// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kpirnie/streamforge/internal/dispatcharr"
	"github.com/kpirnie/streamforge/internal/emby"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kpirnie/streamforge/internal/media"
)

var buildRevision string
var buildDate string

type statusReport struct {
	Version      string            `json:"version"`
	Commit       string            `json:"commit"`
	Built        string            `json:"build_date"`
	Platform     string            `json:"platform"`
	Runtime      string            `json:"runtime"`
	Dependencies map[string]string `json:"dependencies"`
	Devices      map[string]bool   `json:"devices"`
	Mounts       map[string]string `json:"mounts"`
	Services     map[string]string `json:"services"`
	Findings     []string          `json:"findings"`
	Warnings     []string          `json:"warnings"`
	Encoder      string            `json:"encoder"`
	Capabilities map[string]bool   `json:"capabilities"`
	Session      []string          `json:"session_log"`
}

func diagnosticDevicePaths() []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	return []string{"/dev/nvidiactl", "/dev/dri/renderD128"}
}

func diagnosticStatus(ctx context.Context, logFile *os.File, session map[string]string) statusReport {
	report := statusReport{Version: "development", Commit: "unknown", Built: "unknown", Platform: runtime.GOOS + "/" + runtime.GOARCH, Runtime: runtime.GOOS,
		Dependencies: make(map[string]string), Devices: make(map[string]bool), Mounts: make(map[string]string), Services: make(map[string]string),
		Findings: []string{}, Session: []string{}}
	if build, ok := debug.ReadBuildInfo(); ok {
		if build.Main.Version != "" && build.Main.Version != "(devel)" {
			report.Version = build.Main.Version
		}
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				report.Commit = setting.Value
			case "vcs.time":
				report.Built = setting.Value
			}
		}
	}
	if buildRevision != "" && buildRevision != "unknown" {
		report.Commit = buildRevision
	}
	if buildDate != "" {
		report.Built = buildDate
	}
	env, err := readEnvironment()
	report.Warnings = unknownSettings(env)
	if err != nil {
		report.Findings = append(report.Findings, "Configuration could not be read or contains invalid entries")
	}
	value := func(key, fallback string) string { return sessionValue(key, fallback, env, session) }
	for _, device := range diagnosticDevicePaths() {
		file, err := os.OpenFile(device, os.O_RDWR, 0)
		report.Devices[device] = err == nil
		if err == nil {
			file.Close()
		}
	}
	for _, dir := range applicationDirectories() {
		file, err := os.Open(dir)
		report.Mounts[dir] = "unavailable"
		if err == nil {
			info, err := file.Stat()
			if err == nil && info.IsDir() {
				report.Mounts[dir] = "accessible"
			}
			file.Close()
		}
	}
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		binary := mediaExecutable(value, tool)
		probe, cancel := context.WithTimeout(ctx, 3*time.Second)
		data, err := exec.CommandContext(probe, binary, "-version").Output()
		cancel()
		report.Dependencies[tool] = "unavailable"
		fields := strings.Fields(string(data))
		if err == nil && len(fields) >= 3 && fields[0] == tool && fields[1] == "version" {
			report.Dependencies[tool] = fields[2]
		}
	}
	report.Encoder = "unavailable"
	report.Capabilities = make(map[string]bool)
	if report.Dependencies["ffmpeg"] != "unavailable" {
		binary := mediaExecutable(value, "ffmpeg")
		probe, cancel := context.WithTimeout(ctx, 3*time.Second)
		encoders, _ := exec.CommandContext(probe, binary, "-hide_banner", "-encoders").Output()
		cancel()
		probe, cancel = context.WithTimeout(ctx, 3*time.Second)
		filters, _ := exec.CommandContext(probe, binary, "-hide_banner", "-filters").Output()
		cancel()
		for _, name := range []string{"libx264", "libx265", "libsvtav1", "hevc_nvenc", "hevc_qsv", "hevc_vaapi"} {
			report.Capabilities[name] = strings.Contains(string(encoders), " "+name+" ")
		}
		for _, name := range []string{"zscale", "tonemap"} {
			report.Capabilities[name] = strings.Contains(string(filters), " "+name+" ")
		}
		report.Encoder = "CPU"
		switch value("STREAMFORGE_HARDWARE", "auto") {
		case "auto", "nvidia":
			if report.Devices["/dev/nvidiactl"] && report.Capabilities["hevc_nvenc"] {
				report.Encoder = "NVIDIA"
			}
		}
		if report.Encoder == "CPU" && report.Devices["/dev/dri/renderD128"] {
			switch profile := value("STREAMFORGE_HARDWARE", "auto"); profile {
			case "auto", "intel":
				if report.Capabilities["hevc_qsv"] {
					report.Encoder = "Intel"
				}
			case "amd":
				if report.Capabilities["hevc_vaapi"] {
					report.Encoder = "AMD"
				}
			}
		}
	}
	report.Services, report.Findings = serviceStatus(ctx, value, report.Findings)
	if logFile != nil {
		reader := io.NewSectionReader(logFile, 0, 256<<10)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 3 {
				continue
			}
			if _, err := time.Parse("2006-01-02 15:04:05", fields[0]+" "+fields[1]); err != nil {
				continue
			}
			switch fields[2] {
			case "INFO", "WARN", "ERROR", "DEBUG":
				// Keep only the timestamp and level so arbitrary server and file text cannot enter a report.
				report.Session = append(report.Session, strings.Join(fields[:3], " "))
			}
		}
	}
	return report
}

func serviceStatus(ctx context.Context, value func(string, string) string, findings []string) (map[string]string, []string) {
	timeout, err := time.ParseDuration(value("STREAMFORGE_TIMEOUT", "30s"))
	if err != nil || timeout <= 0 {
		findings = append(findings, "STREAMFORGE_TIMEOUT must be a positive duration")
		timeout = 3 * time.Second
	}
	verify, err := strconv.ParseBool(value("STREAMFORGE_VERIFY_TLS", "true"))
	if err != nil {
		findings = append(findings, "STREAMFORGE_VERIFY_TLS must be a boolean")
		verify = true
	}
	transport := makeTransport(timeout, verify)
	defer transport.CloseIdleConnections()
	agent := value("STREAMFORGE_USER_AGENT", "VLC/3.0.20 LibVLC/3.0.20")
	checks := []struct {
		name string
		run  func(context.Context, *http.Transport) (bool, error)
	}{
		{"Dispatcharr", func(ctx context.Context, t *http.Transport) (bool, error) {
			host, user, password := value("DISP_URL", ""), value("DISP_USER", ""), value("DISP_PASS", "")
			if host == "" && user == "" && password == "" {
				return false, nil
			}
			if host == "" || user == "" || password == "" {
				return true, errors.New("missing connection settings")
			}
			_, err := dispatcharr.Connect(ctx, host, user, password, agent, timeout, t)
			return true, err
		}},
		{"Emby", func(ctx context.Context, t *http.Transport) (bool, error) {
			host, key := value("EMBY_HOST", ""), value("EMBY_API_KEY", "")
			if host == "" && key == "" {
				return false, nil
			}
			if host == "" || key == "" {
				return true, errors.New("missing connection settings")
			}
			_, err := emby.Connect(ctx, host, key, agent, timeout, t)
			return true, err
		}},
	}
	statuses := make([]string, len(checks))
	var wait sync.WaitGroup
	for i, check := range checks {
		wait.Go(func() {
			probe, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			configured, err := check.run(probe, transport)
			switch {
			case err != nil:
				statuses[i] = "unavailable: " + err.Error()
			case configured:
				statuses[i] = "connected"
			default:
				statuses[i] = "unconfigured"
			}
		})
	}
	wait.Wait()
	services := make(map[string]string, len(checks))
	for i, check := range checks {
		services[check.name] = statuses[i]
		if strings.HasPrefix(statuses[i], "unavailable") {
			findings = append(findings, check.name+": "+statuses[i])
		}
	}
	return services, findings
}

func runDiagnostics(ctx context.Context, args []string, out io.Writer, logFile *os.File, dryRun bool, session map[string]string) error {
	asJSON, save := false, false
	for _, arg := range args {
		switch arg {
		case "--json":
			asJSON = true
		case "--save":
			save = true
		default:
			return errors.New("usage: streamforge diagnostics [--json] [--save]")
		}
	}
	report := diagnosticStatus(ctx, logFile, session)
	var data []byte
	extension := ".txt"
	if asJSON {
		var err error
		data, err = json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		extension = ".json"
	} else {
		var text strings.Builder
		fmt.Fprintf(&text, "Stream Forge %s (%s)\nBuild date: %s\nRuntime: %s (%s)\n", report.Version, report.Commit, report.Built, report.Runtime, report.Platform)
		fmt.Fprintf(&text, "Encoder: %s\n", report.Encoder)
		for _, name := range []string{"ffmpeg", "ffprobe"} {
			fmt.Fprintf(&text, "%s: %s\n", name, report.Dependencies[name])
		}
		for _, name := range []string{"Dispatcharr", "Emby"} {
			fmt.Fprintf(&text, "%s: %s\n", name, report.Services[name])
		}
		for _, name := range diagnosticDevicePaths() {
			fmt.Fprintf(&text, "%s accessible: %t\n", name, report.Devices[name])
		}
		for _, name := range applicationDirectories() {
			fmt.Fprintf(&text, "%s: %s\n", name, report.Mounts[name])
		}
		for _, warning := range report.Warnings {
			fmt.Fprintln(&text, warning)
		}
		for _, finding := range report.Findings {
			fmt.Fprintln(&text, finding)
		}
		for _, entry := range report.Session {
			fmt.Fprintln(&text, entry)
		}
		data = []byte(text.String())
	}
	if !save || dryRun {
		_, err := out.Write(data)
		return err
	}
	root, err := os.OpenRoot(media.Working)
	if err != nil {
		return err
	}
	defer root.Close()
	name := "streamforge-diagnostics-" + time.Now().Format("20060102-150405.000000000") + extension
	if err := media.AtomicWrite(ctx, root, name, 0600, false, func(file *os.File) error { _, err := file.Write(data); return err }); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, filepath.Join(media.Working, name))
	return err
}
