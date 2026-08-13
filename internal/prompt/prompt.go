// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package prompt asks the operator questions on stdin. A prompter can be non-interactive,
// in which case every question is answered with its default instead of being asked, which is
// what unattended runs (CI, --yes style flags) use.
package prompt

import (
	"bufio"
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
}

// NewPrompter returns a prompter reading from stdin. A non-interactive one never asks
// and answers every question with its default.
func NewPrompter(interactive bool) *StdinPrompter {
	return &StdinPrompter{
		reader:      bufio.NewReader(os.Stdin),
		interactive: interactive,
	}
}

// String asks for a line of text.
func (p *StdinPrompter) String(prompt, defaultValue string) string {
	if !p.interactive {
		return defaultValue
	}

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
