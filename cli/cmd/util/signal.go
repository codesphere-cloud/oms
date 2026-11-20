// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/codesphere-cloud/oms/internal/prompt"
	"github.com/codesphere-cloud/oms/internal/system"
)

// SignalSource delivers the process signals that cancel a command, and forces the process to stop.
//
//mockery:generate: true
type SignalSource interface {
	Notify(c chan<- os.Signal)
	Stop(c chan<- os.Signal)
	// Force stops the process after a second signal sig, without waiting for running work.
	Force(sig os.Signal)
}

// OSSignalSource delivers SIGINT, SIGTERM and SIGHUP.
type OSSignalSource struct{}

// Notify relays SIGINT, SIGTERM and SIGHUP to c instead of terminating the process. SIGHUP is left
// alone when it is ignored, as under nohup, since catching it would undo that.
func (OSSignalSource) Notify(c chan<- os.Signal) {
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	if !signal.Ignored(syscall.SIGHUP) {
		signal.Notify(c, syscall.SIGHUP)
	}
}

// Stop restores default handling for the signals relayed to c.
func (OSSignalSource) Stop(c chan<- os.Signal) {
	signal.Stop(c)
}

// Force passes sig on to the commands running in their own process group, which the terminal does
// not signal, and exits with prompt.InterruptExitCode.
func (OSSignalSource) Force(sig os.Signal) {
	system.SignalOwnGroups(sig)
	os.Exit(prompt.InterruptExitCode)
}

// NotifySignalContext returns a copy of parent that is cancelled on the first SIGINT, SIGTERM or
// SIGHUP. A second signal forces the process to stop even while work that ignores the context is still
// running. Call stop to release the signal handler once the context is no longer needed.
func NotifySignalContext(parent context.Context) (ctx context.Context, stop context.CancelFunc) {
	return NewSignalContext(parent, OSSignalSource{}, os.Stderr)
}

// NewSignalContext works like NotifySignalContext with the given signal source. On the first signal
// it tells the operator on out that the command is stopping and how to force it.
func NewSignalContext(parent context.Context, source SignalSource, out io.Writer) (ctx context.Context, stop context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	signals := make(chan os.Signal, 1)
	stopped := make(chan struct{})
	source.Notify(signals)

	go func() {
		select {
		case <-signals:
		case <-ctx.Done():
			return
		}

		_, _ = fmt.Fprintln(out, "Interrupt received, stopping. Press Ctrl-C again to force.")

		cancel()

		select {
		case sig := <-signals:
			source.Stop(signals)
			source.Force(sig)
		case <-stopped:
		}
	}()

	var once sync.Once

	stop = func() {
		once.Do(func() {
			source.Stop(signals)
			close(stopped)
		})
		cancel()
	}

	return ctx, stop
}

// ExitCode returns the process exit code for the result of a command run with ctx: 0 on success,
// prompt.InterruptExitCode when the command failed after ctx was cancelled by a signal, and 1 otherwise.
// Call it before releasing ctx, since stop cancels it as well.
func ExitCode(ctx context.Context, err error) int {
	if err == nil {
		return 0
	}

	if ctx.Err() != nil {
		return prompt.InterruptExitCode
	}

	return 1
}
