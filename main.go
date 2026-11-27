// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kpirnie/streamforge/internal/app"
	"github.com/kpirnie/streamforge/internal/media"
)

var buildRevision string
var buildDate string
var buildTag string

//go:embed docs/assets/streamforge-app-icon.png
var appIcon []byte

func main() {
	media.RunCPULaunch(os.Args[1:])
	if len(os.Args) == 2 && os.Args[1] == "--icon" {
		if _, err := os.Stdout.Write(appIcon); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, os.Args[1:], buildRevision, buildDate, buildTag); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
