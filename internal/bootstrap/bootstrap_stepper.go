// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"fmt"
)

const (
	LINE_RESET         = "\r\033[2K"
	MOVE_UP            = "\033[1A"
	MOVE_UP_CLEAR_LINE = "\033[1A\033[K"
	RESET_TEXT         = "\033[0m"
	RED_TEXT           = "\033[31m"
	GREEN_TEXT         = "\033[32m"
)

type StepLogger struct {
	silent      bool
	subSteps    int
	currentStep string
}

func NewStepLogger(silent bool) *StepLogger {
	return &StepLogger{
		silent: silent,
	}
}

func (b *StepLogger) Step(name string, fn func() error) error {
	if b.silent {
		return fn()
	}

	b.subSteps = 0
	b.currentStep = name

	fmt.Printf("%s%s%s...", LINE_RESET, RESET_TEXT, name)
	err := fn()
	if err != nil {
		fmt.Printf("%s%s%s failed: %v%s\n", LINE_RESET, RED_TEXT, name, err, RESET_TEXT)
	} else {
		for i := 0; i < b.subSteps; i++ {
			fmt.Printf("%s", MOVE_UP_CLEAR_LINE)
		}
		fmt.Printf("%s%s%s %s✓%s\n", LINE_RESET, RESET_TEXT, name, GREEN_TEXT, RESET_TEXT)
	}
	return err
}

func (b *StepLogger) Substep(name string, fn func() error) error {
	if b.silent {
		return fn()
	}

	b.subSteps += 1
	b.currentStep = name

	fmt.Printf("%s%s   %s...", LINE_RESET, RESET_TEXT, name)
	err := fn()
	if err != nil {
		fmt.Printf("%s%s   %s failed: %v%s\n", LINE_RESET, RED_TEXT, name, err, RESET_TEXT)
	} else {
		fmt.Printf("%s%s   %s %s✓%s\n", LINE_RESET, RESET_TEXT, name, GREEN_TEXT, RESET_TEXT)
	}
	return err
}

// LogRetry prints a retry message for the current step.
func (b *StepLogger) LogRetry() {
	if b.subSteps > 0 {
		fmt.Printf("%s%s   Retrying: %s...%s", LINE_RESET, RESET_TEXT, b.currentStep, RESET_TEXT)
	} else {
		fmt.Printf("%s%sRetrying: %s...%s", LINE_RESET, RESET_TEXT, b.currentStep, RESET_TEXT)
	}
}

// Logf prints a log message for the current step.
func (b *StepLogger) Logf(message string, args ...interface{}) {
	if b.silent {
		return
	}

	b.subSteps += 1
	fmt.Printf("%s%s      %s%s\n", LINE_RESET, RESET_TEXT, fmt.Sprintf(message, args...), RESET_TEXT)
}
