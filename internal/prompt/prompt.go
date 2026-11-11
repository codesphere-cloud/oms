// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package prompt asks the operator questions on stdin. A prompter can be non-interactive,
// in which case every question is answered with its default instead of being asked, which is
// what unattended runs (CI, --yes style flags) use.
package prompt

import (
	"bufio"
	"context"
	"log"
	"os"
	"strconv"
	"strings"
)

// Prompter asks the operator a question and returns their answer, falling back to the
// default whenever there is none: an empty line, a closed stdin, or a prompter that was
// created non-interactive.
//
//mockery:generate: true
type Prompter interface {
	String(prompt, defaultValue string) string
	Int(prompt string, defaultValue int) int
	StringSlice(prompt string, defaultValue []string) []string
	Bool(prompt string, defaultValue bool) bool
	Choice(prompt string, choices []string, defaultValue string) string
}

// StdinPrompter is the Prompter that asks on stdin.
type StdinPrompter struct {
	reader      *bufio.Reader
	interactive bool
	// ctx is the command context. It is checked before every question, because an answer
	// that is already buffered never reaches the interrupt-aware reader below the buffer.
	ctx    context.Context
	exiter Exiter
}

// NewPrompter returns a prompter reading from stdin. A non-interactive one never asks
// and answers every question with its default. An interrupt while it waits for an answer
// ends the process, as nothing is in progress that could be cancelled cleanly. So does a
// question asked after ctx was already cancelled, so an earlier interrupt is not lost.
func NewPrompter(ctx context.Context, interactive bool) *StdinPrompter {
	return &StdinPrompter{
		reader:      bufio.NewReader(&interruptReader{ctx: ctx, reader: os.Stdin, notifier: OSSignalNotifier{}, exiter: OSExiter{}}),
		interactive: interactive,
		ctx:         ctx,
		exiter:      OSExiter{},
	}
}

// exitIfInterrupted ends the process when the command was cancelled before a question is
// asked, so buffered answers (for example pasted ones) are not consumed after an interrupt.
func (p *StdinPrompter) exitIfInterrupted() {
	if p.ctx == nil || p.ctx.Err() == nil {
		return
	}

	log.Println("Interrupted")
	p.exiter.Exit(interruptExitCode)
}

// String asks for a line of text.
func (p *StdinPrompter) String(prompt, defaultValue string) string {
	if !p.interactive {
		return defaultValue
	}

	p.exitIfInterrupted()

	if defaultValue != "" {
		log.Printf("%s (default: %s): ", prompt, defaultValue)
	} else {
		log.Printf("%s: ", prompt)
	}

	input, _ := p.reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}

	return input
}

// Int asks for a number, falling back to the default when the answer is not one.
func (p *StdinPrompter) Int(prompt string, defaultValue int) int {
	if !p.interactive {
		return defaultValue
	}

	p.exitIfInterrupted()

	log.Printf("%s (default: %d): ", prompt, defaultValue)

	input, _ := p.reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(input)
	if err != nil {
		log.Printf("Invalid number, using default: %d\n", defaultValue)
		return defaultValue
	}

	return value
}

// StringSlice asks for a comma-separated list.
func (p *StdinPrompter) StringSlice(prompt string, defaultValue []string) []string {
	if !p.interactive {
		return defaultValue
	}

	p.exitIfInterrupted()

	defaultStr := strings.Join(defaultValue, ", ")
	if defaultStr != "" {
		log.Printf("%s (default: %s): ", prompt, defaultStr)
	} else {
		log.Printf("%s: ", prompt)
	}

	input, _ := p.reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}

	parts := strings.Split(input, ",")

	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return defaultValue
	}

	return result
}

// Bool asks a yes/no question. Only "y" and "yes" are a yes, only "n" and "no" a no;
// anything else falls back to the default.
func (p *StdinPrompter) Bool(prompt string, defaultValue bool) bool {
	if !p.interactive {
		return defaultValue
	}

	p.exitIfInterrupted()

	defaultStr := "n"
	if defaultValue {
		defaultStr = "y"
	}

	log.Printf("%s (y/n, default: %s): ", prompt, defaultStr)

	input, _ := p.reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		return defaultValue
	}

	return input == "y" || input == "yes"
}

// Choice asks for one of the given options, falling back to the default when the answer
// is not among them.
func (p *StdinPrompter) Choice(prompt string, choices []string, defaultValue string) string {
	if !p.interactive {
		return defaultValue
	}

	p.exitIfInterrupted()

	log.Printf("%s [%s] (default: %s): ", prompt, strings.Join(choices, "/"), defaultValue)

	input, _ := p.reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		return defaultValue
	}

	for _, choice := range choices {
		if strings.ToLower(choice) == input {
			return choice
		}
	}

	log.Printf("Invalid choice, using default: %s\n", defaultValue)

	return defaultValue
}
