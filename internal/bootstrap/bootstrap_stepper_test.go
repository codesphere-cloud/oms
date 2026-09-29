// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func captureStepOutput(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = oldStdout })
	run()
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return string(output)
}

func TestStepLoggerTimer(t *testing.T) {
	timerPattern := regexp.MustCompile(`\([0-9]+(?:\.[0-9]+)?(?:ms|s)\)`)
	for _, test := range []struct {
		name      string
		timer     bool
		failure   bool
		substep   bool
		wantTimer bool
	}{
		{name: "default step"},
		{name: "timed step", timer: true, wantTimer: true},
		{name: "timed failed step", timer: true, failure: true, wantTimer: true},
		{name: "timed substep", timer: true, substep: true, wantTimer: true},
		{name: "timed failed substep", timer: true, substep: true, failure: true, wantTimer: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			logger := NewStepLogger(false)
			if test.timer {
				logger = NewStepLogger(false, WithTimer(true))
			}
			output := captureStepOutput(t, func() {
				step := logger.Step
				if test.substep {
					step = logger.Substep
				}
				err := step("Example", func() error {
					time.Sleep(5 * time.Millisecond)
					if test.failure {
						return errors.New("problem")
					}
					return nil
				})
				if (err != nil) != test.failure {
					t.Fatalf("unexpected step error: %v", err)
				}
			})
			if !strings.Contains(output, "Example") {
				t.Fatalf("step name missing from output: %q", output)
			}
			if timerPattern.MatchString(output) != test.wantTimer {
				t.Fatalf("timer output mismatch: %q", output)
			}
		})
	}
}
