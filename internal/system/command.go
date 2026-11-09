// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package system wraps host commands such as container image tooling and installer subprocesses.
package system

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// InterruptGracePeriod is how long a cancelled command may take to clean up after SIGINT before it is killed.
const InterruptGracePeriod = 10 * time.Second

// CommandContext works like exec.CommandContext, but on cancellation it sends SIGINT instead of SIGKILL,
// so the child can clean up, and kills it only if it is still running after InterruptGracePeriod.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Cancel = func() error {
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = InterruptGracePeriod

	return cmd
}
