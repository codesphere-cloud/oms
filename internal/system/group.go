// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package system

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

// ownGroups holds the process groups of the commands currently running through RunInOwnGroup.
var ownGroups = &processGroups{pids: map[int]struct{}{}}

// processGroups is a set of process group IDs, safe for concurrent use.
type processGroups struct {
	mu   sync.Mutex
	pids map[int]struct{}
}

func (g *processGroups) add(pid int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.pids[pid] = struct{}{}
}

func (g *processGroups) remove(pid int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.pids, pid)
}

func (g *processGroups) list() []int {
	g.mu.Lock()
	defer g.mu.Unlock()

	pids := make([]int, 0, len(g.pids))
	for pid := range g.pids {
		pids = append(pids, pid)
	}

	return pids
}

// RunInOwnGroup runs cmd like cmd.Run, but in a process group of its own, so that signals the terminal
// sends to oms' foreground process group (Ctrl-C, hangup) do not reach it. cmd must come from
// exec.CommandContext: when its context is cancelled, the whole group gets exactly one SIGINT, and the
// started process is killed if it is still running after InterruptGracePeriod.
//
// Since the terminal no longer signals the command, oms has to pass on a forced stop itself with
// SignalOwnGroups. The command cannot read from the terminal, so it must not prompt.
func RunInOwnGroup(cmd *exec.Cmd) error {
	setOwnProcessGroup(cmd)
	cmd.Cancel = func() error {
		return interruptGroup(cmd.Process.Pid)
	}
	cmd.WaitDelay = InterruptGracePeriod

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s: %w", cmd.Path, err)
	}

	// The started process leads its group, so its PID is the group ID.
	pid := cmd.Process.Pid

	ownGroups.add(pid)
	defer ownGroups.remove(pid)

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s failed: %w", cmd.Path, err)
	}

	return nil
}

// SignalOwnGroups sends sig to the process groups of all commands currently running through
// RunInOwnGroup. Groups that already exited are ignored.
func SignalOwnGroups(sig os.Signal) {
	for _, pid := range ownGroups.list() {
		_ = signalGroup(pid, sig)
	}
}

// interruptGroup sends SIGINT to the process group pgid. It wraps os.ErrProcessDone when the group
// already exited, which exec.Cmd treats as a successful cancellation.
func interruptGroup(pgid int) error {
	err := signalGroup(pgid, os.Interrupt)
	if errors.Is(err, syscall.ESRCH) {
		err = os.ErrProcessDone
	}

	if err != nil {
		return fmt.Errorf("failed to interrupt process group %d: %w", pgid, err)
	}

	return nil
}
