// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kpirnie/streamforge/internal/app/menu"
	"github.com/kpirnie/streamforge/internal/media"
	"github.com/kpirnie/streamforge/internal/xtream"
)

type terminalBuffer struct {
	bytes.Buffer
}

func (b *terminalBuffer) Write(data []byte) (int, error) {
	if b.Len() < 256<<10 {
		_, _ = b.Buffer.Write(data)
	}
	return len(data), nil
}

func runTerminal(ctx context.Context) error {
	restoreTerminal, err := terminalMode()
	if err != nil {
		return err
	}
	defer restoreTerminal()
	fmt.Fprint(os.Stdout, "\x1b[?1049h\x1b[?25l")
	defer fmt.Fprint(os.Stdout, "\x1b[0m\x1b[?25h\x1b[?1049l")
	fmt.Fprint(os.Stdout,
		"\x1b[2J\x1b[H"+
			"\x1b[96m       o-----------o\r\n"+
			"       | o       o |\r\n"+
			"       | o   \x1b[92m>\x1b[96m   o |\r\n"+
			"       o-----------o\r\n"+
			"    ~~~~~~~\\      \x1b[92m* *\x1b[96m\r\n"+
			"      ~~~~~~\\____/_______\r\n"+
			"         \\_____________/\r\n"+
			"              /   \\\r\n"+
			"            _/_____\\_\r\n"+
			"\r\n"+
			"┌"+strings.Repeat("─", 34)+"┐\r\n"+
			"│ \x1b[92mStreamForge - Forge Your Streams\x1b[96m │\r\n"+
			"└"+strings.Repeat("─", 34)+"┘\x1b[0m\r\n",
	)
	session, err := openLog()
	if err != nil {
		return err
	}
	defer session.Close()
	queue, err := openDownloadQueue()
	if err != nil {
		return fmt.Errorf("open download queue: %w", err)
	}
	defer queue.close()

	cfg := configurationFor(ctx)
	settings, err := cfg.settings(ctx)
	if err != nil {
		return err
	}
	records, err := cfg.providers(ctx)
	if err != nil {
		return err
	}
	editor := configurationEditor{store: cfg, settings: settings, records: records}
	startupValue := func(key, fallback string) string { return settingValue(key, fallback, editor.settings) }
	var providerTransport *http.Transport
	var providerCache *cachedProviders
	refreshProviders := func() {
		if providerTransport != nil {
			providerTransport.CloseIdleConnections()
		}
		timeout, err := time.ParseDuration(startupValue("STREAMFORGE_TIMEOUT", "30s"))
		if err != nil || timeout <= 0 {
			timeout = 30 * time.Second
		}
		verify, err := strconv.ParseBool(startupValue("STREAMFORGE_VERIFY_TLS", "true"))
		providerTransport = makeTransport(timeout, err != nil || verify)
		agent := startupValue("STREAMFORGE_USER_AGENT", "VLC/3.0.20 LibVLC/3.0.20")
		providers, warnings, providerErr := xtream.LoadProviders(editor.records, agent, timeout, providerTransport)
		providerCache = &cachedProviders{providers: providers, err: providerErr, titles: media.NewTitles(startupValue("TMDB_API_KEY", ""), agent, timeout, providerTransport, func(format string, args ...any) {
			fmt.Fprintf(session, "%s WARN %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
		})}
		for _, warning := range warnings {
			fmt.Fprintf(session, "%s WARN %s\n", time.Now().Format("2006-01-02 15:04:05"), warning)
		}
	}
	refreshProviders()
	defer func() { providerTransport.CloseIdleConnections() }()
	startup := make(chan statusReport, 1)
	go func() { startup <- diagnosticStatus(ctx, session) }()
	splash := time.NewTimer(5 * time.Second)
	defer splash.Stop()
	var status statusReport
	select {
	case status = <-startup:
	case <-ctx.Done():
		return nil
	}
	select {
	case <-splash.C:
	case <-ctx.Done():
		return nil
	}

	inputContext, stopInput := context.WithCancel(ctx)
	defer stopInput()
	keys := terminalInput(inputContext, os.Stdin)
	redraw := time.NewTicker(250 * time.Millisecond)
	defer redraw.Stop()
	page, mode, filter, input := "Main", "menu", "", ""
	selection, scroll := 0, 0
	var choice menu.Choice
	var arguments []string
	var result string
	browser := menu.NewBrowser(downloadPath)
	browser.RestoreQueue(queue.snapshot())
	var cancelOperation context.CancelFunc
	type operationResult struct {
		clipboard string
		discovery menu.Discovery
		text      string
		status    *statusReport
		preview   bool
		failed    bool
	}
	finished := make(chan operationResult, 1)
	var started time.Time
	var progress *operationProgress
	activeSelection := 0
	for _, warning := range status.Warnings {
		fmt.Fprintf(session, "%s WARN %s\n", time.Now().Format("2006-01-02 15:04:05"), warning)
	}
	if len(status.Findings) != 0 {
		mode, result = "startup", strings.Join(status.Findings, "\n")
	}
	navigate := func(destination string) {
		page, mode, filter, input, selection, scroll = destination, "menu", "", "", 0, 0
	}
	launch := func(preview bool) {
		operation, cancel := context.WithCancel(ctx)
		cancelOperation = cancel
		mode, started = "running", time.Now()
		progress = &operationProgress{active: make(map[string]context.CancelFunc)}
		if !preview && page == menu.DownloadQueue {
			progress.complete = queue.complete
		}
		activeSelection = 0
		currentProgress := progress
		args := slices.Clone(arguments)
		if preview {
			args = append(args, "--dry-run")
		}
		download := !preview && len(args) > 1 && args[1] == "download"
		retry := choice.Label == "Retry Connections"
		copyReport := choice.Label == "Copy Diagnostics"
		clipboard := copyReport && supportsClipboard()
		if copyReport && !clipboard {
			args = append(args, "--save")
		}
		go func() {
			defer cancel()
			buffer := &terminalBuffer{}
			var refreshed *statusReport
			if retry {
				result := diagnosticStatus(operation, session)
				refreshed = &result
			}
			var discovery menu.Discovery
			err := runCommand(operation, args, session, buffer, buffer, &discovery, currentProgress, providerCache)
			var copied string
			if clipboard && err == nil {
				copied = base64.StdEncoding.EncodeToString(buffer.Bytes())
				buffer.Reset()
				fmt.Fprintln(buffer, "Sent diagnostics to the terminal clipboard.")
			}
			switch {
			case operation.Err() != nil:
				discovery = menu.Discovery{}
				fmt.Fprintln(buffer, "Cancelled.")
			case err != nil:
				fmt.Fprintln(buffer, err)
			default:
				fmt.Fprintln(buffer, "Completed.")
			}
			result := buffer.String()
			if download {
				heading := "Completion Summary"
				if currentProgress.complete != nil {
					heading += fmt.Sprintf(" · %d still queued", len(queue.snapshot()))
				}
				result = heading + "\n\n" + result
			}
			finished <- operationResult{copied, discovery, result, refreshed, preview, err != nil || operation.Err() != nil}
		}()
	}
	showConfigurationError := func(err error) { mode, result, scroll = "result", err.Error(), 0 }
	leaveReconfigure := func() {
		navigate("Main")
		if editor.dirty {
			editor.dirty = false
			refreshProviders()
			choice = menu.Choice{Label: "Retry Connections"}
			arguments = []string{"diagnostics"}
			launch(false)
		}
	}
	for {
		choices := editor.choices(page)
		if choices == nil {
			choices = browser.Choices(page)
		}
		available := make([]menu.Choice, 0, len(choices))
		for _, item := range choices {
			if providerCache.err != nil && page == "Downloads" && (item.Page == menu.DownloadQueue || (len(item.Args) > 1 && item.Args[1] != "transcode")) {
				item.Label += " (disabled)"
				item.Args = []string{"diagnostics"}
				item.Prompt = ""
				item.Page = ""
				item.Confirm = false
			}
			if state, ok := status.Services[item.Label]; ok && state != "connected" {
				item.Label += " (disabled)"
				item.Page = ""
				item.Args = []string{"diagnostics"}
			}
			if strings.Contains(strings.ToLower(item.Label), strings.ToLower(filter)) {
				available = append(available, item)
			}
		}
		if selection >= len(available) {
			selection = max(0, len(available)-1)
		}
		width, height := terminalDimensions()
		frame := []string{fmt.Sprintf("Stream Forge  |  %s  |  %s", status.Runtime, status.Encoder), fmt.Sprintf("Emby %s  |  Dispatcharr %s", strings.Fields(status.Services["Emby"])[0], strings.Fields(status.Services["Dispatcharr"])[0]), browser.Heading(page), ""}
		footer := "↑↓ Navigate   Enter Select   Esc Back   Ctrl+C Exit"
		var activeDownloads []string
		switch mode {
		case "menu":
			if filter != "" {
				frame = append(frame, "Filter: "+filter)
			}
			slots := max(1, height-len(frame)-2)
			first := max(0, selection-slots+1)
			for i, item := range available[first:min(len(available), first+slots)] {
				marker := "  "
				if first+i == selection {
					marker = "› "
				}
				frame = append(frame, marker+item.Label)
			}
		case "input":
			shown := input
			if choice.Secret {
				shown = strings.Repeat("•", utf8.RuneCountInString(input))
			}
			frame = append(frame, choice.Label, "", choice.Prompt, shown)
			footer = "Enter Continue   Esc Back   Ctrl+C Exit"
		case "confirm":
			frame = append(frame, "Apply "+choice.Label+"?", "No (Enter)   Yes (Y)", "")
			lines := strings.Split(strings.TrimSpace(result), "\n")
			slots := max(1, height-len(frame)-1)
			scroll = min(scroll, max(0, len(lines)-slots))
			frame = append(frame, lines[scroll:min(len(lines), scroll+slots)]...)
			footer = "Y Confirm   Enter / Esc Cancel   Ctrl+C Exit"
		case "running":
			frame = append(frame, choice.Label, "", fmt.Sprintf("Running for %s", time.Since(started).Truncate(time.Second)))
			footer = "Esc Cancel operation   Ctrl+C Exit"
			state := progress.snapshot()
			if state.total > 0 {
				done := state.done + state.failed + state.skipped + state.cancelled
				filled := min(12, 12*done/state.total)
				frame = append(frame, "["+strings.Repeat("=", filled)+strings.Repeat(" ", 12-filled)+"]")
				frame = append(frame, fmt.Sprintf("%d%% · %d/%d · %d failed · %d skipped · %d cancelled", 100*done/state.total, done, state.total, state.failed, state.skipped, state.cancelled))
			}
			if state.note != "" {
				frame = append(frame, state.note)
			}
			if len(state.active) > 0 {
				slots := max(0, height-len(frame)-1)
				if slots > 0 {
					activeDownloads = state.active
					activeSelection = min(activeSelection, len(activeDownloads)-1)
					first := max(0, activeSelection-slots+1)
					for i, label := range activeDownloads[first:min(len(activeDownloads), first+slots)] {
						marker := "  "
						if first+i == activeSelection {
							marker = "› "
						}
						frame = append(frame, marker+label)
					}
				}
			}
			if len(activeDownloads) > 0 {
				footer = "↑↓ Active downloads  X Cancel selected  Esc Cancel all"
			}
		case "result", "startup":
			lines := strings.Split(strings.TrimSpace(result), "\n")
			slots := max(1, height-len(frame)-2)
			scroll = min(scroll, max(0, len(lines)-slots))
			frame = append(frame, lines[scroll:min(len(lines), scroll+slots)]...)
			footer = "↑↓ Scroll   Enter Return   Ctrl+C Exit"
			if mode == "startup" {
				footer = "Enter Continue   Esc Quit"
			}
		}
		drawTerminal(os.Stdout, width, height, frame, footer)
		select {
		case <-ctx.Done():
			if mode == "running" {
				cancelOperation()
				<-finished
			}
			return nil
		case <-redraw.C:
		case completion := <-finished:
			browser.RestoreQueue(queue.snapshot())
			result = completion.text
			if completion.clipboard != "" {
				fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\a", completion.clipboard)
			}
			if completion.status != nil {
				status = *completion.status
			}
			mode, scroll = "result", 0
			if completion.preview {
				if !completion.failed {
					mode = "confirm"
				}
			} else if destination := browser.Browse(completion.discovery, page); destination != "" {
				navigate(destination)
			}
		case key, ok := <-keys:
			if !ok {
				if mode == "running" {
					cancelOperation()
					<-finished
				}
				return nil
			}
			switch mode {
			case "startup":
				if key == "escape" {
					return nil
				}
				if key == "enter" {
					navigate(page)
				}
			case "menu":
				switch {
				case key == "escape":
					if filter != "" {
						filter, selection = "", 0
					} else if page == reconfigurePage {
						leaveReconfigure()
					} else if previous := reconfigurePrevious(page); previous != "" {
						navigate(previous)
					} else if page != "Main" {
						navigate(browser.Previous(page))
					}
				case key == "up":
					selection = max(0, selection-1)
				case key == "down":
					selection = min(max(0, len(available)-1), selection+1)
				case key == "backspace":
					filter, selection = shorten(filter), 0
				case strings.HasPrefix(key, "text:"):
					filter, selection = filter+strings.TrimPrefix(key, "text:"), 0
				case key == "enter" && len(available) > 0:
					choice = available[selection]
					switch {
					case choice.Mark == "config-done":
						leaveReconfigure()
					case choice.Mark == "config-setting" || choice.Mark == "config-provider-field":
						mode, input = "input", editor.input(choice)
					case choice.Mark == "config-provider-new" || choice.Mark == "config-provider-open":
						editor.openProvider(choice.Setting)
						navigate(providerEditorPage)
					case choice.Mark == "config-provider-save":
						if err := editor.saveProvider(ctx); err != nil {
							showConfigurationError(err)
						} else {
							navigate(providerListPage)
						}
					case choice.Mark == "config-provider-remove":
						choice.Label = "Remove provider " + editor.name
						mode, result, scroll = "confirm", "", 0
					case choice.Mark != "":
						arguments = browser.Mark(choice)
						if len(arguments) != 0 {
							launch(true)
						} else if err := queue.replace(browser.Queue()); err != nil {
							browser.RestoreQueue(queue.snapshot())
							mode, result, scroll = "result", fmt.Sprintf("Save download queue: %v", err), 0
						}
					case choice.Page != "":
						browser.Open(choice)
						navigate(choice.Page)
					case choice.Args == nil:
						return nil
					case choice.Prompt != "":
						mode, input = "input", ""
					default:
						arguments = slices.Clone(choice.Args)
						if choice.Confirm && startupValue("STREAMFORGE_CONFIRM", "true") != "false" {
							launch(true)
						} else {
							launch(false)
						}
					}
				}
			case "input":
				switch {
				case key == "escape":
					mode = "menu"
				case key == "backspace":
					input = shorten(input)
				case strings.HasPrefix(key, "text:"):
					input += strings.TrimPrefix(key, "text:")
				case key == "enter":
					var err error
					if choice.Mark == "config-setting" || choice.Mark == "config-provider-field" {
						if err := editor.edit(ctx, choice, input); err != nil {
							showConfigurationError(err)
						} else {
							mode, input = "menu", ""
						}
						break
					}
					arguments, err = menu.Arguments(choice, input)
					if err != nil {
						mode, result, scroll = "result", err.Error(), 0
					} else if choice.Confirm && startupValue("STREAMFORGE_CONFIRM", "true") != "false" {
						launch(true)
					} else {
						launch(false)
					}
				}
			case "confirm":
				if key == "up" {
					scroll = max(0, scroll-1)
				} else if key == "down" {
					scroll++
				}
				if key == "text:y" || key == "text:Y" {
					if choice.Mark == "config-provider-remove" {
						if err := editor.removeProvider(ctx); err != nil {
							showConfigurationError(err)
						} else {
							navigate(providerListPage)
						}
					} else {
						arguments = append(arguments, "--yes")
						launch(false)
					}
				} else if key == "enter" || key == "escape" {
					mode = "menu"
				}
			case "running":
				switch key {
				case "escape":
					cancelOperation()
				case "up":
					activeSelection = max(0, activeSelection-1)
				case "down":
					activeSelection = min(activeSelection+1, max(0, len(activeDownloads)-1))
				case "text:x", "text:X":
					if len(activeDownloads) > 0 {
						progress.cancel(activeDownloads[activeSelection])
					}
				}
			case "result":
				switch key {
				case "enter":
					mode = "menu"
				case "down":
					scroll++
				case "up":
					scroll = max(0, scroll-1)
				}
			}
		}
	}
}

func drawTerminal(out io.Writer, width, height int, lines []string, footer string) {
	var frame strings.Builder
	frame.WriteString("\x1b[H")
	for i := 0; i < height; i++ {
		frame.WriteString("\x1b[2K")
		line := ""
		if i == height-1 {
			line = footer
		} else if i < len(lines) {
			line = lines[i]
		}
		line = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				return ' '
			}
			return r
		}, line)
		runes := []rune(line)
		if len(runes) > max(0, width-1) {
			runes = runes[:max(0, width-1)]
		}
		if i == 0 || i == height-1 {
			frame.WriteString("\x1b[96m")
		}
		if strings.HasPrefix(line, "› ") {
			frame.WriteString("\x1b[92m")
		}
		frame.WriteString(string(runes))
		frame.WriteString("\x1b[0m")
		if i < height-1 {
			frame.WriteString("\r\n")
		}
	}
	fmt.Fprint(out, frame.String())
}

func shorten(input string) string {
	_, size := utf8.DecodeLastRuneInString(input)
	return input[:len(input)-size]
}

func supportsClipboard() bool {
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app", "WezTerm", "kitty":
		return true
	default:
		return false
	}
}
