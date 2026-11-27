// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

const (
	cpuLaunchMode    = "--streamforge-media-cpu-launch"
	cpuLaunchFailure = 125
)

type cpuLimitKey struct{}

var errCPUEnforcement = errors.New("could not establish CPU enforcement or launch the media executable")

func mediaCommandError(ctx context.Context, err error, message string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var exit *exec.ExitError
	if ctx.Value(cpuLimitKey{}) != nil && (!errors.As(err, &exit) || exit.ExitCode() == cpuLaunchFailure) {
		return fmt.Errorf("%w: %w", errCPUEnforcement, err)
	}
	return errors.New(message)
}
