// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBootstrap(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bootstrap Step Logger Suite")
}

func captureStepOutput(run func()) string {
	GinkgoHelper()
	reader, writer, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(reader.Close)
	DeferCleanup(func() { _ = writer.Close() })
	oldStdout := os.Stdout

	DeferCleanup(func() { os.Stdout = oldStdout })
	os.Stdout = writer
	run()
	os.Stdout = oldStdout

	Expect(writer.Close()).To(Succeed())
	output, err := io.ReadAll(reader)
	Expect(err).NotTo(HaveOccurred())
	return string(output)
}

var _ = Describe("Step logger", func() {
	const timerPattern = `\([0-9]+(?:\.[0-9]+)?(?:ms|s)\)`

	It("omits elapsed time by default", func() {
		output := captureStepOutput(func() {
			Expect(NewStepLogger(false).Step("Example", func() error { return nil })).To(Succeed())
		})
		Expect(output).To(ContainSubstring("Example"))
		Expect(output).NotTo(MatchRegexp(timerPattern))
	})

	It("shows elapsed time for a successful step", func() {
		output := captureStepOutput(func() {
			Expect(NewStepLogger(false, WithTimer(true)).Step("Example", func() error { return nil })).To(Succeed())
		})
		Expect(output).To(MatchRegexp(timerPattern))
	})

	It("preserves errors and times failed steps", func() {
		failure := errors.New("problem")
		output := captureStepOutput(func() {
			Expect(NewStepLogger(false, WithTimer(true)).Step("Example", func() error { return failure })).To(MatchError(failure))
		})
		Expect(output).To(ContainSubstring("Example failed: problem"))
		Expect(output).To(MatchRegexp(timerPattern))
	})

	It("shows elapsed time for substeps", func() {
		output := captureStepOutput(func() {
			Expect(NewStepLogger(false, WithTimer(true)).Substep("Example", func() error { return nil })).To(Succeed())
		})
		Expect(output).To(ContainSubstring("Example"))
		Expect(output).To(MatchRegexp(timerPattern))
	})

	It("preserves errors and times failed substeps", func() {
		failure := errors.New("problem")
		output := captureStepOutput(func() {
			Expect(NewStepLogger(false, WithTimer(true)).Substep("Example", func() error { return failure })).To(MatchError(failure))
		})
		Expect(output).To(ContainSubstring("Example failed: problem"))
		Expect(output).To(MatchRegexp(timerPattern))
	})

	It("summarizes nested steps in execution order, including failures", func() {
		logger := NewStepLogger(false, WithTimer(true))
		failure := errors.New("problem")
		output := captureStepOutput(func() {
			defer logger.PrintSummary()

			err := logger.Step("Deploy", func() error {
				if err := logger.Substep("Prepare", func() error { return nil }); err != nil {
					return err
				}

				return logger.Substep("Install", func() error { return failure })
			})
			Expect(err).To(MatchError(failure))
		})
		_, summary, found := strings.Cut(output, "Step timing summary:\n")
		Expect(found).To(BeTrue())
		Expect(summary).To(MatchRegexp(`(?m)^Step\s+Status\s+Duration$`))
		Expect(summary).To(MatchRegexp(`(?m)^Deploy\s+FAILED\s+.*\n  Prepare\s+OK\s+.*\n  Install\s+FAILED`))
		Expect(summary).To(MatchRegexp(`(?m)^Total\s+`))
	})

	It("excludes nested durations from the total", func() {
		logger := NewStepLogger(false, WithTimer(true))
		logger.timings = []stepTiming{
			{name: "Deploy", duration: 3 * time.Second},
			{name: "Install", duration: 2 * time.Second, depth: 1},
			{name: "Verify", duration: time.Second},
		}
		Expect(captureStepOutput(logger.PrintSummary)).To(MatchRegexp(`(?m)^Total\s+4s$`))
	})

	It("omits the summary when timing is disabled", func() {
		logger := NewStepLogger(false)
		output := captureStepOutput(func() {
			Expect(logger.Step("Deploy", func() error { return nil })).To(Succeed())
			logger.PrintSummary()
		})
		Expect(output).NotTo(ContainSubstring("Step timing summary"))
	})

	It("stays silent when timing is enabled on a silent logger", func() {
		logger := NewStepLogger(true, WithTimer(true))
		output := captureStepOutput(func() {
			Expect(logger.Step("Deploy", func() error { return nil })).To(Succeed())
			logger.PrintSummary()
		})
		Expect(output).To(BeEmpty())
	})

	It("omits the summary when no steps ran", func() {
		Expect(captureStepOutput(NewStepLogger(false, WithTimer(true)).PrintSummary)).To(BeEmpty())
	})
})
