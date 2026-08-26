// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package github

import (
	"context"
	"fmt"
)

// GetSSHKeysFromGitHubTeam fetches the public SSH keys of all members of the specified GitHub team and formats them for inclusion in instance metadata.
func GetSSHKeysFromGitHubTeam(client GitHubClient, org, teamSlug string) (string, error) {
	if org == "" || teamSlug == "" {
		return "", fmt.Errorf("GitHub team slug and org must be specified to fetch SSH keys from GitHub team")
	}

	members, err := client.GetTeamMemberSSHKeys(context.Background(), org, teamSlug)
	if err != nil {
		return "", fmt.Errorf("failed to fetch SSH keys from GitHub team: %w", err)
	}

	fmt.Printf("Found %d members in team '%s'\n", len(members), teamSlug)

	allKeys := ""
	for _, member := range members {
		for _, key := range member.Keys {
			allKeys += fmt.Sprintf("root:%s %sroot\nubuntu:%s %subuntu\n", key, member.Login, key, member.Login)
		}
	}

	return allKeys, nil
}
