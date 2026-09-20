// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	csio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/codesphere/testplan"
	"github.com/codesphere-cloud/oms/internal/codesphere/teststeps"
	"github.com/spf13/cobra"
)

const (
	// defaultTestTimeout bounds the whole playlist, not a single test.
	defaultTestTimeout = 20 * time.Minute
	// DefaultPlaylist is run when neither --playlist nor --tests is given.
	DefaultPlaylist = "default"
)

// Names of the tests that can be part of a playlist.
const (
	StatusTestName    = "status"
	SmoketestTestName = "smoketest"
)

// TestCodesphereOpts configures a test run against a Codesphere installation.
type TestCodesphereOpts struct {
	BaseURL     string
	Token       string
	TeamID      string
	PlanID      string
	Profile     string
	Playlist    string
	Tests       []string
	Wait        bool
	WaitTimeout time.Duration
	Timeout     time.Duration
	FailFast    bool
	Quiet       bool
	Client      codesphere.Client
}

// TestCodesphereCmd represents the test codesphere command.
type TestCodesphereCmd struct {
	cmd  *cobra.Command
	Opts *TestCodesphereOpts
}

// Registry returns the tests that can run against a Codesphere installation,
// together with the playlists that group them. The tests close over opts, so
// the registry has to be built after the flags are parsed and the client is
// set. Building it with zero options is safe as long as no test is run, which
// is what the test list command does.
func Registry(opts *TestCodesphereOpts) *testplan.Registry {
	statusTest := &testplan.Func{
		TestName: StatusTestName,
		Desc:     "Report the state of the installation and verify the API answers",
		Fn: func(ctx context.Context, out io.Writer) error {
			waitTimeout := opts.WaitTimeout
			if waitTimeout <= 0 {
				waitTimeout = defaultStatusTimeout
			}

			statusOpts := &StatusCodesphereOpts{
				BaseURL: opts.BaseURL,
				Token:   opts.Token,
				Wait:    opts.Wait,
				Timeout: waitTimeout,
				Client:  opts.Client,
			}

			report := fetchStatus(ctx, statusOpts)
			printStatus(out, opts.BaseURL, report)

			if !report.Ready {
				if report.Err != nil {
					return fmt.Errorf("codesphere installation is not ready: %w", report.Err)
				}

				return fmt.Errorf("codesphere installation is not ready")
			}

			return nil
		},
	}

	smoketest := &testplan.Func{
		TestName: SmoketestTestName,
		Desc:     "Create a workspace, deploy a sample app in it and clean up afterwards",
		Fn: func(ctx context.Context, _ io.Writer) error {
			c := SmoketestCodesphereCmd{
				Opts: &teststeps.SmoketestCodesphereOpts{
					BaseURL: opts.BaseURL,
					Token:   opts.Token,
					TeamID:  opts.TeamID,
					PlanID:  opts.PlanID,
					Profile: opts.Profile,
					Quiet:   opts.Quiet,
					Timeout: opts.Timeout,
					Client:  opts.Client,
				},
			}

			return c.RunSmoketest(ctx)
		},
	}

	registry := testplan.NewRegistry(statusTest, smoketest)
	registry.AddPlaylist(testplan.Playlist{
		Name:        DefaultPlaylist,
		Description: "Verify the installation is up and can run a workspace",
		Tests:       []string{StatusTestName, SmoketestTestName},
	})
	registry.AddPlaylist(testplan.Playlist{
		Name:        "readiness",
		Description: "Only check that the installation is reachable and ready",
		Tests:       []string{StatusTestName},
	})

	return registry
}

// selectTests resolves the requested tests. An explicit --tests selection wins
// over --playlist, so a playlist default doesn't have to be unset first.
func (c *TestCodesphereCmd) selectTests() ([]testplan.Test, error) {
	registry := Registry(c.Opts)

	if len(c.Opts.Tests) > 0 {
		tests, err := registry.Select(c.Opts.Tests)
		if err != nil {
			return nil, fmt.Errorf("failed to select tests: %w", err)
		}

		return tests, nil
	}

	tests, err := registry.SelectPlaylist(c.Opts.Playlist)
	if err != nil {
		return nil, fmt.Errorf("failed to select playlist: %w", err)
	}

	return tests, nil
}

// RunE runs the selected tests and fails the command if any of them failed.
func (c *TestCodesphereCmd) RunE(cmd *cobra.Command, _ []string) error {
	tests, err := c.selectTests()
	if err != nil {
		return err
	}

	client, err := codesphere.NewClient(c.Opts.BaseURL, c.Opts.Token)
	if err != nil {
		return fmt.Errorf("failed to create Codesphere client: %w", err)
	}

	c.Opts.Client = client

	ctx, cancel := context.WithTimeout(cmd.Context(), c.Opts.Timeout)
	defer cancel()

	out := cmd.OutOrStdout()
	runner := &testplan.Runner{
		Out:      out,
		FailFast: c.Opts.FailFast,
		Quiet:    c.Opts.Quiet,
	}

	results := runner.Run(ctx, tests)
	testplan.Summarize(out, results)

	if err := testplan.Err(results); err != nil {
		return fmt.Errorf("test run failed: %w", err)
	}

	return nil
}

// AddTestCmd adds the test codesphere command to the given parent command.
func AddTestCmd(parent *cobra.Command, _ *util.GlobalOptions) {
	registry := Registry(&TestCodesphereOpts{})

	c := TestCodesphereCmd{
		cmd: &cobra.Command{
			Use:   "codesphere",
			Short: "Run a playlist of tests against a Codesphere installation",
			Long: csio.Long(`Run a playlist of tests against a Codesphere installation.

				A playlist is an ordered selection of tests, for example a status report
				followed by a smoke test. Every test is run even if an earlier one failed,
				unless --fail-fast is set, and the results are summarized at the end.

				Run 'oms test list' to see the available tests and playlists.`),
			Example: util.FormatExamples("test codesphere", []csio.Example{
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN",
					Desc: fmt.Sprintf("Run the %q playlist against a Codesphere installation", DefaultPlaylist),
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --playlist readiness",
					Desc: "Run a specific playlist",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --tests status,smoketest",
					Desc: "Run a specific list of tests, in the given order",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --wait",
					Desc: "Wait for the installation to become ready before running the remaining tests",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --fail-fast",
					Desc: "Stop at the first failing test instead of running the whole playlist",
				},
			}),
		},
		Opts: &TestCodesphereOpts{},
	}

	c.cmd.Flags().StringVar(&c.Opts.BaseURL, "baseurl", "", "Base URL of the Codesphere API")
	c.cmd.Flags().StringVar(&c.Opts.Token, "token", "", "API token for authentication")
	c.cmd.Flags().StringVar(&c.Opts.TeamID, "team-id", "", "Team ID to run tests in")
	c.cmd.Flags().StringVar(&c.Opts.PlanID, "plan-id", "", "Plan ID to use for workspaces created by tests")
	c.cmd.Flags().StringVar(&c.Opts.Profile, "profile", defaultProfile, "CI profile to use for landscape and pipeline")
	c.cmd.Flags().StringVar(&c.Opts.Playlist, "playlist", DefaultPlaylist,
		fmt.Sprintf("Playlist of tests to run (%s)", strings.Join(registry.PlaylistNames(), ",")))
	c.cmd.Flags().StringSliceVar(&c.Opts.Tests, "tests", []string{},
		fmt.Sprintf("Comma-separated list of tests to run, in the given order (%s). Takes precedence over --playlist.", strings.Join(registry.TestNames(), ",")))
	c.cmd.Flags().BoolVar(&c.Opts.Wait, "wait", false, "Wait for the installation to become ready during the status test")
	c.cmd.Flags().DurationVar(&c.Opts.WaitTimeout, "wait-timeout", defaultStatusTimeout, "Timeout when waiting for the installation to become ready")
	c.cmd.Flags().DurationVar(&c.Opts.Timeout, "timeout", defaultTestTimeout, "Timeout for the entire test run")
	c.cmd.Flags().BoolVar(&c.Opts.FailFast, "fail-fast", false, "Skip the remaining tests after the first failure")
	c.cmd.Flags().BoolVarP(&c.Opts.Quiet, "quiet", "q", false, "Suppress progress logging")

	util.MarkFlagRequired(c.cmd, "baseurl")
	util.MarkFlagRequired(c.cmd, "token")

	c.cmd.RunE = c.RunE

	util.AddCmd(parent, c.cmd)
}
