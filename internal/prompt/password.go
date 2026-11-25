// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package prompt

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/term"
)

// Terminal reads passwords without echo and saves and restores the terminal mode.
//
//mockery:generate: true
type Terminal interface {
	GetState(fd int) (*term.State, error)
	Restore(fd int, state *term.State) error
	ReadPassword(fd int) ([]byte, error)
}

// OSTerminal is the Terminal backed by golang.org/x/term.
type OSTerminal struct{}

// GetState returns the current terminal mode of fd.
func (OSTerminal) GetState(fd int) (*term.State, error) {
	state, err := term.GetState(fd)
	if err != nil {
		return nil, fmt.Errorf("failed to get terminal state: %w", err)
	}

	return state, nil
}

// Restore sets the terminal mode of fd back to state.
func (OSTerminal) Restore(fd int, state *term.State) error {
	if err := term.Restore(fd, state); err != nil {
		return fmt.Errorf("failed to restore terminal state: %w", err)
	}

	return nil
}

// ReadPassword reads a line from fd without echo.
func (OSTerminal) ReadPassword(fd int) ([]byte, error) {
	password, err := term.ReadPassword(fd)
	if err != nil {
		return nil, fmt.Errorf("failed to read password: %w", err)
	}

	return password, nil
}

type passwordResult struct {
	password []byte
	err      error
}

// ReadPassword reads a password from fd and stops waiting once ctx is cancelled. Reading turns
// off echo, so on cancellation the terminal mode saved beforehand is restored; otherwise the
// shell would be left without echo after oms exits.
func ReadPassword(ctx context.Context, terminal Terminal, fd int) ([]byte, error) {
	state, err := terminal.GetState(fd)
	if err != nil {
		return nil, fmt.Errorf("failed to save terminal state: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("stopped waiting for password: %w", err)
	}

	done := make(chan passwordResult, 1)

	go func() {
		password, err := terminal.ReadPassword(fd)
		done <- passwordResult{password: password, err: err}
	}()

	select {
	case result := <-done:
		return result.password, result.err
	case <-ctx.Done():
		return nil, stopWaitingForPassword(ctx, terminal, fd, state)
	}
}

func stopWaitingForPassword(ctx context.Context, terminal Terminal, fd int, state *term.State) error {
	if err := terminal.Restore(fd, state); err != nil {
		return fmt.Errorf("stopped waiting for password, failed to restore terminal: %w", errors.Join(ctx.Err(), err))
	}

	return fmt.Errorf("stopped waiting for password: %w", ctx.Err())
}
