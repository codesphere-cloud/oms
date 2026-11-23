// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package system

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// setOwnProcessGroup makes cmd start as the leader of a new process group.
func setOwnProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.Setpgid = true
}

// signalGroup sends sig to every process in the process group pgid.
func signalGroup(pgid int, sig os.Signal) error {
	s, ok := sig.(syscall.Signal)
	if !ok {
		return fmt.Errorf("unsupported signal %v", sig)
	}

	if err := syscall.Kill(-pgid, s); err != nil {
		return fmt.Errorf("failed to send %v to process group %d: %w", sig, pgid, err)
	}

	return nil
}
