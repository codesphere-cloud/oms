// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

const (
	defaultTimeout         = 10 * time.Minute
	defaultProfile         = "ci.yml"
	smoketestEnvVarKey     = "TEST_VAR"
	smoketestEnvVarValue   = "smoketest"
	smoketestPipelineStage = "run"

	// Step names
	stepCreateWorkspace = "createWorkspace"
	stepSetEnvVar       = "setEnvVar"
	stepCreateFiles     = "createFiles"
	stepSyncLandscape   = "syncLandscape"
	stepStartPipeline   = "startPipeline"
	stepDeleteWorkspace = "deleteWorkspace"

	ciYmlContent = `schemaVersion: v0.2
prepare:
  steps: []
test:
  steps: []
run:
  service:
    steps:
      - name: Run php server
        command: php -S 0.0.0.0:3000 index.html
    plan: 20
    replicas: 1
    network:
      ports:
        - port: 3000
          isPublic: true
      paths:
        - port: 3000
          path: /
    env: {}
`

	indexHtmlContent = `<!DOCTYPE html>
<html>
<head>
    <title>Smoketest</title>
</head>
<body>
    <h1>Smoketest Successful</h1>
</body>
</html>
`

	// ANSI color codes
	colorGreen = "\033[32m"
	colorRed   = "\033[31m"
	colorReset = "\033[0m"
)

type SmoketestCodesphereCmd struct {
	cmd    *cobra.Command
	Opts   *SmoketestCodesphereOpts
	Client codesphere.Client
}

type SmoketestCodesphereOpts struct {
	*GlobalOptions
	BaseURL string
	Token   string
	TeamID  string
	PlanID  string
	Quiet   bool
	Timeout time.Duration
	Profile string
	Steps   string
}

func (c *SmoketestCodesphereCmd) RunE(_ *cobra.Command, args []string) error {
	// Initialize client if not set (for testing)
	if c.Client == nil {
		client, err := codesphere.NewClient(c.Opts.BaseURL, c.Opts.Token)
		if err != nil {
			return fmt.Errorf("failed to create Codesphere client: %w", err)
		}
		c.Client = client
	}

	return c.RunSmoketest()
}

func AddSmoketestCodesphereCmd(parent *cobra.Command, opts *GlobalOptions) {
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
		Opts: &SmoketestCodesphereOpts{GlobalOptions: opts},
	}
	c.cmd.Flags().StringVar(&c.Opts.BaseURL, "baseurl", "", "Base URL of the Codesphere API")
	c.cmd.Flags().StringVar(&c.Opts.Token, "token", "", "API token for authentication")
	c.cmd.Flags().StringVar(&c.Opts.TeamID, "team-id", "", "Team ID for workspace creation")
	c.cmd.Flags().StringVar(&c.Opts.PlanID, "plan-id", "", "Plan ID for workspace creation")
	c.cmd.Flags().BoolVarP(&c.Opts.Quiet, "quiet", "q", false, "Suppress progress logging")
	c.cmd.Flags().DurationVar(&c.Opts.Timeout, "timeout", defaultTimeout, "Timeout for the entire smoke test")
	c.cmd.Flags().StringVar(&c.Opts.Profile, "profile", defaultProfile, "CI profile to use for landscape and pipeline")
	c.cmd.Flags().StringVar(&c.Opts.Steps, "steps", "", "Comma-separated list of steps to run (createWorkspace,setEnvVar,createFiles,syncLandscape,startPipeline,deleteWorkspace). If empty, all steps including deleteWorkspace are run. If specified without deleteWorkspace, the workspace will be kept for manual inspection.")

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

	teamID, parseErr := strconv.Atoi(c.Opts.TeamID)
	if parseErr != nil {
		return fmt.Errorf("invalid team-id: %w", parseErr)
	}
	planID, parseErr := strconv.Atoi(c.Opts.PlanID)
	if parseErr != nil {
		return fmt.Errorf("invalid plan-id: %w", parseErr)
	}
	steps := []string{stepCreateWorkspace, stepSetEnvVar, stepCreateFiles, stepSyncLandscape, stepStartPipeline, stepDeleteWorkspace}
	if c.Opts.Steps != "" {
		steps = strings.Split(c.Opts.Steps, ",")
		for i := range steps {
			steps[i] = strings.TrimSpace(steps[i])
		}
	}
	workspaceName := fmt.Sprintf("smoketest-%s", time.Now().Format("20060102-150405"))

	var workspaceID int
	defer func() {
		if err != nil {
			c.logf("\n%sSmoketest failed: %s%s\n", colorRed, err.Error(), colorReset)
		}

		if workspaceID != 0 && slices.Contains(steps, stepDeleteWorkspace) {
			c.logStep(fmt.Sprintf("\nDeleting workspace %d", workspaceID))
			deleteErr := c.Client.DeleteWorkspace(context.Background(), workspaceID)
			if deleteErr != nil {
				c.logFailure()
				if err == nil {
					err = fmt.Errorf("failed to delete workspace: %w", deleteErr)
				}
			}
			c.logSuccess()
		}

		if err == nil {
			c.logf("\n%sSmoketest completed successfully!%s\n", colorGreen, colorReset)
		}
	}()

	// Execute steps
	for _, step := range steps {
		switch step {
		case stepCreateWorkspace:
			if err = c.stepCreateWorkspace(ctx, teamID, planID, workspaceName, &workspaceID); err != nil {
				return err
			}
		case stepSetEnvVar:
			if err = c.stepSetEnvVar(ctx, workspaceID); err != nil {
				return err
			}
		case stepCreateFiles:
			if err = c.stepCreateFiles(ctx, workspaceID); err != nil {
				return err
			}
		case stepSyncLandscape:
			if err = c.stepSyncLandscape(ctx, workspaceID); err != nil {
				return err
			}
		case stepStartPipeline:
			if err = c.stepStartPipeline(ctx, workspaceID); err != nil {
				return err
			}
		case stepDeleteWorkspace:
			// Skip - handled in defer
			continue
		default:
			return fmt.Errorf("unknown step: %s", step)
		}
	}

	return nil
}

func (c *SmoketestCodesphereCmd) stepCreateWorkspace(ctx context.Context, teamID, planID int, workspaceName string, workspaceID *int) error {
	c.logStep(fmt.Sprintf("Creating empty workspace '%s'", workspaceName))
	id, err := c.Client.CreateWorkspace(ctx, teamID, planID, workspaceName, nil)
	if err != nil {
		c.logFailure()
		return fmt.Errorf("failed to create workspace: %w", err)
	}
	*workspaceID = id
	c.logSuccess()
	return nil
}

func (c *SmoketestCodesphereCmd) stepSetEnvVar(ctx context.Context, workspaceID int) error {
	c.logStep(fmt.Sprintf("Setting environment variable %s=%s", smoketestEnvVarKey, smoketestEnvVarValue))
	if err := c.Client.SetEnvVar(ctx, workspaceID, smoketestEnvVarKey, smoketestEnvVarValue); err != nil {
		c.logFailure()
		return fmt.Errorf("failed to set environment variable: %w", err)
	}
	c.logSuccess()
	return nil
}

func (c *SmoketestCodesphereCmd) stepCreateFiles(ctx context.Context, workspaceID int) error {
	c.logStep("Creating ci.yml file")
	ciYmlCmd := fmt.Sprintf(`echo '%s' > ci.yml`, ciYmlContent)
	err := c.Client.ExecuteCommand(ctx, workspaceID, ciYmlCmd)
	if err != nil {
		c.logFailure()
		return fmt.Errorf("failed to create ci.yml: %w", err)
	}
	c.logSuccess()

	c.logStep("Creating index.html file")
	indexHtmlCmd := fmt.Sprintf(`echo '%s' > index.html`, indexHtmlContent)
	err = c.Client.ExecuteCommand(ctx, workspaceID, indexHtmlCmd)
	if err != nil {
		c.logFailure()
		return fmt.Errorf("failed to create index.html: %w", err)
	}
	c.logSuccess()
	return nil
}

func (c *SmoketestCodesphereCmd) stepSyncLandscape(ctx context.Context, workspaceID int) error {
	c.logStep(fmt.Sprintf("Syncing landscape with profile '%s'", c.Opts.Profile))
	if err := c.Client.SyncLandscape(ctx, workspaceID, c.Opts.Profile); err != nil {
		c.logFailure()
		return fmt.Errorf("failed to sync landscape: %w", err)
	}
	c.logSuccess()
	return nil
}

func (c *SmoketestCodesphereCmd) stepStartPipeline(ctx context.Context, workspaceID int) error {
	c.logStep(fmt.Sprintf("Starting '%s' pipeline stage", smoketestPipelineStage))
	if err := c.Client.StartPipeline(ctx, workspaceID, c.Opts.Profile, smoketestPipelineStage); err != nil {
		c.logFailure()
		return fmt.Errorf("failed to start pipeline: %w", err)
	}
	c.logSuccess()
	return nil
}

// Logging helpers

func (c *SmoketestCodesphereCmd) logf(format string, args ...interface{}) {
	if !c.Opts.Quiet {
		fmt.Printf(format, args...)
	}
}

func (c *SmoketestCodesphereCmd) logStep(message string) {
	if !c.Opts.Quiet {
		fmt.Printf("%s...", message)
	}
}

func (c *SmoketestCodesphereCmd) logSuccess() {
	if !c.Opts.Quiet {
		fmt.Printf(" %ssucceeded%s\n", colorGreen, colorReset)
	}
}

func (c *SmoketestCodesphereCmd) logFailure() {
	if !c.Opts.Quiet {
		fmt.Printf(" %sfailed%s\n", colorRed, colorReset)
	}
}
