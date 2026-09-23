// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"
	"strings"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/codesphere/teststeps"
	"github.com/spf13/cobra"
)

type SmoketestCodesphereCmd struct {
	cmd           *cobra.Command
	GlobalOptions *util.GlobalOptions
	Opts          *teststeps.SmoketestCodesphereOpts
}

func (c *SmoketestCodesphereCmd) RunE(cmd *cobra.Command, _ []string) error {
	c.Opts.Quiet = !c.GlobalOptions.Verbose
	client, err := codesphere.NewClient(c.Opts.BaseURL, c.Opts.Token)
	if err != nil {
		return fmt.Errorf("failed to create Codesphere client: %w", err)
	}
	c.Opts.Client = client

	return teststeps.RunSmoketest(cmd.Context(), c.Opts)
}

func AddSmoketestCmd(parent *cobra.Command, opts *util.GlobalOptions) {
	c := SmoketestCodesphereCmd{
		cmd: &cobra.Command{
			Use:        "codesphere",
			Short:      "Run smoke tests for a Codesphere installation",
			Deprecated: "use 'oms test codesphere --tests smoketest' instead.",
			Long: io.Long(`Run automated smoke tests for a Codesphere installation by creating a workspace,
				setting environment variables, executing commands, syncing landscape, and running a pipeline stage.
				The workspace is automatically deleted after the test completes.`),
			Example: util.FormatExamples("smoketest codesphere", []io.Example{
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN",
					Desc: "Run smoke tests against a Codesphere installation",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --team-id TEAM_ID",
					Desc: "Run smoke tests against a specific team within your Codesphere installation",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --team-id TEAM_ID --plan-id PLAN_ID",
					Desc: "Run smoke tests against a specific team within your Codesphere installation, using a specific workspace plan",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --timeout 15m",
					Desc: "Run smoke tests with custom timeout",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --steps createWorkspace,syncLandscape",
					Desc: "Run only specific steps of the smoke test (workspace won't be deleted)",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --steps createWorkspace,syncLandscape,deleteWorkspace",
					Desc: "Run specific steps and delete the workspace afterwards",
				},
			}),
		},
		GlobalOptions: opts,
		Opts:          &teststeps.SmoketestCodesphereOpts{},
	}
	c.cmd.Flags().StringVar(&c.Opts.BaseURL, "baseurl", "", "Base URL of the Codesphere API")
	c.cmd.Flags().StringVar(&c.Opts.Token, "token", "", "API token for authentication")
	c.cmd.Flags().StringVar(&c.Opts.TeamID, "team-id", "", "Team ID for workspace creation")
	c.cmd.Flags().StringVar(&c.Opts.PlanID, "plan-id", "", "Plan ID for workspace creation")
	c.cmd.Flags().DurationVar(&c.Opts.Timeout, "timeout", teststeps.DefaultTimeout, "Timeout for the entire smoke test")
	c.cmd.Flags().StringVar(&c.Opts.Profile, "profile", teststeps.DefaultProfile, "CI profile to use for landscape and pipeline")
	c.cmd.Flags().StringSliceVar(&c.Opts.Steps, "steps", []string{}, fmt.Sprintf("Comma-separated list of steps to run (%s). If empty, all steps including deleteWorkspace are run. If specified without deleteWorkspace, the workspace will be kept for manual inspection.", strings.Join(teststeps.StepNames(), ",")))

	util.MarkFlagRequired(c.cmd, "baseurl")
	util.MarkFlagRequired(c.cmd, "token")

	c.cmd.RunE = c.RunE

	util.AddCmd(parent, c.cmd)
}
