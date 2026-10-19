// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package github

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestGitHubClient(response string) *RealGitHubClient {
	return &RealGitHubClient{
		httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(response)),
				Header:     make(http.Header),
			}, nil
		})},
		endpoint: "https://example.invalid/graphql",
	}
}

func TestGetTeamMemberSSHKeysReportsInvisibleOrganization(t *testing.T) {
	client := newTestGitHubClient(`{"data":{"organization":null}}`)

	_, err := client.GetTeamMemberSSHKeys(context.Background(), "codesphere-cloud", "dev")
	if err == nil || !strings.Contains(err.Error(), `organization "codesphere-cloud" was not found or is not visible`) {
		t.Fatalf("expected an organization visibility error, got %v", err)
	}
}

func TestGetTeamMemberSSHKeysReportsInvisibleTeam(t *testing.T) {
	client := newTestGitHubClient(`{"data":{"organization":{"team":null}}}`)

	_, err := client.GetTeamMemberSSHKeys(context.Background(), "codesphere-cloud", "dev")
	if err == nil || !strings.Contains(err.Error(), `team "dev" was not found or is not visible`) {
		t.Fatalf("expected a team visibility error, got %v", err)
	}
}

func TestGetTeamMemberSSHKeysAllowsEmptyTeam(t *testing.T) {
	client := newTestGitHubClient(`{"data":{"organization":{"team":{"members":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}}}}`)

	members, err := client.GetTeamMemberSSHKeys(context.Background(), "codesphere-cloud", "dev")
	if err != nil {
		t.Fatalf("expected an empty team to be valid, got %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("expected no members, got %d", len(members))
	}
}
