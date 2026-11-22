// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/kpirnie/streamforge/internal/media"
	"github.com/kpirnie/streamforge/internal/xtream"
)

type terminalChoice struct {
	label   string
	args    []string
	page    string
	prompt  string
	confirm bool
	mark    string
	season  int
	episode string
	key     seriesKey
	movie   xtream.Movie
	setting string
	secret  bool
}

var terminalMenus = map[string][]terminalChoice{
	"Main": {
		{label: "Dispatcharr", page: "Dispatcharr"},
		{label: "Emby", page: "Emby"},
		{label: "Downloads", page: "Downloads"},
		{label: "Transcode media", args: []string{"movies", "transcode"}, prompt: "Media path", confirm: true},
		{label: "Generate .strm companions", args: []string{"strm"}, prompt: "Media path", confirm: true},
		{label: "System Status", page: "System Status"},
		{label: "Exit"},
	},
	"Dispatcharr": {
		{label: "Refresh", args: []string{"dispatcharr", "refresh"}, confirm: true},
		{label: "Create/Update", args: []string{"dispatcharr", "create-update"}, confirm: true},
		{label: "Prune", args: []string{"dispatcharr", "prune"}, confirm: true},
		{label: "Match", args: []string{"dispatcharr", "match"}, confirm: true},
		{label: "Renumber", args: []string{"dispatcharr", "renumber"}, confirm: true},
		{label: "Reorder", args: []string{"dispatcharr", "reorder"}, confirm: true},
		{label: "Clear EPG Mappings", args: []string{"dispatcharr", "clear-epg"}, confirm: true},
		{label: "Export", args: []string{"dispatcharr", "export"}},
		{label: "All", args: []string{"dispatcharr", "all"}, confirm: true},
	},
	"Emby": {
		{label: "Refresh Guide", args: []string{"emby", "refresh"}, confirm: true},
		{label: "Logos", args: []string{"emby", "logos"}, confirm: true},
		{label: "Numbers", args: []string{"emby", "numbers"}, confirm: true},
		{label: "All", args: []string{"emby", "all"}, confirm: true},
	},
	"Downloads": {
		{label: "Search series", args: []string{"series", "search"}, prompt: "Title"},
		{label: "Search movies", args: []string{"movies", "search"}, prompt: "Title"},
		{label: "Download queue", page: downloadQueue},
		{label: "List episodes", args: []string{"series", "episodes"}, prompt: "Provider name | Series ID"},
		{label: "Download series", args: []string{"series", "download"}, prompt: "Provider name | Series ID | Episode IDs (comma separated or all)", confirm: true},
		{label: "Download and process series", args: []string{"series", "download", "--process"}, prompt: "Provider name | Series ID | Episode IDs (comma separated or all)", confirm: true},
		{label: "Transcode media", args: []string{"movies", "transcode"}, prompt: "Media path", confirm: true},
	},
	"System Status": {
		{label: "Show status", args: []string{"diagnostics"}},
		{label: "Retry Connections", args: []string{"diagnostics"}, page: ""},
		{label: "Save diagnostics", args: []string{"diagnostics", "--save"}},
		{label: "Copy Diagnostics", args: []string{"diagnostics"}},
	},
}

func terminalAvailable(file *os.File) bool {
	var settings syscall.Termios
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&settings)))
	return err == 0
}

func terminalDimensions() (int, int) {
	var size struct{ Rows, Columns, X, Y uint16 }
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&size)))
	if err != 0 || size.Columns == 0 || size.Rows == 0 {
		return 80, 24
	}
	return int(size.Columns), int(size.Rows)
}

func terminalInput(ctx context.Context, fd int) <-chan string {
	keys := make(chan string, 32)
	go func() {
		defer close(keys)
		buffer := make([]byte, 128)
		pending := ""
		for ctx.Err() == nil {
			count, err := syscall.Read(fd, buffer)
			if err != nil && err != syscall.EINTR {
				return
			}
			if count == 0 && pending == "" {
				continue
			}
			if count > 0 {
				pending += string(buffer[:count])
			}
			for pending != "" {
				size := 1
				var key string
				switch {
				case strings.HasPrefix(pending, "\x1b[A"):
					key, size = "up", 3
				case strings.HasPrefix(pending, "\x1b[B"):
					key, size = "down", 3
				case strings.HasPrefix(pending, "\x1b[") && len(pending) >= 3:
					key, size = "", 3
				case pending[0] == '\x1b':
					if count > 0 && (len(pending) == 1 || pending == "\x1b[") {
						break
					}
					key = "escape"
				case pending[0] == '\r' || pending[0] == '\n':
					key = "enter"
				case pending[0] == 127 || pending[0] == 8:
					key = "backspace"
				default:
					r, n := utf8.DecodeRuneInString(pending)
					if count > 0 && !utf8.FullRuneInString(pending) {
						break
					}
					if n == 0 && count > 0 {
						break
					}
					size = max(1, n)
					if unicode.IsPrint(r) {
						key = "text:" + string(r)
					}
				}
				if key == "" && (pending == "\x1b" || pending == "\x1b[") {
					break
				}
				pending = pending[size:]
				if key == "" {
					continue
				}
				select {
				case keys <- key:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return keys
}

func terminalArgs(choice terminalChoice, input string) ([]string, error) {
	args := slices.Clone(choice.args)
	if choice.prompt == "" {
		return args, nil
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, errors.New("an input value is required")
	}
	if len(args) < 2 || args[0] != "series" || (args[1] != "episodes" && args[1] != "download") {
		return append(args, input), nil
	}
	values := strings.Split(input, "|")
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}
	size := 2
	if args[1] == "download" {
		size = 3
	}
	if len(values) != size || values[0] == "" || values[1] == "" {
		return nil, errors.New("enter the requested values separated by |")
	}
	args = append(args, "--provider="+values[0], values[1])
	if size == 3 {
		if strings.EqualFold(values[2], "all") {
			args = append(args, "--all-episodes")
		} else {
			for _, id := range strings.Split(values[2], ",") {
				args = append(args, "--episode="+strings.TrimSpace(id))
			}
		}
	}
	return args, nil
}

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
	fd := int(os.Stdin.Fd())
	var saved syscall.Termios
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TCGETS, uintptr(unsafe.Pointer(&saved))); err != 0 {
		return err
	}
	raw := saved
	raw.Lflag &^= syscall.ECHO | syscall.ICANON
	raw.Cc[syscall.VMIN], raw.Cc[syscall.VTIME] = 0, 1
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TCSETS, uintptr(unsafe.Pointer(&raw))); err != 0 {
		return err
	}
	defer syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TCSETS, uintptr(unsafe.Pointer(&saved)))
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

	settings, _ := readEnvironment()
	values := make(map[string]string)
	startupValue := func(key, fallback string) string { return sessionValue(key, fallback, settings, nil) }
	timeout, err := time.ParseDuration(startupValue("STREAMFORGE_TIMEOUT", "30s"))
	if err != nil || timeout <= 0 {
		timeout = 30 * time.Second
	}
	verify, err := strconv.ParseBool(startupValue("STREAMFORGE_VERIFY_TLS", "true"))
	verify = err != nil || verify
	providerTransport := makeTransport(timeout, verify)
	defer providerTransport.CloseIdleConnections()
	providers, warnings, providerErr := xtream.LoadProviders("/config/providers.json", startupValue("STREAMFORGE_USER_AGENT", "VLC/3.0.20 LibVLC/3.0.20"), timeout, providerTransport)
	providerCache := &cachedProviders{providers: providers, err: providerErr, titles: media.NewTitles(startupValue("TMDB_API_KEY", ""), startupValue("STREAMFORGE_USER_AGENT", "VLC/3.0.20 LibVLC/3.0.20"), timeout, providerTransport, func(format string, args ...any) {
		fmt.Fprintf(session, "%s WARN %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
	})}
	for _, warning := range warnings {
		fmt.Fprintf(session, "%s WARN %s\n", time.Now().Format("2006-01-02 15:04:05"), warning)
	}
	startup := make(chan statusReport, 1)
	go func() { startup <- diagnosticStatus(ctx, session, nil) }()
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
	keys := terminalInput(inputContext, fd)
	redraw := time.NewTicker(250 * time.Millisecond)
	defer redraw.Stop()
	page, mode, filter, input := "Main", "menu", "", ""
	selection, scroll := 0, 0
	var choice terminalChoice
	var arguments []string
	var result string
	var discovered seriesDiscovery
	browser := terminalSeries{selected: make(map[seriesKey]map[string]string), movies: make(map[seriesKey]xtream.Movie)}
	if len(terminalSettings(settings, values)) > 1 {
		page = sessionSettings
	}
	var cancelOperation context.CancelFunc
	type operationResult struct {
		clipboard string
		discovery seriesDiscovery
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
		activeSelection = 0
		currentProgress := progress
		currentValues := maps.Clone(values)
		args := slices.Clone(arguments)
		if preview {
			args = append(args, "--dry-run")
		}
		retry := choice.label == "Retry Connections"
		copyReport := choice.label == "Copy Diagnostics"
		clipboard := copyReport && supportsClipboard()
		if copyReport && !clipboard {
			args = append(args, "--save")
		}
		go func() {
			defer cancel()
			buffer := &terminalBuffer{}
			var refreshed *statusReport
			if retry {
				result := diagnosticStatus(operation, session, currentValues)
				refreshed = &result
			}
			var discovery seriesDiscovery
			err := runCommand(operation, args, session, buffer, buffer, &discovery, currentProgress, currentValues, providerCache)
			var copied string
			if clipboard && err == nil {
				copied = base64.StdEncoding.EncodeToString(buffer.Bytes())
				buffer.Reset()
				fmt.Fprintln(buffer, "Sent diagnostics to the terminal clipboard.")
			}
			switch {
			case operation.Err() != nil:
				fmt.Fprintln(buffer, "Cancelled.")
			case err != nil:
				fmt.Fprintln(buffer, err)
			default:
				fmt.Fprintln(buffer, "Completed.")
			}
			finished <- operationResult{copied, discovery, buffer.String(), refreshed, preview, err != nil || operation.Err() != nil}
		}()
	}
	for {
		menu := browser.choices(page)
		if page == sessionSettings {
			menu = terminalSettings(settings, values)
		}
		if page != "Main" && page != sessionSettings {
			menu = append(slices.Clone(menu), terminalChoice{label: "Back to Previous", page: terminalPrevious(page)}, terminalChoice{label: "Back to Main", page: "Main"}, terminalChoice{label: "Exit"})
		}
		available := make([]terminalChoice, 0, len(menu))
		for _, item := range menu {
			if providerErr != nil && page == "Downloads" && (item.page == downloadQueue || (len(item.args) > 1 && item.args[1] != "transcode")) {
				item.label += " (disabled)"
				item.args = []string{"diagnostics"}
				item.prompt = ""
				item.page = ""
				item.confirm = false
			}
			if state, ok := status.Services[item.label]; ok && state != "connected" {
				item.label += " (disabled)"
				item.page = ""
				item.args = []string{"diagnostics"}
			}
			if strings.Contains(strings.ToLower(item.label), strings.ToLower(filter)) {
				available = append(available, item)
			}
		}
		if selection >= len(available) {
			selection = max(0, len(available)-1)
		}
		width, height := terminalDimensions()
		frame := []string{fmt.Sprintf("Stream Forge  |  %s  |  %s", status.Runtime, status.Encoder), fmt.Sprintf("Emby %s  |  Dispatcharr %s", strings.Fields(status.Services["Emby"])[0], strings.Fields(status.Services["Dispatcharr"])[0]), browser.heading(page), ""}
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
				frame = append(frame, marker+item.label)
			}
		case "input":
			shown := input
			if choice.secret {
				shown = strings.Repeat("•", utf8.RuneCountInString(input))
			}
			frame = append(frame, choice.label, "", choice.prompt, shown)
			footer = "Enter Continue   Esc Back   Ctrl+C Exit"
		case "confirm":
			frame = append(frame, "Apply "+choice.label+"?", "No (Enter)   Yes (Y)", "")
			lines := strings.Split(strings.TrimSpace(result), "\n")
			slots := max(1, height-len(frame)-1)
			scroll = min(scroll, max(0, len(lines)-slots))
			frame = append(frame, lines[scroll:min(len(lines), scroll+slots)]...)
			footer = "Y Confirm   Enter / Esc Cancel   Ctrl+C Exit"
		case "running":
			frame = append(frame, choice.label, "", fmt.Sprintf("Running for %s", time.Since(started).Truncate(time.Second)))
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
			if len(discovered.groups) > 0 || len(discovered.movies) > 0 || len(discovered.episodes) > 0 {
				footer = "B Browse   ↑↓ Scroll   Enter Return"
			}
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
			result = completion.text
			if completion.clipboard != "" {
				fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\a", completion.clipboard)
			}
			discovered = completion.discovery
			if completion.status != nil {
				status = *completion.status
			}
			mode, scroll = "result", 0
			if completion.preview && !completion.failed {
				mode = "confirm"
			} else if !completion.failed {
				if destination := browser.browse(discovered); destination != "" {
					navigate(destination)
				}
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
					} else if page != "Main" && page != sessionSettings {
						navigate(terminalPrevious(page))
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
					case choice.mark == "accept-settings":
						navigate("Main")
						if len(values) > 0 {
							choice = terminalChoice{label: "Retry Connections"}
							arguments = []string{"diagnostics"}
							launch(false)
						}
					case choice.mark != "":
						arguments = browser.mark(choice)
						if len(arguments) != 0 {
							launch(true)
						}
					case choice.page != "":
						navigate(choice.page)
					case choice.setting != "":
						mode, input = "input", ""
					case choice.args == nil:
						return nil
					case choice.prompt != "":
						mode, input = "input", ""
					default:
						arguments = slices.Clone(choice.args)
						if choice.confirm && startupValue("STREAMFORGE_CONFIRM", "true") != "false" {
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
					if choice.setting != "" {
						value := strings.TrimSpace(input)
						if choice.secret {
							value = input
						}
						if checkTerminalSetting(choice.setting, value) {
							values[choice.setting] = value
							input = ""
							mode = "menu"
						} else {
							mode, result, scroll = "result", "Invalid setting value", 0
						}
						break
					}
					arguments, err = terminalArgs(choice, input)
					if err != nil {
						mode, result, scroll = "result", err.Error(), 0
					} else if choice.confirm && startupValue("STREAMFORGE_CONFIRM", "true") != "false" {
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
					arguments = append(arguments, "--yes")
					launch(false)
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
				case "text:b", "text:B":
					if destination := browser.browse(discovered); destination != "" {
						navigate(destination)
					}
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
