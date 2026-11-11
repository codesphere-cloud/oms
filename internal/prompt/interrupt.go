// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package prompt

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// interruptExitCode is the conventional exit code of a process ended by SIGINT.
const interruptExitCode = 130

// SignalNotifier returns a context that is cancelled when the process receives an interrupt.
//
//mockery:generate: true
type SignalNotifier interface {
	NotifyContext(parent context.Context) (context.Context, context.CancelFunc)
}

// Exiter ends the process.
//
//mockery:generate: true
type Exiter interface {
	Exit(code int)
}

// OSSignalNotifier notifies on SIGINT and SIGTERM.
type OSSignalNotifier struct{}

// NotifyContext returns a context cancelled on SIGINT or SIGTERM.
func (OSSignalNotifier) NotifyContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

// OSExiter ends the process with os.Exit.
type OSExiter struct{}

// Exit ends the process with the given code.
func (OSExiter) Exit(code int) {
	os.Exit(code)
}

// NewContextReader returns a reader that stops waiting for input once ctx is cancelled and then
// returns ctx.Err(). Use it for blocking stdin reads, which otherwise ignore cancellation.
func NewContextReader(ctx context.Context, r io.Reader) io.Reader {
	return &contextReader{ctx: ctx, reader: r}
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

type readResult struct {
	n   int
	err error
}

// Read reads into a separate buffer, so a read still pending after cancellation never writes
// into p once Read has returned.
func (c *contextReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, fmt.Errorf("stopped waiting for input: %w", err)
	}

	buf := make([]byte, len(p))
	done := make(chan readResult, 1)

	go func() {
		n, err := c.reader.Read(buf)
		done <- readResult{n: n, err: err}
	}()

	select {
	case result := <-done:
		copy(p, buf[:result.n])
		return result.n, result.err
	case <-c.ctx.Done():
		return 0, fmt.Errorf("stopped waiting for input: %w", c.ctx.Err())
	}
}

// interruptReader ends the process when it is interrupted while waiting for input. Prompts
// cannot return errors, and the root signal handler only cancels the command context, so
// without this an interrupt at a prompt would be swallowed until the next one.
type interruptReader struct {
	reader   io.Reader
	notifier SignalNotifier
	exiter   Exiter
}

func (i *interruptReader) Read(p []byte) (int, error) {
	ctx, stop := i.notifier.NotifyContext(context.Background())
	defer stop()

	// The concrete reader passes io.EOF through unwrapped, as io.Reader callers compare it directly.
	n, err := (&contextReader{ctx: ctx, reader: i.reader}).Read(p)
	if ctx.Err() == nil {
		return n, err
	}

	log.Println("Interrupted")
	i.exiter.Exit(interruptExitCode)

	return 0, io.EOF
}
