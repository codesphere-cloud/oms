// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"context"
	"fmt"
	"strings"
	"time"

	csio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/codesphere/status"
	"github.com/codesphere-cloud/oms/internal/codesphere/testplan"
	"github.com/codesphere-cloud/oms/internal/codesphere/testplan/steps"
	"github.com/codesphere-cloud/oms/internal/codesphere/teststeps"
	"github.com/spf13/cobra"
)

// defaultTestTimeout bounds the whole playlist, not a single step.
const defaultTestTimeout = 20 * time.Minute

// TestCodesphereOpts configures a test run against a Codesphere installation.
// The embedded options are the ones the playlist steps read; the rest selects
// what is run.
type TestCodesphereOpts struct {
	steps.Options

	Playlist string
	Tests    []string
	FailFast bool
}

// TestCodesphereCmd represents the test codesphere command.
type TestCodesphereCmd struct {
	cmd  *cobra.Command
	Opts *TestCodesphereOpts
}

// Registry returns the steps that can run against a Codesphere installation,
// together with the playlists that group them. The steps keep a reference to
// opts, so the registry can be built before the flags are parsed and the
// client is set, which is what the test list command does.
func Registry(opts *TestCodesphereOpts) *testplan.Registry {
	return steps.Registry(&opts.Options)
}

// selectTests resolves the requested steps. An explicit --tests selection wins
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
	c := TestCodesphereCmd{
		cmd: &cobra.Command{
			Use:   "codesphere",
			Short: "Run a playlist of tests against a Codesphere installation",
			Long: csio.Long(`Run a playlist of tests against a Codesphere installation.

				A playlist is an ordered selection of test steps, for example a status report
				followed by a smoke test. Every step is run even if an earlier one failed,
				unless --fail-fast is set, and the results are summarized at the end.

				Run 'oms test list' to see the available steps and playlists.

				This replaces 'oms smoketest codesphere', which is deprecated. The smoke test is
				the 'smoketest' step here, so 'oms test codesphere --tests smoketest' runs exactly
				what that command did.`),
			Example: util.FormatExamples("test codesphere", []csio.Example{
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN",
					Desc: fmt.Sprintf("Run the %q playlist against a Codesphere installation", steps.DefaultPlaylist),
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --playlist readiness",
					Desc: "Run a specific playlist",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --tests status,smoketest",
					Desc: "Run a specific list of steps, in the given order",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --wait",
					Desc: "Wait for the installation to become ready before running the remaining steps",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --fail-fast",
					Desc: "Stop at the first failing step instead of running the whole playlist",
				},
			}),
		},
		Opts: &TestCodesphereOpts{},
	}

	c.cmd.Flags().StringVar(&c.Opts.BaseURL, "baseurl", "", "Base URL of the Codesphere API")
	c.cmd.Flags().StringVar(&c.Opts.Token, "token", "", "API token for authentication")
	c.cmd.Flags().StringVar(&c.Opts.TeamID, "team-id", "", "Team ID to run tests in")
	c.cmd.Flags().StringVar(&c.Opts.PlanID, "plan-id", "", "Plan ID to use for workspaces created by tests")
	c.cmd.Flags().StringVar(&c.Opts.Profile, "profile", teststeps.DefaultProfile, "CI profile to use for landscape and pipeline")
	c.cmd.Flags().StringVar(&c.Opts.Playlist, "playlist", steps.DefaultPlaylist,
		fmt.Sprintf("Playlist of steps to run (%s)", strings.Join(steps.PlaylistNames(), ",")))
	c.cmd.Flags().StringSliceVar(&c.Opts.Tests, "tests", []string{},
		fmt.Sprintf("Comma-separated list of steps to run, in the given order (%s). Takes precedence over --playlist.", strings.Join(steps.Names(), ",")))
	c.cmd.Flags().BoolVar(&c.Opts.Wait, "wait", false, "Wait for the installation to become ready during the status step")
	c.cmd.Flags().DurationVar(&c.Opts.WaitTimeout, "wait-timeout", status.DefaultTimeout, "Timeout when waiting for the installation to become ready")
	c.cmd.Flags().DurationVar(&c.Opts.Timeout, "timeout", defaultTestTimeout, "Timeout for the entire test run")
	c.cmd.Flags().BoolVar(&c.Opts.FailFast, "fail-fast", false, "Skip the remaining steps after the first failure")
	c.cmd.Flags().BoolVarP(&c.Opts.Quiet, "quiet", "q", false, "Suppress progress logging")

	util.MarkFlagRequired(c.cmd, "baseurl")
	util.MarkFlagRequired(c.cmd, "token")

	c.cmd.RunE = c.RunE

	util.AddCmd(parent, c.cmd)
}
