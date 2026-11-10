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

// InterruptGracePeriod is how long a cancelled command may keep running after SIGINT before it is killed.
const InterruptGracePeriod = 10 * time.Second

// CommandContext works like exec.CommandContext, but on cancellation it sends SIGINT instead of SIGKILL
// and kills the process only if it is still running after InterruptGracePeriod.
//
// Only the started process is signalled, not its process group. Whether it cleans up depends on the
// program: docker and podman cancel cleanly on SIGINT, while the Node installer has no signal handler,
// exits immediately and leaves its own children (ssh, helm) running. A terminal Ctrl-C is unaffected,
// since the tty signals the whole foreground process group.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Cancel = func() error {
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = InterruptGracePeriod

	return cmd
}
