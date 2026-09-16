// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package testplan runs ordered playlists of tests against a Codesphere
// installation and reports their results.
//
// A Test is a single, self-contained check (for example a status report or a
// smoke test). A Playlist is a named, ordered selection of those tests, so
// operators can run a well-known set of checks with a single command.
package testplan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"time"
)

const (
	// ANSI color codes
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorBold   = "\033[1m"
	colorReset  = "\033[0m"
)

// Status is the outcome of a single test run.
type Status string

// The outcomes a test can have. A test that was not run at all, because an
// earlier test failed or the run was cancelled, is skipped.
const (
	StatusPassed  Status = "PASS"
	StatusFailed  Status = "FAIL"
	StatusSkipped Status = "SKIP"
)

func (s Status) colored() string {
	switch s {
	case StatusPassed:
		return colorGreen + string(s) + colorReset
	case StatusFailed:
		return colorRed + string(s) + colorReset
	default:
		return colorYellow + string(s) + colorReset
	}
}

// Test is a single, independently runnable check of a Codesphere installation.
type Test interface {
	Name() string
	Description() string
	Run(ctx context.Context, out io.Writer) error
}

// Func adapts a plain function into a Test.
type Func struct {
	TestName string
	Desc     string
	Fn       func(ctx context.Context, out io.Writer) error
}

// Name returns the name the test is selected by.
func (f *Func) Name() string { return f.TestName }

// Description returns what the test does, as shown in listings and progress logs.
func (f *Func) Description() string { return f.Desc }

// Run executes the wrapped function.
func (f *Func) Run(ctx context.Context, out io.Writer) error {
	return f.Fn(ctx, out)
}

// Result records the outcome of a single test.
type Result struct {
	Name     string
	Status   Status
	Duration time.Duration
	Err      error
}

// Playlist is a named, ordered selection of tests.
type Playlist struct {
	Name        string
	Description string
	Tests       []string
}

// Registry holds the tests that can be run and the playlists that select them.
type Registry struct {
	tests     []Test
	playlists []Playlist
}

// NewRegistry returns a registry of the given tests, in the order they are
// passed. Tests keep that order unless a playlist specifies a different one.
func NewRegistry(tests ...Test) *Registry {
	return &Registry{tests: tests}
}

// AddPlaylist registers a named selection of tests.
func (r *Registry) AddPlaylist(p Playlist) {
	r.playlists = append(r.playlists, p)
}

// Tests returns all registered tests.
func (r *Registry) Tests() []Test {
	return slices.Clone(r.tests)
}

// Playlists returns all registered playlists.
func (r *Registry) Playlists() []Playlist {
	return slices.Clone(r.playlists)
}

// TestNames returns the names of all registered tests, in registration order.
func (r *Registry) TestNames() []string {
	names := make([]string, 0, len(r.tests))
	for _, t := range r.tests {
		names = append(names, t.Name())
	}

	return names
}

// PlaylistNames returns the names of all registered playlists.
func (r *Registry) PlaylistNames() []string {
	names := make([]string, 0, len(r.playlists))
	for _, p := range r.playlists {
		names = append(names, p.Name)
	}

	return names
}

// Select resolves test names to tests, keeping the requested order. Unknown
// names are reported instead of silently ignored, so a typo doesn't quietly
// shrink the test run.
func (r *Registry) Select(names []string) ([]Test, error) {
	if len(names) == 0 {
		return nil, errors.New("no tests selected")
	}

	byName := make(map[string]Test, len(r.tests))
	for _, t := range r.tests {
		byName[t.Name()] = t
	}

	selected := make([]Test, 0, len(names))

	var unknown []string

	for _, name := range names {
		test, ok := byName[name]
		if !ok {
			unknown = append(unknown, name)
			continue
		}

		if slices.ContainsFunc(selected, func(t Test) bool { return t.Name() == name }) {
			continue
		}

		selected = append(selected, test)
	}

	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown test(s) %s, available tests are %s",
			strings.Join(unknown, ","), strings.Join(r.TestNames(), ","))
	}

	return selected, nil
}

// SelectPlaylist resolves a playlist name to the tests it contains.
func (r *Registry) SelectPlaylist(name string) ([]Test, error) {
	idx := slices.IndexFunc(r.playlists, func(p Playlist) bool { return p.Name == name })
	if idx < 0 {
		return nil, fmt.Errorf("unknown playlist %q, available playlists are %s",
			name, strings.Join(r.PlaylistNames(), ","))
	}

	tests, err := r.Select(r.playlists[idx].Tests)
	if err != nil {
		return nil, fmt.Errorf("playlist %q: %w", name, err)
	}

	return tests, nil
}

// Describe writes the available tests and playlists in a human readable form.
func (r *Registry) Describe(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)

	printf(tw, "%sTests%s\n", colorBold, colorReset)

	for _, t := range r.tests {
		printf(tw, "  %s\t%s\n", t.Name(), t.Description())
	}

	printf(tw, "\n%sPlaylists%s\n", colorBold, colorReset)

	for _, p := range r.playlists {
		printf(tw, "  %s\t%s\t[%s]\n", p.Name, p.Description, strings.Join(p.Tests, ", "))
	}

	//nolint:errcheck // flushing to the command's output stream, nothing to recover from
	tw.Flush()
}

// Runner executes tests in order and reports what happened.
type Runner struct {
	// Out receives both the progress log and the output of the tests themselves.
	Out io.Writer
	// FailFast skips the remaining tests as soon as one fails.
	FailFast bool
	// Quiet suppresses the per-test progress log, but not the summary.
	Quiet bool
}

// Run executes the tests in order and returns one result per test. Tests that
// are not run (because of a failure with FailFast, or an expired context) are
// reported as skipped, so the result list always covers the full playlist.
func (r *Runner) Run(ctx context.Context, tests []Test) []Result {
	results := make([]Result, 0, len(tests))

	for i, test := range tests {
		if err := ctx.Err(); err != nil {
			results = append(results, skipRemaining(tests[i:], fmt.Errorf("test run aborted: %w", err))...)
			break
		}

		r.logf("\n%s▶ %s%s: %s\n", colorBold, test.Name(), colorReset, test.Description())

		start := time.Now()
		err := test.Run(ctx, r.Out)
		result := Result{Name: test.Name(), Duration: time.Since(start), Err: err}

		result.Status = StatusPassed
		if err != nil {
			result.Status = StatusFailed
		}

		results = append(results, result)

		r.logf("%s %s (%s)\n", test.Name(), result.Status.colored(), formatDuration(result.Duration))

		if err != nil && r.FailFast {
			results = append(results, skipRemaining(tests[i+1:], errors.New("skipped after earlier failure"))...)
			break
		}
	}

	return results
}

func (r *Runner) logf(format string, args ...any) {
	if r.Quiet || r.Out == nil {
		return
	}

	printf(r.Out, format, args...)
}

func skipRemaining(tests []Test, reason error) []Result {
	skipped := make([]Result, 0, len(tests))
	for _, t := range tests {
		skipped = append(skipped, Result{Name: t.Name(), Status: StatusSkipped, Err: reason})
	}

	return skipped
}

// Summarize writes a table of results followed by a one line tally.
func Summarize(w io.Writer, results []Result) {
	var (
		passed, failed, skipped int
		total                   time.Duration
	)
	for _, res := range results {
		total += res.Duration
		switch res.Status {
		case StatusPassed:
			passed++
		case StatusFailed:
			failed++
		default:
			skipped++
		}
	}

	printf(w, "\n%sTest results%s\n", colorBold, colorReset)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	for _, res := range results {
		detail := ""
		if res.Err != nil {
			detail = res.Err.Error()
		}

		printf(tw, "  %s\t%s\t%s\t%s\n", res.Status.colored(), res.Name, formatDuration(res.Duration), detail)
	}
	//nolint:errcheck // flushing to the command's output stream, nothing to recover from
	tw.Flush()

	printf(w, "\n%d test(s): %d passed, %d failed, %d skipped in %s\n",
		len(results), passed, failed, skipped, formatDuration(total))
}

// Err aggregates the failures of a test run into a single error, or returns
// nil if nothing failed.
func Err(results []Result) error {
	var failed []string

	for _, res := range results {
		if res.Status == StatusFailed {
			failed = append(failed, res.Name)
		}
	}

	if len(failed) == 0 {
		return nil
	}

	return fmt.Errorf("%d of %d test(s) failed: %s", len(failed), len(results), strings.Join(failed, ","))
}

// printf writes to the report output. Write errors are ignored: the output is
// the operator's terminal, and there is no fallback to report them on.
func printf(w io.Writer, format string, args ...any) {
	//nolint:errcheck // see above
	fmt.Fprintf(w, format, args...)
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}

	return d.Round(100 * time.Millisecond).String()
}
