// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"context"
	"fmt"
	"net/url"

	"github.com/codesphere-cloud/cs-go/api"
)

// Client interface abstracts Codesphere API operations for testing
//
//mockery:generate: true
type Client interface {
	CreateWorkspace(teamID, planID int, name string, repoURL *string) (workspaceID int, err error)
	WorkspaceRunning(workspaceID int) (bool, error)
	SetEnvVar(workspaceID int, key, value string) error
	ExecuteCommand(workspaceID int, command string) error
	SyncLandscape(workspaceID int, profile string) error
	StartPipeline(workspaceID int, profile, stage string) error
	GetPipelineState(workspaceID int, stage string) ([]api.PipelineStatus, error)
	DeleteWorkspace(workspaceID int) error
	ListWorkspacePlans() ([]api.WorkspacePlan, error)
	ListTeams(orgId string) ([]api.Team, error)
}

// APIClient wraps the cs-go API client
type APIClient struct {
	client *api.Client
	// cleanupClient is not cancelled with the client's context, so a cancelled run can still
	// delete what it created.
	cleanupClient *api.Client
}

// NewClient creates a new Codesphere API client
func NewClient(ctx context.Context, baseURL, token string) (*APIClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}

	if token == "" {
		return nil, fmt.Errorf("token is required")
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid baseURL: %w", err)
	}

	config := api.Configuration{
		BaseUrl: parsedURL,
		Token:   token,
	}

	return &APIClient{
		client:        api.NewClient(ctx, config),
		cleanupClient: api.NewClient(context.WithoutCancel(ctx), config),
	}, nil
}

// CreateWorkspace creates a new workspace without waiting for it to be running. The cs-go
// DeployWorkspace waits as well, but its readiness loop ignores cancellation.
func (c *APIClient) CreateWorkspace(teamID, planID int, name string, repoURL *string) (int, error) {
	workspace, err := c.client.CreateWorkspace(api.CreateWorkspaceArgs{
		TeamId:        teamID,
		PlanId:        planID,
		Name:          name,
		GitUrl:        repoURL,
		IsPrivateRepo: true,
		Replicas:      1,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create workspace: %w", err)
	}

	return workspace.Id, nil
}

// WorkspaceRunning reports whether the workspace is running.
func (c *APIClient) WorkspaceRunning(workspaceID int) (bool, error) {
	status, err := c.client.WorkspaceStatus(workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workspace status: %w", err)
	}

	return status.IsRunning, nil
}

// SetEnvVar sets an environment variable in the workspace
func (c *APIClient) SetEnvVar(workspaceID int, key, value string) error {
	envVars := map[string]string{key: value}

	err := c.client.SetEnvVarOnWorkspace(workspaceID, envVars)
	if err != nil {
		return fmt.Errorf("failed to set environment variable: %w", err)
	}

	return nil
}

// ExecuteCommand executes a command in the workspace
func (c *APIClient) ExecuteCommand(workspaceID int, command string) error {
	_, _, err := c.client.ExecCommand(workspaceID, command, "", map[string]string{})
	if err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}

	return nil
}

// SyncLandscape syncs the landscape/CI configuration
func (c *APIClient) SyncLandscape(workspaceID int, profile string) error {
	err := c.client.DeployLandscape(workspaceID, profile)
	if err != nil {
		return fmt.Errorf("failed to sync landscape: %w", err)
	}

	return nil
}

// StartPipeline starts a pipeline stage
func (c *APIClient) StartPipeline(workspaceID int, profile, stage string) error {
	err := c.client.StartPipelineStage(workspaceID, profile, stage)
	if err != nil {
		return fmt.Errorf("failed to start pipeline: %w", err)
	}

	return nil
}

// GetPipelineState returns the current state of a pipeline stage
func (c *APIClient) GetPipelineState(workspaceID int, stage string) ([]api.PipelineStatus, error) {
	states, err := c.client.GetPipelineState(workspaceID, stage)
	if err != nil {
		return nil, fmt.Errorf("failed to get pipeline state: %w", err)
	}

	return states, nil
}

// DeleteWorkspace deletes a workspace. It runs on the cleanup client, so it still works after
// the client's context is cancelled.
func (c *APIClient) DeleteWorkspace(workspaceID int) error {
	err := c.cleanupClient.DeleteWorkspace(workspaceID)
	if err != nil {
		return fmt.Errorf("failed to delete workspace: %w", err)
	}

	return nil
}

// ListTeams lists the teams available
func (c *APIClient) ListTeams(orgId string) ([]api.Team, error) {
	teams, err := c.client.ListTeams(orgId)
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}

	return teams, nil
}

// ListWorkspacePlans lists the plans available
func (c *APIClient) ListWorkspacePlans() ([]api.WorkspacePlan, error) {
	plans, err := c.client.ListWorkspacePlans()
	if err != nil {
		return nil, fmt.Errorf("failed to list workspace plans: %w", err)
	}

	return plans, nil
}
