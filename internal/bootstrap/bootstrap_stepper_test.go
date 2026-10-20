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

func TestStepLoggerSummary(t *testing.T) {
	logger := NewStepLogger(false, WithTimer(true))
	failure := errors.New("problem")
	output := captureStepOutput(t, func() {
		defer logger.PrintSummary()
		err := logger.Step("Deploy", func() error {
			if err := logger.Substep("Prepare", func() error { return nil }); err != nil {
				return err
			}
			return logger.Substep("Install", func() error { return failure })
		})
		if !errors.Is(err, failure) {
			t.Fatalf("step error was not preserved: %v", err)
		}
	})
	_, summary, found := strings.Cut(output, "Step timing summary:\n")
	if !found {
		t.Fatalf("summary missing: %q", output)
	}
	lines := strings.Split(strings.TrimSpace(summary), "\n")
	wantRows := [][]string{
		{"Step", "Status", "Duration"},
		{"Deploy", "FAILED"},
		{"Prepare", "OK"},
		{"Install", "FAILED"},
		{"Total"},
	}
	if len(lines) != len(wantRows) {
		t.Fatalf("unexpected summary rows: %q", summary)
	}
	for i, want := range wantRows {
		fields := strings.Fields(lines[i])
		for j, value := range want {
			if len(fields) <= j || fields[j] != value {
				t.Fatalf("unexpected summary row: %q", lines[i])
			}
		}
	}
	if !strings.HasPrefix(lines[2], "  Prepare") || !strings.HasPrefix(lines[3], "  Install") {
		t.Fatalf("substeps are not indented: %q", summary)
	}
}

func TestStepLoggerSummaryTotalExcludesNestedDurations(t *testing.T) {
	logger := NewStepLogger(false, WithTimer(true))
	logger.timings = []stepTiming{
		{name: "Deploy", duration: 3 * time.Second},
		{name: "Install", duration: 2 * time.Second, depth: 1},
		{name: "Verify", duration: time.Second},
	}
	output := captureStepOutput(t, logger.PrintSummary)
	if !regexp.MustCompile(`(?m)^Total\s+4s$`).MatchString(output) {
		t.Fatalf("total must include only outer steps: %q", output)
	}
}

func TestStepLoggerSummaryDisabled(t *testing.T) {
	for _, test := range []struct {
		name   string
		logger *StepLogger
		run    bool
	}{
		{name: "timer disabled", logger: NewStepLogger(false), run: true},
		{name: "silent", logger: NewStepLogger(true, WithTimer(true)), run: true},
		{name: "no steps", logger: NewStepLogger(false, WithTimer(true))},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := captureStepOutput(t, func() {
				if test.run {
					_ = test.logger.Step("Deploy", func() error { return nil })
				}
				test.logger.PrintSummary()
			})
			if strings.Contains(output, "Step timing summary") {
				t.Fatalf("unexpected summary: %q", output)
			}
		})
	}
}
