// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// InterruptExitCode is the conventional exit code of a process ended by SIGINT.
const InterruptExitCode = 130

// SignalSource delivers the process signals that cancel a command.
//
//mockery:generate: true
type SignalSource interface {
	Notify(c chan<- os.Signal)
	Stop(c chan<- os.Signal)
}

// OSSignalSource delivers SIGINT and SIGTERM.
type OSSignalSource struct{}

// Notify relays SIGINT and SIGTERM to c instead of terminating the process.
func (OSSignalSource) Notify(c chan<- os.Signal) {
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
}

// Stop restores default handling for the signals relayed to c.
func (OSSignalSource) Stop(c chan<- os.Signal) {
	signal.Stop(c)
}

// NotifySignalContext returns a copy of parent that is cancelled on the first SIGINT or SIGTERM.
// Default signal handling is restored after that first signal, so a second one terminates the
// process even while work that ignores the context is still running. Call stop to release the
// signal handler once the context is no longer needed.
func NotifySignalContext(parent context.Context) (ctx context.Context, stop context.CancelFunc) {
	return NewSignalContext(parent, OSSignalSource{}, os.Stderr)
}

// NewSignalContext works like NotifySignalContext with the given signal source. On the first signal
// it tells the operator on out that the command is stopping and how to force it.
func NewSignalContext(parent context.Context, source SignalSource, out io.Writer) (ctx context.Context, stop context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	signals := make(chan os.Signal, 1)
	source.Notify(signals)

	go func() {
		select {
		case <-signals:
			source.Stop(signals)

			_, _ = fmt.Fprintln(out, "Interrupt received, stopping. Press Ctrl-C again to force.")

			cancel()
		case <-ctx.Done():
		}
	}()

	stop = func() {
		source.Stop(signals)
		cancel()
	}

	return ctx, stop
}

// ExitCode returns the process exit code for the result of a command run with ctx: 0 on success,
// InterruptExitCode when the command failed after ctx was cancelled by a signal, and 1 otherwise.
// Call it before releasing ctx, since stop cancels it as well.
func ExitCode(ctx context.Context, err error) int {
	if err == nil {
		return 0
	}

	if ctx.Err() != nil {
		return InterruptExitCode
	}

	return 1
}
