// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package system wraps host commands such as container image tooling and installer subprocesses.
package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// InterruptGracePeriod is how long a cancelled command may keep running after SIGINT before it is killed.
const InterruptGracePeriod = 10 * time.Second

// CommandContext works like exec.CommandContext, but on cancellation it sends SIGINT instead of SIGKILL
// and kills the process only if it is still running after InterruptGracePeriod.
//
// On Linux the SIGINT also goes to every process the command started (for example the ssh and helm
// children of the Node installer), like a terminal Ctrl-C that signals the whole foreground process
// group. The started process stays in oms' process group, so it can still read from the terminal.
// The descendants are collected before the command itself is signalled, since the Node installer has
// no signal handler and exits immediately, which would hand its children over to init. Only the
// started process is killed after the grace period; on other systems only it is signalled at all.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Cancel = func() error {
		return interruptProcessTree(cmd.Process, descendantPIDs(hostProcFS(), cmd.Process.Pid))
	}
	cmd.WaitDelay = InterruptGracePeriod

	return cmd
}

// interruptProcessTree sends SIGINT to process and then to its descendants. Descendants that already
// exited are ignored. The returned error comes from signalling process and wraps os.ErrProcessDone when it
// already exited, which exec.Cmd treats as a successful cancellation.
func interruptProcessTree(process *os.Process, descendants []int) error {
	err := process.Signal(os.Interrupt)

	for _, pid := range descendants {
		if descendant, findErr := os.FindProcess(pid); findErr == nil {
			_ = descendant.Signal(os.Interrupt)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to interrupt process %d: %w", process.Pid, err)
	}

	return nil
}
