// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package prompt

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
)

// InterruptExitCode is the conventional exit code of a process ended by SIGINT.
const InterruptExitCode = 130

// Exiter ends the process.
//
//mockery:generate: true
type Exiter interface {
	Exit(code int)
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

// interruptReader ends the process once the command context is cancelled while it waits for
// input. Prompts cannot return errors, and the root signal handler only cancels the command
// context, so without this an interrupt at a prompt would be swallowed until the next one. An
// interrupt that arrived before the prompt (for example while a package was being extracted)
// ends the process as well instead of being lost.
type interruptReader struct {
	ctx    context.Context
	reader io.Reader
	exiter Exiter
}

func (i *interruptReader) Read(p []byte) (int, error) {
	// The concrete reader passes io.EOF through unwrapped, as io.Reader callers compare it directly.
	n, err := (&contextReader{ctx: i.ctx, reader: i.reader}).Read(p)
	if i.ctx.Err() == nil {
		return n, err
	}

	log.Println("Interrupted")
	i.exiter.Exit(InterruptExitCode)

	return 0, io.EOF
}
