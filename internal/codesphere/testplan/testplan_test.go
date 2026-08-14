// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testplan_test

import (
	"bytes"
	"context"
	"fmt"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/codesphere/testplan"
)

// recordingTest records that it ran and returns a fixed error.
type recordingTest struct {
	name string
	err  error
	ran  *[]string
}

func (t *recordingTest) Name() string        { return t.name }
func (t *recordingTest) Description() string { return t.name + " description" }

func (t *recordingTest) Run(_ context.Context, out io.Writer) error {
	*t.ran = append(*t.ran, t.name)
	_, _ = fmt.Fprintf(out, "output of %s\n", t.name)

	return t.err
}

var _ = Describe("Testplan", func() {
	var (
		ran    []string
		out    *bytes.Buffer
		passes *recordingTest
		fails  *recordingTest
		second *recordingTest
	)

	newTest := func(name string, err error) *recordingTest {
		return &recordingTest{name: name, err: err, ran: &ran}
	}

	BeforeEach(func() {
		ran = []string{}
		out = &bytes.Buffer{}
		passes = newTest("passes", nil)
		fails = newTest("fails", fmt.Errorf("boom"))
		second = newTest("second", nil)
	})

	Describe("Registry", func() {
		var registry *testplan.Registry

		BeforeEach(func() {
			registry = testplan.NewRegistry(passes, fails, second)
			registry.AddPlaylist(testplan.Playlist{
				Name:  "default",
				Tests: []string{"fails", "passes"},
			})
		})

		It("lists tests and playlists in registration order", func() {
			Expect(registry.TestNames()).To(Equal([]string{"passes", "fails", "second"}))
			Expect(registry.PlaylistNames()).To(Equal([]string{"default"}))
		})

		It("selects tests in the requested order", func() {
			tests, err := registry.Select([]string{"second", "passes"})

			Expect(err).NotTo(HaveOccurred())
			Expect(tests).To(HaveLen(2))
			Expect(tests[0].Name()).To(Equal("second"))
			Expect(tests[1].Name()).To(Equal("passes"))
		})

		It("ignores duplicates in a selection", func() {
			tests, err := registry.Select([]string{"passes", "passes"})

			Expect(err).NotTo(HaveOccurred())
			Expect(tests).To(HaveLen(1))
		})

		It("reports unknown test names", func() {
			_, err := registry.Select([]string{"passes", "nope"})

			Expect(err).To(MatchError(ContainSubstring("unknown test(s) nope")))
			Expect(err).To(MatchError(ContainSubstring("passes,fails,second")))
		})

		It("returns an error for an empty selection", func() {
			_, err := registry.Select(nil)

			Expect(err).To(MatchError(ContainSubstring("no tests selected")))
		})

		It("resolves a playlist to its tests, keeping the playlist order", func() {
			tests, err := registry.SelectPlaylist("default")

			Expect(err).NotTo(HaveOccurred())
			Expect(tests[0].Name()).To(Equal("fails"))
			Expect(tests[1].Name()).To(Equal("passes"))
		})

		It("reports an unknown playlist", func() {
			_, err := registry.SelectPlaylist("nope")

			Expect(err).To(MatchError(ContainSubstring(`unknown playlist "nope"`)))
			Expect(err).To(MatchError(ContainSubstring("available playlists are default")))
		})

		It("reports a playlist that references an unknown test", func() {
			registry.AddPlaylist(testplan.Playlist{Name: "broken", Tests: []string{"nope"}})

			_, err := registry.SelectPlaylist("broken")

			Expect(err).To(MatchError(ContainSubstring(`playlist "broken"`)))
			Expect(err).To(MatchError(ContainSubstring("unknown test(s) nope")))
		})

		It("describes tests and playlists", func() {
			registry.Describe(out)

			Expect(out.String()).To(ContainSubstring("passes description"))
			Expect(out.String()).To(ContainSubstring("default"))
			Expect(out.String()).To(ContainSubstring("[fails, passes]"))
		})
	})

	Describe("Runner", func() {
		var runner *testplan.Runner

		BeforeEach(func() {
			runner = &testplan.Runner{Out: out}
		})

		It("runs all tests and reports their status", func() {
			results := runner.Run(context.Background(), []testplan.Test{passes, fails, second})

			Expect(ran).To(Equal([]string{"passes", "fails", "second"}))
			Expect(results).To(HaveLen(3))
			Expect(results[0].Status).To(Equal(testplan.StatusPassed))
			Expect(results[1].Status).To(Equal(testplan.StatusFailed))
			Expect(results[1].Err).To(MatchError("boom"))
			Expect(results[2].Status).To(Equal(testplan.StatusPassed))
		})

		It("continues after a failure by default", func() {
			runner.Run(context.Background(), []testplan.Test{fails, second})

			Expect(ran).To(Equal([]string{"fails", "second"}))
		})

		It("skips the remaining tests with fail-fast", func() {
			runner.FailFast = true

			results := runner.Run(context.Background(), []testplan.Test{fails, second})

			Expect(ran).To(Equal([]string{"fails"}))
			Expect(results).To(HaveLen(2))
			Expect(results[1].Name).To(Equal("second"))
			Expect(results[1].Status).To(Equal(testplan.StatusSkipped))
		})

		It("skips all tests when the context is already done", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			results := runner.Run(ctx, []testplan.Test{passes, second})

			Expect(ran).To(BeEmpty())
			Expect(results).To(HaveLen(2))

			for _, res := range results {
				Expect(res.Status).To(Equal(testplan.StatusSkipped))
				Expect(res.Err).To(MatchError(ContainSubstring("test run aborted")))
			}
		})

		It("forwards test output and logs progress", func() {
			runner.Run(context.Background(), []testplan.Test{passes})

			Expect(out.String()).To(ContainSubstring("passes description"))
			Expect(out.String()).To(ContainSubstring("output of passes"))
			Expect(out.String()).To(ContainSubstring("PASS"))
		})

		It("keeps test output but drops progress logging when quiet", func() {
			runner.Quiet = true

			runner.Run(context.Background(), []testplan.Test{passes})

			Expect(out.String()).To(ContainSubstring("output of passes"))
			Expect(out.String()).NotTo(ContainSubstring("passes description"))
		})
	})

	Describe("Summarize", func() {
		It("lists every result and tallies them", func() {
			results := []testplan.Result{
				{Name: "passes", Status: testplan.StatusPassed},
				{Name: "fails", Status: testplan.StatusFailed, Err: fmt.Errorf("boom")},
				{Name: "second", Status: testplan.StatusSkipped},
			}

			testplan.Summarize(out, results)

			Expect(out.String()).To(ContainSubstring("passes"))
			Expect(out.String()).To(ContainSubstring("boom"))
			Expect(out.String()).To(ContainSubstring("3 test(s): 1 passed, 1 failed, 1 skipped"))
		})
	})

	Describe("Err", func() {
		It("returns nil if nothing failed", func() {
			Expect(testplan.Err([]testplan.Result{
				{Name: "passes", Status: testplan.StatusPassed},
				{Name: "second", Status: testplan.StatusSkipped},
			})).To(BeNil())
		})

		It("names the failed tests", func() {
			err := testplan.Err([]testplan.Result{
				{Name: "passes", Status: testplan.StatusPassed},
				{Name: "fails", Status: testplan.StatusFailed},
			})

			Expect(err).To(MatchError("1 of 2 test(s) failed: fails"))
		})
	})
})
