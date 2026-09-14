// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
)

const githubGraphQLEndpoint = "https://api.github.com/graphql"

// publicKeysPageSize is how many public SSH keys we request per team member. A user is very
// unlikely to have this many keys; totalCount lets us detect and log the rare case where they do.
const publicKeysPageSize = 20

// teamMemberSSHKeysQuery fetches every member of a team together with their public SSH keys in a
// single request. Members are paginated with the $after cursor; publicKeys are fetched in a single
// page of publicKeysPageSize and totalCount is used to detect truncation.
const teamMemberSSHKeysQuery = `query($org: String!, $team: String!, $after: String) {
  organization(login: $org) {
    team(slug: $team) {
      members(first: 100, after: $after) {
        pageInfo { hasNextPage endCursor }
        nodes {
          login
          publicKeys(first: 20) { totalCount nodes { key } }
        }
      }
    }
  }
}`

// TeamMemberKeys holds a team member's login and their public SSH keys.
type TeamMemberKeys struct {
	Login string
	Keys  []string
}

// GitHubClient abstracts the GitHub API call used to fetch team SSH keys.
//
//mockery:generate: true
type GitHubClient interface {
	GetTeamMemberSSHKeys(ctx context.Context, org, teamSlug string) ([]TeamMemberKeys, error)
}

type RealGitHubClient struct {
	httpClient *http.Client
	endpoint   string
}

// NewGitHubClient creates a new RealGitHubClient with the provided OAuth token.
func NewGitHubClient(ctx context.Context, token string) (*RealGitHubClient, error) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return &RealGitHubClient{
		httpClient: oauth2.NewClient(ctx, ts),
		endpoint:   githubGraphQLEndpoint,
	}, nil
}

// graphQLResponse mirrors the shape of the teamMemberSSHKeysQuery response.
type graphQLResponse struct {
	Data struct {
		Organization struct {
			Team struct {
				Members struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []struct {
						Login      string `json:"login"`
						PublicKeys struct {
							TotalCount int `json:"totalCount"`
							Nodes      []struct {
								Key string `json:"key"`
							} `json:"nodes"`
						} `json:"publicKeys"`
					} `json:"nodes"`
				} `json:"members"`
			} `json:"team"`
		} `json:"organization"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// GetTeamMemberSSHKeys fetches all members of the team and their public SSH keys via the GitHub
// GraphQL API, following member pagination until every member has been retrieved.
func (c *RealGitHubClient) GetTeamMemberSSHKeys(ctx context.Context, org, teamSlug string) ([]TeamMemberKeys, error) {
	var members []TeamMemberKeys
	var after *string

	for {
		resp, err := c.queryTeamMembers(ctx, org, teamSlug, after)
		if err != nil {
			return nil, err
		}

		team := resp.Data.Organization.Team
		for _, node := range team.Members.Nodes {
			if node.PublicKeys.TotalCount > publicKeysPageSize {
				fmt.Printf("User %s has %d public keys but only the first %d were fetched\n",
					node.Login, node.PublicKeys.TotalCount, publicKeysPageSize)
			}
			keys := make([]string, 0, len(node.PublicKeys.Nodes))
			for _, k := range node.PublicKeys.Nodes {
				keys = append(keys, k.Key)
			}
			members = append(members, TeamMemberKeys{Login: node.Login, Keys: keys})
		}

		if !team.Members.PageInfo.HasNextPage {
			break
		}
		cursor := team.Members.PageInfo.EndCursor
		after = &cursor
	}

	return members, nil
}

// queryTeamMembers executes a single page of the teamMemberSSHKeysQuery.
func (c *RealGitHubClient) queryTeamMembers(ctx context.Context, org, teamSlug string, after *string) (*graphQLResponse, error) {
	variables := map[string]any{"org": org, "team": teamSlug}
	if after != nil {
		variables["after"] = *after
	}

	body, err := json.Marshal(map[string]any{"query": teamMemberSSHKeysQuery, "variables": variables})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create GraphQL request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute GraphQL request: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read GraphQL response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GraphQL request failed with status %d: %s", httpResp.StatusCode, string(respBody))
	}

	var result graphQLResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GraphQL response: %w", err)
	}
	if len(result.Errors) > 0 {
		msgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("GraphQL query returned errors: %s", strings.Join(msgs, "; "))
	}

	return &result, nil
}
