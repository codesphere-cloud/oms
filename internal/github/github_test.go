// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package github_test

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/github"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Github", func() {
	Describe("GetSSHKeysFromGitHubTeam", func() {
		var (
			mockGitHubClient *github.MockGitHubClient
			org              string
			teamSlug         string
		)

		Context("when org and team slug are provided", func() {
			BeforeEach(func() {
				mockGitHubClient = github.NewMockGitHubClient(GinkgoT())
				org = "example-org"
				teamSlug = "dev"
			})

			It("fetches GitHub team keys", func() {
				mockGitHubClient.EXPECT().GetTeamMemberSSHKeys(mock.Anything, org, teamSlug).Return([]github.TeamMemberKeys{
					{Login: "alice", Keys: []string{"ssh-rsa AAALICE..."}},
				}, nil).Once()

				keys, err := github.GetSSHKeysFromGitHubTeam(mockGitHubClient, org, teamSlug)
				Expect(err).ToNot(HaveOccurred())
				Expect(keys).To(ContainSubstring("root:ssh-rsa AAALICE... alice"))
				Expect(keys).To(ContainSubstring("ubuntu:ssh-rsa AAALICE... alice"))
			})

			Context("when fetching team member keys fails", func() {
				It("returns an error", func() {
					mockGitHubClient.EXPECT().GetTeamMemberSSHKeys(mock.Anything, org, teamSlug).Return(nil, fmt.Errorf("GitHub API error")).Once()
					keys, err := github.GetSSHKeysFromGitHubTeam(mockGitHubClient, org, teamSlug)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to fetch SSH keys from GitHub team"))
					Expect(keys).To(BeEmpty())
				})
			})

			Context("when a member has no keys", func() {
				It("skips the member and continues", func() {
					mockGitHubClient.EXPECT().GetTeamMemberSSHKeys(mock.Anything, org, teamSlug).Return([]github.TeamMemberKeys{
						{Login: "alice", Keys: nil},
					}, nil).Once()
					keys, err := github.GetSSHKeysFromGitHubTeam(mockGitHubClient, org, teamSlug)
					Expect(err).ToNot(HaveOccurred())
					Expect(keys).To(BeEmpty())
				})
			})

			Context("when team has no members", func() {
				It("returns an empty string", func() {
					mockGitHubClient.EXPECT().GetTeamMemberSSHKeys(mock.Anything, org, teamSlug).Return([]github.TeamMemberKeys{}, nil).Once()
					keys, err := github.GetSSHKeysFromGitHubTeam(mockGitHubClient, org, teamSlug)
					Expect(err).ToNot(HaveOccurred())
					Expect(keys).To(BeEmpty())
				})
			})

			Context("when the team has many members", func() {
				It("formats keys for every member", func() {
					members := make([]github.TeamMemberKeys, 150)
					for i := 0; i < 150; i++ {
						members[i] = github.TeamMemberKeys{
							Login: fmt.Sprintf("user%d", i+1),
							Keys:  []string{fmt.Sprintf("ssh-rsa AAAUSER%d...", i+1)},
						}
					}

					mockGitHubClient.EXPECT().GetTeamMemberSSHKeys(mock.Anything, org, teamSlug).Return(members, nil).Once()

					keys, err := github.GetSSHKeysFromGitHubTeam(mockGitHubClient, org, teamSlug)
					Expect(err).ToNot(HaveOccurred())
					for i := 1; i <= 150; i++ {
						Expect(keys).To(ContainSubstring(fmt.Sprintf("root:ssh-rsa AAAUSER%d... user%d", i, i)))
						Expect(keys).To(ContainSubstring(fmt.Sprintf("ubuntu:ssh-rsa AAAUSER%d... user%d", i, i)))
					}
				})
			})
		})

		Context("when org or team slug is missing", func() {
			It("returns an error if org is missing", func() {
				keys, err := github.GetSSHKeysFromGitHubTeam(mockGitHubClient, "", "dev")
				Expect(err).To(HaveOccurred())
				Expect(keys).To(BeEmpty())
			})

			It("returns an error if team slug is missing", func() {
				keys, err := github.GetSSHKeysFromGitHubTeam(mockGitHubClient, "example-org", "")
				Expect(err).To(HaveOccurred())
				Expect(keys).To(BeEmpty())
			})
		})

	})
})
