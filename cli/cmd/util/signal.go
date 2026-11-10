// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// NotifySignalContext returns a copy of parent that is cancelled on the first SIGINT or SIGTERM.
// Default signal handling is restored after that first signal, so a second one terminates the
// process even while work that ignores the context is still running. Call stop to release the
// signal handler once the context is no longer needed.
func NotifySignalContext(parent context.Context) (ctx context.Context, stop context.CancelFunc) {
	ctx, stop = signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	context.AfterFunc(ctx, stop)

	return ctx, stop
}
