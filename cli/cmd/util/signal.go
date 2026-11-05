// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// CancelOnSignal wraps a RunE so that the command context is cancelled on SIGINT or SIGTERM.
// Only wrap commands that observe cmd.Context(): signals are intercepted while run executes,
// and default handling is restored after the first signal so a second one terminates the process.
func CancelOnSignal(run func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		parent := cmd.Context()

		ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
		defer stop()
		defer cmd.SetContext(parent)

		context.AfterFunc(ctx, stop)

		cmd.SetContext(ctx)

		return run(cmd, args)
	}
}
