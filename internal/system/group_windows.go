// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package system

import (
	"fmt"
	"os"
	"os/exec"
)

// setOwnProcessGroup does nothing on Windows, which has no process groups in the Unix sense.
func setOwnProcessGroup(*exec.Cmd) {}

// signalGroup signals only the process pid on Windows.
func signalGroup(pid int, sig os.Signal) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	return process.Signal(sig)
}
