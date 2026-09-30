// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package bootstrap

// Note: In this file we need to use fmt.Print for the line reset.
// Line resets don't work with log.Print as expected.
import (
	"fmt"
	"time"
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
	config      StepLoggerConfig
	subSteps    int
	currentStep string
}

// StepLoggerConfig controls step logger output.
type StepLoggerConfig struct {
	Silent bool
	Timer  bool
}

// StepLoggerOption configures a step logger.
type StepLoggerOption func(*StepLoggerConfig)

// WithTimer controls whether completed steps show their elapsed time.
func WithTimer(enabled bool) StepLoggerOption {
	return func(config *StepLoggerConfig) {
		config.Timer = enabled
	}
}

// NewStepLogger creates a logger with the supplied options.
func NewStepLogger(silent bool, options ...StepLoggerOption) *StepLogger {
	config := StepLoggerConfig{Silent: silent}
	for _, option := range options {
		option(&config)
	}
	return &StepLogger{config: config}
}

func (b *StepLogger) elapsed(start time.Time) string {
	if !b.config.Timer {
		return ""
	}
	return fmt.Sprintf(" (%s)", time.Since(start).Round(time.Millisecond))
}

func (b *StepLogger) Step(name string, fn func() error) error {
	if b.config.Silent {
		return fn()
	}

	b.subSteps = 0
	b.currentStep = name

	fmt.Printf("%s%s%s...", LINE_RESET, RESET_TEXT, name)
	start := time.Now()
	err := fn()
	if err != nil {
		fmt.Printf("%s%s%s failed: %v%s%s\n", LINE_RESET, RED_TEXT, name, err, b.elapsed(start), RESET_TEXT)
	} else {
		for i := 0; i < b.subSteps; i++ {
			fmt.Printf("%s", MOVE_UP_CLEAR_LINE)
		}
		fmt.Printf("%s%s%s %s✓%s%s\n", LINE_RESET, RESET_TEXT, name, GREEN_TEXT, b.elapsed(start), RESET_TEXT)
	}
	return err
}

func (b *StepLogger) Substep(name string, fn func() error) error {
	if b.config.Silent {
		return fn()
	}

	b.subSteps += 1
	b.currentStep = name

	fmt.Printf("%s%s   %s...", LINE_RESET, RESET_TEXT, name)
	start := time.Now()
	err := fn()
	if err != nil {
		fmt.Printf("%s%s   %s failed: %v%s%s\n", LINE_RESET, RED_TEXT, name, err, b.elapsed(start), RESET_TEXT)
	} else {
		fmt.Printf("%s%s   %s %s✓%s%s\n", LINE_RESET, RESET_TEXT, name, GREEN_TEXT, b.elapsed(start), RESET_TEXT)
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
	if b.config.Silent {
		return
	}

	b.subSteps += 1
	fmt.Printf("%s%s      %s%s\n", LINE_RESET, RESET_TEXT, fmt.Sprintf(message, args...), RESET_TEXT)
}
