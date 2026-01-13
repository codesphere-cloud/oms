// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/codesphere/teststeps"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

const (
	defaultTimeout = 10 * time.Minute
	defaultProfile = "ci.yml"
)

var availableSteps = []teststeps.SmokeTestStep{
	&teststeps.CreateWorkspaceStep{},
	&teststeps.SetEnvVarStep{},
	&teststeps.CreateFilesStep{},
	&teststeps.SyncLandscapeStep{},
	&teststeps.StartPipelineStep{},
	&teststeps.DeleteWorkspaceStep{},
}

type SmoketestCodesphereCmd struct {
	cmd  *cobra.Command
	Opts *teststeps.SmoketestCodesphereOpts
}

func (c *SmoketestCodesphereCmd) RunE(_ *cobra.Command, args []string) error {
	// Initialize client if not set (for testing)
	if c.Opts.Client == nil {
		client, err := codesphere.NewClient(c.Opts.BaseURL, c.Opts.Token)
		if err != nil {
			return fmt.Errorf("failed to create Codesphere client: %w", err)
		}
		c.Opts.Client = client
	}

	return c.RunSmoketest()
}

func AddSmoketestCodesphereCmd(parent *cobra.Command, opts *GlobalOptions) {
	var stepNames []string
	for _, s := range availableSteps {
		stepNames = append(stepNames, s.Name())
	}

	c := SmoketestCodesphereCmd{
		cmd: &cobra.Command{
			Use:   "codesphere",
			Short: "Run smoke tests for a Codesphere installation",
			Long: io.Long(`Run automated smoke tests for a Codesphere installation by creating a workspace,
				setting environment variables, executing commands, syncing landscape, and running a pipeline stage.
				The workspace is automatically deleted after the test completes.`),
			Example: formatExamplesWithBinary("smoketest codesphere", []io.Example{
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --team-id TEAM_ID --plan-id PLAN_ID",
					Desc: "Run smoke tests against a Codesphere installation",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --team-id TEAM_ID --plan-id PLAN_ID --quiet",
					Desc: "Run smoke tests in quiet mode (no progress logging)",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --team-id TEAM_ID --plan-id PLAN_ID --timeout 15m",
					Desc: "Run smoke tests with custom timeout",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --team-id TEAM_ID --plan-id PLAN_ID --steps createWorkspace,syncLandscape",
					Desc: "Run only specific steps of the smoke test (workspace won't be deleted)",
				},
				{
					Cmd:  "--baseurl https://codesphere.example.com/api --token YOUR_TOKEN --team-id TEAM_ID --plan-id PLAN_ID --steps createWorkspace,syncLandscape,deleteWorkspace",
					Desc: "Run specific steps and delete the workspace afterwards",
				},
			}, "oms-cli"),
		},
		Opts: &teststeps.SmoketestCodesphereOpts{},
	}
	c.cmd.Flags().StringVar(&c.Opts.BaseURL, "baseurl", "", "Base URL of the Codesphere API")
	c.cmd.Flags().StringVar(&c.Opts.Token, "token", "", "API token for authentication")
	c.cmd.Flags().StringVar(&c.Opts.TeamID, "team-id", "", "Team ID for workspace creation")
	c.cmd.Flags().StringVar(&c.Opts.PlanID, "plan-id", "", "Plan ID for workspace creation")
	c.cmd.Flags().BoolVarP(&c.Opts.Quiet, "quiet", "q", false, "Suppress progress logging")
	c.cmd.Flags().DurationVar(&c.Opts.Timeout, "timeout", defaultTimeout, "Timeout for the entire smoke test")
	c.cmd.Flags().StringVar(&c.Opts.Profile, "profile", defaultProfile, "CI profile to use for landscape and pipeline")
	c.cmd.Flags().StringSliceVar(&c.Opts.Steps, "steps", []string{}, fmt.Sprintf("Comma-separated list of steps to run (%s). If empty, all steps including deleteWorkspace are run. If specified without deleteWorkspace, the workspace will be kept for manual inspection.", strings.Join(stepNames, ",")))

	util.MarkFlagRequired(c.cmd, "baseurl")
	util.MarkFlagRequired(c.cmd, "token")
	util.MarkFlagRequired(c.cmd, "team-id")
	util.MarkFlagRequired(c.cmd, "plan-id")

	c.cmd.RunE = c.RunE

	parent.AddCommand(c.cmd)
}

func (c *SmoketestCodesphereCmd) RunSmoketest() (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.Opts.Timeout)
	defer cancel()

	availableStepsMap := make(map[string]teststeps.SmokeTestStep)
	for _, s := range availableSteps {
		availableStepsMap[s.Name()] = s
	}

	stepsToRun := make([]teststeps.SmokeTestStep, len(availableSteps))
	copy(stepsToRun, availableSteps)

	if len(c.Opts.Steps) > 0 {
		stepsToRun = slices.DeleteFunc(stepsToRun, func(s teststeps.SmokeTestStep) bool {
			return !slices.Contains(c.Opts.Steps, s.Name())
		})
	}

	var workspaceID int
	deleteStep := &teststeps.DeleteWorkspaceStep{}
	defer func() {
		if err != nil {
			log.Printf("Smoketest failed: %s", err.Error())
		}

		shouldDelete := false
		for _, s := range stepsToRun {
			if s.Name() == deleteStep.Name() {
				shouldDelete = true
				break
			}
		}

		if workspaceID != 0 && shouldDelete {
			deleteErr := deleteStep.Run(context.Background(), c.Opts, &workspaceID)
			if deleteErr != nil {
				if err == nil {
					err = deleteErr
				}
			}
		}

		if err == nil {
			log.Println("Smoketest completed successfully!")
		}
	}()

	// Execute steps
	for _, step := range stepsToRun {
		// Skip deleteWorkspace in the main loop as it's handled in defer
		if step.Name() == deleteStep.Name() {
			continue
		}
		if err = step.Run(ctx, c.Opts, &workspaceID); err != nil {
			return err
		}
	}

	return nil
}
