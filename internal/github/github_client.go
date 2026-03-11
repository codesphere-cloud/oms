// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package github

import (
	"context"

	"github.com/google/go-github/v74/github"
	"golang.org/x/oauth2"
)

// GitHubClient abstracts the GitHub API calls used to fetch team SSH keys.
//
//mockery:generate: true
type GitHubClient interface {
	ListTeamMembersBySlug(ctx context.Context, org, teamSlug string, opts *github.TeamListTeamMembersOptions) ([]*github.User, error)
	ListUserKeys(ctx context.Context, username string) ([]*github.Key, error)
}

type RealGitHubClient struct {
	client *github.Client
}

func NewGitHubClient(ctx context.Context, token string) *RealGitHubClient {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	return &RealGitHubClient{client: github.NewClient(tc)}
}

func (c *RealGitHubClient) ListTeamMembersBySlug(ctx context.Context, org, teamSlug string, opts *github.TeamListTeamMembersOptions) ([]*github.User, error) {
	members, _, err := c.client.Teams.ListTeamMembersBySlug(ctx, org, teamSlug, opts)
	return members, err
}

func (c *RealGitHubClient) ListUserKeys(ctx context.Context, username string) ([]*github.Key, error) {
	keys, _, err := c.client.Users.ListKeys(ctx, username, nil)
	return keys, err
}
