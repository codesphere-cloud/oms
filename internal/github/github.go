package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v74/github"
)

// GetSSHKeysFromGitHubTeam fetches the public SSH keys of all members of the specified GitHub team and formats them for inclusion in instance metadata.
func GetSSHKeysFromGitHubTeam(client GitHubClient, org, teamSlug string) (string, error) {
	if org == "" || teamSlug == "" {
		return "", fmt.Errorf("GitHub team slug and org must be specified to fetch SSH keys from GitHub team")
	}
	allKeys := ""

	allMembers, err := listAllGitHubTeamMembers(client, org, teamSlug)
	if err != nil {
		return "", fmt.Errorf("failed to list GitHub team members: %w", err)
	}

	fmt.Printf("Found %d members in team '%s'\n", len(allMembers), teamSlug)

	for _, user := range allMembers {
		username := user.GetLogin()
		keys, err := client.ListUserKeys(context.Background(), username)
		if err != nil {
			fmt.Printf("Could not fetch keys for %s: %v\n", username, err)
			continue
		}

		for _, key := range keys {
			allKeys += fmt.Sprintf("root:%s %sroot\nubuntu:%s %subuntu\n", key.GetKey(), username, key.GetKey(), username)
		}
	}

	return allKeys, nil
}

// listAllGitHubTeamMembers retrieves all members of the specified GitHub team, handling pagination to ensure all members are fetched.
func listAllGitHubTeamMembers(client GitHubClient, org string, teamSlug string) ([]*github.User, error) {
	perPage := 100
	page := 1
	var allMembers []*github.User

	for {
		opts := &github.TeamListTeamMembersOptions{
			ListOptions: github.ListOptions{
				Page:    page,
				PerPage: perPage,
			},
		}

		members, err := client.ListTeamMembersBySlug(context.Background(), org, teamSlug, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch team members from GitHub: %w", err)
		}

		if len(members) == 0 {
			break
		}

		allMembers = append(allMembers, members...)

		if len(members) < perPage {
			break
		}

		page++
	}

	return allMembers, nil
}
