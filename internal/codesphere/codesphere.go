// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/codesphere-cloud/cs-go/api"
)

// Client interface abstracts Codesphere API operations for testing
type Client interface {
	CreateWorkspace(ctx context.Context, teamID, planID int, name string, repoURL *string) (workspaceID int, err error)
	SetEnvVar(ctx context.Context, workspaceID int, key, value string) error
	ExecuteCommand(ctx context.Context, workspaceID int, command string) error
	SyncLandscape(ctx context.Context, workspaceID int, profile string) error
	StartPipeline(ctx context.Context, workspaceID int, profile, stage string) error
	DeleteWorkspace(ctx context.Context, workspaceID int) error
}

// APIClient wraps the cs-go API client
type APIClient struct {
	client *api.Client
}

// NewClient creates a new Codesphere API client
func NewClient(baseURL, token string) (Client, error) {
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

	return &APIClient{client: client}, nil
}

// CreateWorkspace creates a new workspace and waits for it to be running
func (c *APIClient) CreateWorkspace(ctx context.Context, teamID, planID int, name string, repoURL *string) (int, error) {
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
func (c *APIClient) SetEnvVar(ctx context.Context, workspaceID int, key, value string) error {
	envVars := map[string]string{key: value}
	err := c.client.SetEnvVarOnWorkspace(workspaceID, envVars)
	if err != nil {
		return fmt.Errorf("failed to set environment variable: %w", err)
	}
	return nil
}

// ExecuteCommand executes a command in the workspace
func (c *APIClient) ExecuteCommand(ctx context.Context, workspaceID int, command string) error {
	_, _, err := c.client.ExecCommand(workspaceID, command, "", map[string]string{})
	if err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}
	return nil
}

// SyncLandscape syncs the landscape/CI configuration
func (c *APIClient) SyncLandscape(ctx context.Context, workspaceID int, profile string) error {
	err := c.client.DeployLandscape(workspaceID, profile)
	if err != nil {
		return fmt.Errorf("failed to sync landscape: %w", err)
	}
	return nil
}

// StartPipeline starts a pipeline stage
func (c *APIClient) StartPipeline(ctx context.Context, workspaceID int, profile, stage string) error {
	err := c.client.StartPipelineStage(workspaceID, profile, stage)
	if err != nil {
		return fmt.Errorf("failed to start pipeline: %w", err)
	}
	return nil
}

// DeleteWorkspace deletes a workspace
func (c *APIClient) DeleteWorkspace(ctx context.Context, workspaceID int) error {
	err := c.client.DeleteWorkspace(workspaceID)
	if err != nil {
		return fmt.Errorf("failed to delete workspace: %w", err)
	}
	return nil
}
