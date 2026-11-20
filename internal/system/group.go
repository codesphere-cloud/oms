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

// GroupSignaller sends signals to process groups.
//
//mockery:generate: true
type GroupSignaller interface {
	SignalGroup(pgid int, sig os.Signal) error
}

// OSGroupSignaller signals process groups of the host.
type OSGroupSignaller struct{}

// SignalGroup sends sig to every process in the process group pgid.
func (OSGroupSignaller) SignalGroup(pgid int, sig os.Signal) error {
	return signalGroup(pgid, sig)
}

// ownGroups holds the process groups of the commands currently running through RunInOwnGroup.
var ownGroups = NewProcessGroups(OSGroupSignaller{})

// ProcessGroups tracks running process groups and whether each already got its interrupt, so that a
// cancellation and a forced stop together deliver exactly the signals the operator asked for. It is safe
// for concurrent use.
type ProcessGroups struct {
	signaller GroupSignaller

	mu sync.Mutex
	// interrupted maps each running group ID to whether it already got its SIGINT.
	interrupted map[int]bool
}

// NewProcessGroups returns an empty set of process groups signalled through signaller.
func NewProcessGroups(signaller GroupSignaller) *ProcessGroups {
	return &ProcessGroups{signaller: signaller, interrupted: map[int]bool{}}
}

// Add registers the running process group pgid. A group interrupted before it was added stays interrupted.
func (g *ProcessGroups) Add(pgid int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.interrupted[pgid]; !ok {
		g.interrupted[pgid] = false
	}
}

// Remove forgets the process group pgid once it exited.
func (g *ProcessGroups) Remove(pgid int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.interrupted, pgid)
}

// Interrupt sends SIGINT to the process group pgid unless it already got one. It wraps os.ErrProcessDone
// when the group already exited, which exec.Cmd treats as a successful cancellation.
func (g *ProcessGroups) Interrupt(pgid int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.interruptLocked(pgid)
}

// Force passes a forced stop with sig on to all registered groups. A group that did not get its SIGINT
// yet gets it first, so that sig is always its second signal, even when the forced stop overtakes the
// cancellation. Groups that already exited are ignored.
func (g *ProcessGroups) Force(sig os.Signal) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for pgid := range g.interrupted {
		if err := g.interruptLocked(pgid); err != nil {
			continue
		}

		_ = g.signaller.SignalGroup(pgid, sig)
	}
}

// interruptLocked works like Interrupt, with g.mu already held.
func (g *ProcessGroups) interruptLocked(pgid int) error {
	if g.interrupted[pgid] {
		return nil
	}

	g.interrupted[pgid] = true

	err := g.signaller.SignalGroup(pgid, os.Interrupt)
	if errors.Is(err, syscall.ESRCH) {
		err = os.ErrProcessDone
	}

	if err != nil {
		return fmt.Errorf("failed to interrupt process group %d: %w", pgid, err)
	}

	return nil
}

// RunInOwnGroup runs cmd like cmd.Run, but in a process group of its own, so that signals the terminal
// sends to oms' foreground process group (Ctrl-C, hangup) do not reach it. cmd must come from
// exec.CommandContext: when its context is cancelled, the whole group gets exactly one SIGINT, and the
// started process is killed if it is still running after InterruptGracePeriod.
//
// Since the terminal no longer signals the command, oms has to pass on a forced stop itself with
// ForceOwnGroups. The command cannot read from the terminal, so it must not prompt.
func RunInOwnGroup(cmd *exec.Cmd) error {
	setOwnProcessGroup(cmd)
	cmd.Cancel = func() error {
		return ownGroups.Interrupt(cmd.Process.Pid)
	}
	cmd.WaitDelay = InterruptGracePeriod

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s: %w", cmd.Path, err)
	}

	// The started process leads its group, so its PID is the group ID.
	pid := cmd.Process.Pid

	ownGroups.Add(pid)
	defer ownGroups.Remove(pid)

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s failed: %w", cmd.Path, err)
	}

	return nil
}

// ForceOwnGroups passes a forced stop with sig on to the process groups of all commands currently
// running through RunInOwnGroup. See ProcessGroups.Force.
func ForceOwnGroups(sig os.Signal) {
	ownGroups.Force(sig)
}
