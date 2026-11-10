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

// CancelOnSignal adapts run to a Cobra RunE whose context is cancelled on SIGINT or SIGTERM.
// Only use it for runs that observe ctx: signals are intercepted while run executes,
// and default handling is restored after the first signal so a second one terminates the process.
func CancelOnSignal(run func(ctx context.Context) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		context.AfterFunc(ctx, stop)

		return run(ctx)
	}
}
