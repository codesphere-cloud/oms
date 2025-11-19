// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Prompter struct {
	reader      *bufio.Reader
	interactive bool
}

func NewPrompter(interactive bool) *Prompter {
	return &Prompter{
		reader:      bufio.NewReader(os.Stdin),
		interactive: interactive,
	}
}

func (p *Prompter) String(prompt, defaultValue string) string {
	if !p.interactive {
		return defaultValue
	}

	if defaultValue != "" {
		fmt.Printf("%s (default: %s): ", prompt, defaultValue)
	} else {
		fmt.Printf("%s: ", prompt)
	}

	input, _ := p.reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}
	return input
}

func (p *Prompter) Int(prompt string, defaultValue int) int {
	if !p.interactive {
		return defaultValue
	}

	fmt.Printf("%s (default: %d): ", prompt, defaultValue)

	input, _ := p.reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(input)
	if err != nil {
		fmt.Printf("Invalid number, using default: %d\n", defaultValue)
		return defaultValue
	}
	return value
}

func (p *Prompter) StringSlice(prompt string, defaultValue []string) []string {
	if !p.interactive {
		return defaultValue
	}

	defaultStr := strings.Join(defaultValue, ", ")
	if defaultStr != "" {
		fmt.Printf("%s (default: %s): ", prompt, defaultStr)
	} else {
		fmt.Printf("%s: ", prompt)
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

func (p *Prompter) Bool(prompt string, defaultValue bool) bool {
	if !p.interactive {
		return defaultValue
	}

	defaultStr := "n"
	if defaultValue {
		defaultStr = "y"
	}
	fmt.Printf("%s (y/n, default: %s): ", prompt, defaultStr)

	input, _ := p.reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		return defaultValue
	}

	return input == "y" || input == "yes"
}

func (p *Prompter) Choice(prompt string, choices []string, defaultValue string) string {
	if !p.interactive {
		return defaultValue
	}

	fmt.Printf("%s [%s] (default: %s): ", prompt, strings.Join(choices, "/"), defaultValue)

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

	fmt.Printf("Invalid choice, using default: %s\n", defaultValue)
	return defaultValue
}
