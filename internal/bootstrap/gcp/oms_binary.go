// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// BuildOmsLinuxBinary returns the path to an OMS binary built for linux/amd64.
func BuildOmsLinuxBinary() (path string, cleanup func(), err error) {
	noop := func() {}

	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		execPath, err := os.Executable()
		if err != nil {
			return "", noop, fmt.Errorf("failed to locate current OMS binary: %w", err)
		}
		return execPath, noop, nil
	}

	// Cross-compile for linux/amd64 from the current working directory (project root).
	cwd, err := os.Getwd()
	if err != nil {
		return "", noop, fmt.Errorf("failed to determine project directory: %w", err)
	}

	outFile, err := os.CreateTemp("", "oms-linux-amd64-*")
	if err != nil {
		return "", noop, fmt.Errorf("failed to create temp file for OMS binary: %w", err)
	}
	if err = outFile.Close(); err != nil {
		return "", noop, fmt.Errorf("failed to close temp file for OMS binary: %w", err)
	}
	outPath := outFile.Name()
	rmCleanup := func() { _ = os.Remove(outPath) }

	cmd := exec.Command("go", "build", "-o", outPath, "./cli")
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	if output, cmdErr := cmd.CombinedOutput(); cmdErr != nil {
		rmCleanup()
		return "", noop, fmt.Errorf("failed to cross-compile OMS binary for linux/amd64: %w\n%s", cmdErr, output)
	}

	return outPath, rmCleanup, nil
}
