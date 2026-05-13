// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/codesphere-cloud/cs-go/api"
	"github.com/codesphere-cloud/cs-go/api/openapi_client"
)

// Client interface abstracts Codesphere API operations for testing
//
//mockery:generate: true
type Client interface {
	CreateWorkspace(teamID, planID int, name string, repoURL *string) (workspaceID int, err error)
	SetEnvVar(workspaceID int, key, value string) error
	ExecuteCommand(workspaceID int, command string) error
	SyncLandscape(workspaceID int, profile string) error
	StartPipeline(workspaceID int, profile, stage string) error
	StopPipeline(workspaceID int, stage string) error
	GetPipelineState(workspaceID int, stage string) ([]api.PipelineStatus, error)
	TeardownLandscape(workspaceID int) error
	DeleteWorkspace(workspaceID int) error
	ListWorkspacePlans() ([]api.WorkspacePlan, error)
	ListTeams() ([]api.Team, error)
}

// APIClient wraps the cs-go API client
type APIClient struct {
	client *api.Client
	rawAPI *openapi_client.APIClient
	ctx    context.Context
}

// NewClient creates a new Codesphere API client
func NewClient(baseURL, token string) (*APIClient, error) {
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

	ctx := context.Background()
	client := api.NewClient(ctx, api.Configuration{
		BaseUrl: parsedURL,
		Token:   token,
	})

	cfg := openapi_client.NewConfiguration()
	cfg.Servers = []openapi_client.ServerConfiguration{{URL: parsedURL.String()}}
	rawAPI := openapi_client.NewAPIClient(cfg)
	rawCtx := context.WithValue(ctx, openapi_client.ContextAccessToken, token)

	return &APIClient{client: client, rawAPI: rawAPI, ctx: rawCtx}, nil
}

// CreateWorkspace creates a new workspace and waits for it to be running
func (c *APIClient) CreateWorkspace(teamID, planID int, name string, repoURL *string) (int, error) {
	workspace, err := c.client.DeployWorkspace(api.DeployWorkspaceArgs{
		TeamId:        teamID,
		PlanId:        planID,
		Name:          name,
		GitUrl:        repoURL,
		Timeout:       10 * time.Minute,
		EnvVars:       map[string]string{}, // Empty map to avoid null
		IsPrivateRepo: true,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create workspace: %w", err)
	}
	return workspace.Id, nil
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

// StopPipeline stops a running pipeline stage
func (c *APIClient) StopPipeline(workspaceID int, stage string) error {
	_, err := c.rawAPI.WorkspacesAPI.WorkspacesStopPipelineStage(c.ctx, float32(workspaceID), stage).Execute()
	if err != nil {
		return fmt.Errorf("failed to stop pipeline: %w", err)
	}
	return nil
}

// GetPipelineState returns the current status of a pipeline stage
func (c *APIClient) GetPipelineState(workspaceID int, stage string) ([]api.PipelineStatus, error) {
	return c.client.GetPipelineState(workspaceID, stage)
}

// TeardownLandscape tears down all running services in the workspace landscape
func (c *APIClient) TeardownLandscape(workspaceID int) error {
	_, err := c.rawAPI.WorkspacesAPI.WorkspacesTeardownLandscape(c.ctx, float32(workspaceID)).Execute()
	if err != nil {
		return fmt.Errorf("failed to teardown landscape: %w", err)
	}
	return nil
}

// DeleteWorkspace deletes a workspace
func (c *APIClient) DeleteWorkspace(workspaceID int) error {
	err := c.client.DeleteWorkspace(workspaceID)
	if err != nil {
		return fmt.Errorf("failed to delete workspace: %w", err)
	}
	return nil
}

// ListTeams lists the teams available
func (c *APIClient) ListTeams() ([]api.Team, error) {
	teams, err := c.client.ListTeams()
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
