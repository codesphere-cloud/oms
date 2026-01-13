// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package teststeps

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

const (
	smoketestEnvVarKey     = "TEST_VAR"
	smoketestEnvVarValue   = "smoketest"
	smoketestPipelineStage = "run"

	stepNameCreateWorkspace = "createWorkspace"
	stepNameSetEnvVar       = "setEnvVar"
	stepNameCreateFiles     = "createFiles"
	stepNameSyncLandscape   = "syncLandscape"
	stepNameStartPipeline   = "startPipeline"
	stepNameDeleteWorkspace = "deleteWorkspace"

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
)

type CreateWorkspaceStep struct{}

func (s *CreateWorkspaceStep) Name() string { return stepNameCreateWorkspace }

func (s *CreateWorkspaceStep) Run(ctx context.Context, c *SmoketestCodesphereOpts, workspaceID *int) error {
	teamID, parseErr := strconv.Atoi(c.TeamID)
	if parseErr != nil {
		return fmt.Errorf("invalid team-id: %w", parseErr)
	}
	planID, parseErr := strconv.Atoi(c.PlanID)
	if parseErr != nil {
		return fmt.Errorf("invalid plan-id: %w", parseErr)
	}
	workspaceName := fmt.Sprintf("smoketest-%s", time.Now().Format("20060102-150405"))

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

type SetEnvVarStep struct{}

func (s *SetEnvVarStep) Name() string { return stepNameSetEnvVar }

func (s *SetEnvVarStep) Run(ctx context.Context, c *SmoketestCodesphereOpts, workspaceID *int) error {
	c.logStep(fmt.Sprintf("Setting environment variable %s=%s", smoketestEnvVarKey, smoketestEnvVarValue))
	if err := c.Client.SetEnvVar(ctx, *workspaceID, smoketestEnvVarKey, smoketestEnvVarValue); err != nil {
		c.logFailure()
		return fmt.Errorf("failed to set environment variable: %w", err)
	}
	c.logSuccess()
	return nil
}

type CreateFilesStep struct{}

func (s *CreateFilesStep) Name() string { return stepNameCreateFiles }

func (s *CreateFilesStep) Run(ctx context.Context, c *SmoketestCodesphereOpts, workspaceID *int) error {
	c.logStep("Creating ci.yml file")
	ciYmlCmd := fmt.Sprintf(`echo '%s' > ci.yml`, ciYmlContent)
	err := c.Client.ExecuteCommand(ctx, *workspaceID, ciYmlCmd)
	if err != nil {
		c.logFailure()
		return fmt.Errorf("failed to create ci.yml: %w", err)
	}
	c.logSuccess()

	c.logStep("Creating index.html file")
	indexHtmlCmd := fmt.Sprintf(`echo '%s' > index.html`, indexHtmlContent)
	err = c.Client.ExecuteCommand(ctx, *workspaceID, indexHtmlCmd)
	if err != nil {
		c.logFailure()
		return fmt.Errorf("failed to create index.html: %w", err)
	}
	c.logSuccess()
	return nil
}

type SyncLandscapeStep struct{}

func (s *SyncLandscapeStep) Name() string { return stepNameSyncLandscape }

func (s *SyncLandscapeStep) Run(ctx context.Context, c *SmoketestCodesphereOpts, workspaceID *int) error {
	c.logStep(fmt.Sprintf("Syncing landscape with profile '%s'", c.Profile))
	if err := c.Client.SyncLandscape(ctx, *workspaceID, c.Profile); err != nil {
		c.logFailure()
		return fmt.Errorf("failed to sync landscape: %w", err)
	}
	c.logSuccess()
	return nil
}

type StartPipelineStep struct{}

func (s *StartPipelineStep) Name() string { return stepNameStartPipeline }

func (s *StartPipelineStep) Run(ctx context.Context, c *SmoketestCodesphereOpts, workspaceID *int) error {
	c.logStep(fmt.Sprintf("Starting '%s' pipeline stage", smoketestPipelineStage))
	if err := c.Client.StartPipeline(ctx, *workspaceID, c.Profile, smoketestPipelineStage); err != nil {
		c.logFailure()
		return fmt.Errorf("failed to start pipeline: %w", err)
	}
	c.logSuccess()
	return nil
}

type DeleteWorkspaceStep struct{}

func (s *DeleteWorkspaceStep) Name() string { return stepNameDeleteWorkspace }

func (s *DeleteWorkspaceStep) Run(ctx context.Context, c *SmoketestCodesphereOpts, workspaceID *int) error {
	c.logStep(fmt.Sprintf("\nDeleting workspace %d", *workspaceID))
	deleteErr := c.Client.DeleteWorkspace(ctx, *workspaceID)
	if deleteErr != nil {
		c.logFailure()
		return fmt.Errorf("failed to delete workspace: %w", deleteErr)
	}
	c.logSuccess()
	return nil
}
