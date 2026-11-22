// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

type terminalChoice struct {
	label   string
	args    []string
	page    string
	prompt  string
	confirm bool
}

var terminalMenus = map[string][]terminalChoice{
	"Main": {
		{label: "Dispatcharr", page: "Dispatcharr"},
		{label: "Emby", page: "Emby"},
		{label: "Series", page: "Series"},
		{label: "Transcode media", args: []string{"movies", "transcode"}, prompt: "Media path"},
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
	"Series": {
		{label: "Search providers", args: []string{"series", "search"}, prompt: "Title"},
		{label: "List episodes", args: []string{"series", "episodes"}, prompt: "Provider name | Series ID"},
		{label: "Download series", args: []string{"series", "download"}, prompt: "Provider name | Series ID | Episode IDs (comma separated or all)", confirm: true},
		{label: "Download and process", args: []string{"series", "download", "--process"}, prompt: "Provider name | Series ID | Episode IDs (comma separated or all)", confirm: true},
		{label: "Transcode series", args: []string{"series", "transcode"}, prompt: "Media path"},
	},
	"System Status": {
		{label: "Show status", args: []string{"diagnostics"}},
		{label: "Retry Connections", args: []string{"diagnostics"}, page: ""},
		{label: "Save diagnostics", args: []string{"diagnostics", "--save"}},
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
	fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H\x1b[96m╔════════════════╗\r\n║  STREAM FORGE  ║\r\n╚════════════════╝\x1b[0m")
	session, err := openLog()
	if err != nil {
		return err
	}
	defer session.Close()

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
	keys := terminalInput(inputContext, fd)
	redraw := time.NewTicker(250 * time.Millisecond)
	defer redraw.Stop()
	page, mode, filter, input := "Main", "menu", "", ""
	selection, scroll := 0, 0
	var choice terminalChoice
	var arguments []string
	var result string
	var cancelOperation context.CancelFunc
	type operationResult struct {
		text   string
		status *statusReport
	}
	finished := make(chan operationResult, 1)
	var started time.Time
	if len(status.Findings) != 0 {
		mode, result = "startup", strings.Join(status.Findings, "\n")
	}
	navigate := func(destination string) {
		page, mode, filter, input, selection, scroll = destination, "menu", "", "", 0, 0
	}
	launch := func() {
		operation, cancel := context.WithCancel(ctx)
		cancelOperation = cancel
		mode, started = "running", time.Now()
		args := slices.Clone(arguments)
		retry := choice.label == "Retry Connections"
		go func() {
			defer cancel()
			buffer := &terminalBuffer{}
			var refreshed *statusReport
			if retry {
				result := diagnosticStatus(operation, session)
				refreshed = &result
			}
			err := runCommand(operation, args, session, buffer, buffer)
			switch {
			case operation.Err() != nil:
				fmt.Fprintln(buffer, "Cancelled.")
			case err != nil:
				fmt.Fprintln(buffer, err)
			default:
				fmt.Fprintln(buffer, "Completed.")
			}
			finished <- operationResult{buffer.String(), refreshed}
		}()
	}
	for {
		menu := terminalMenus[page]
		if page != "Main" {
			menu = append(slices.Clone(menu), terminalChoice{label: "Back to Previous", page: "Main"}, terminalChoice{label: "Back to Main", page: "Main"}, terminalChoice{label: "Exit"})
		}
		available := make([]terminalChoice, 0, len(menu))
		for _, item := range menu {
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
		frame := []string{fmt.Sprintf("Stream Forge  |  %s  |  Emby %s  |  Dispatcharr %s", status.Runtime, strings.Fields(status.Services["Emby"])[0], strings.Fields(status.Services["Dispatcharr"])[0]), "", page, ""}
		footer := "↑↓ Navigate   Enter Select   Esc Back   Ctrl+C Exit"
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
			frame = append(frame, choice.label, "", choice.prompt, input)
			footer = "Enter Continue   Esc Back   Ctrl+C Exit"
		case "confirm":
			frame = append(frame, "Apply "+choice.label+"?", "", strings.Join(choice.args, " "), "", "No (Enter)   Yes (Y)")
			footer = "Y Confirm   Enter / Esc Cancel   Ctrl+C Exit"
		case "running":
			frame = append(frame, choice.label, "", fmt.Sprintf("Running for %s", time.Since(started).Truncate(time.Second)))
			footer = "Esc Cancel operation   Ctrl+C Exit"
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
			result = completion.text
			if completion.status != nil {
				status = *completion.status
			}
			mode, scroll = "result", 0
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
					navigate("Main")
				}
			case "menu":
				switch {
				case key == "escape":
					if filter != "" {
						filter, selection = "", 0
					} else if page != "Main" {
						navigate("Main")
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
					case choice.page != "":
						navigate(choice.page)
					case choice.args == nil:
						return nil
					case choice.prompt != "":
						mode, input = "input", ""
					default:
						arguments = slices.Clone(choice.args)
						if choice.confirm {
							mode = "confirm"
						} else {
							launch()
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
					arguments, err = terminalArgs(choice, input)
					if err != nil {
						mode, result, scroll = "result", err.Error(), 0
					} else if choice.confirm {
						mode = "confirm"
					} else {
						launch()
					}
				}
			case "confirm":
				if key == "text:y" || key == "text:Y" {
					arguments = append(arguments, "--yes")
					launch()
				} else if key == "enter" || key == "escape" {
					mode = "menu"
				}
			case "running":
				if key == "escape" {
					cancelOperation()
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
