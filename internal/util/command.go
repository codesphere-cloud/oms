package util

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func RunCommand(command string, args []string, cmdDir string) error {
	cmd := exec.CommandContext(context.Background(), command, args...)

	if cmdDir != "" {
		cmd.Dir = cmdDir
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("command failed with exit status %w", err)
	}

	return nil
}
