// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

func RunCommand(command string, args []string, cmdDir string) error {
	cmd := newCommand(command, args, cmdDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed with exit status: %w", err)
	}
	return nil
}

// RunCommandWithOutput runs a command and returns its stdout as a string.
// Stderr is still forwarded to os.Stderr for visibility.
func RunCommandWithOutput(command string, args []string, cmdDir string) (string, error) {
	cmd := newCommand(command, args, cmdDir)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command failed with exit status: %w", err)
	}
	return stdout.String(), nil
}

// newCommand creates an exec.Cmd with context and optional working directory.
func newCommand(command string, args []string, cmdDir string) *exec.Cmd {
	cmd := exec.CommandContext(context.Background(), command, args...)
	if cmdDir != "" {
		cmd.Dir = cmdDir
	}
	return cmd
}
