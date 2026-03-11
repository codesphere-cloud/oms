package github_test

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/github"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	gh "github.com/google/go-github/v74/github"
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
				mockGitHubClient = github.NewMockGitHubClient(ginkgo.GinkgoT())
				org = "example-org"
				teamSlug = "dev"
			})

			It("fetches GitHub team keys", func() {
				mockGitHubClient.EXPECT().ListTeamMembersBySlug(mock.Anything, org, teamSlug, mock.Anything).Return([]*gh.User{{Login: gh.Ptr("alice")}}, nil).Once()
				mockGitHubClient.EXPECT().ListUserKeys(mock.Anything, "alice").Return([]*gh.Key{{Key: gh.Ptr("ssh-rsa AAALICE...")}}, nil).Once()

				keys, err := github.GetSSHKeysFromGitHubTeam(mockGitHubClient, org, teamSlug)
				Expect(err).ToNot(HaveOccurred())
				Expect(keys).To(ContainSubstring("root:ssh-rsa AAALICE... alice"))
				Expect(keys).To(ContainSubstring("ubuntu:ssh-rsa AAALICE... alice"))
			})

			Context("when fetching team members fails", func() {
				It("returns an error", func() {
					mockGitHubClient.EXPECT().ListTeamMembersBySlug(mock.Anything, org, teamSlug, mock.Anything).Return(nil, fmt.Errorf("GitHub API error")).Once()
					keys, err := github.GetSSHKeysFromGitHubTeam(mockGitHubClient, org, teamSlug)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to list GitHub team members"))
					Expect(keys).To(BeEmpty())
				})
			})

			Context("when fetching user keys fails", func() {
				It("skips the user and continues", func() {
					mockGitHubClient.EXPECT().ListTeamMembersBySlug(mock.Anything, org, teamSlug, mock.Anything).Return([]*gh.User{{Login: gh.Ptr("alice")}}, nil).Once()
					mockGitHubClient.EXPECT().ListUserKeys(mock.Anything, "alice").Return(nil, fmt.Errorf("GitHub API error")).Once()
					keys, err := github.GetSSHKeysFromGitHubTeam(mockGitHubClient, org, teamSlug)
					Expect(err).ToNot(HaveOccurred())
					Expect(keys).To(BeEmpty())
				})
			})

			Context("when team has no members", func() {
				It("returns an empty string", func() {
					mockGitHubClient.EXPECT().ListTeamMembersBySlug(mock.Anything, org, teamSlug, mock.Anything).Return([]*gh.User{}, nil).Once()
					keys, err := github.GetSSHKeysFromGitHubTeam(mockGitHubClient, org, teamSlug)
					Expect(err).ToNot(HaveOccurred())
					Expect(keys).To(BeEmpty())
				})
			})

			Context("when team has more than 100 members", func() {
				It("handles pagination correctly", func() {
					// Simulate 150 members to trigger pagination
					membersPage1 := make([]*gh.User, 100)
					for i := 0; i < 100; i++ {
						membersPage1[i] = &gh.User{Login: gh.Ptr(fmt.Sprintf("user%d", i+1))}
					}
					membersPage2 := make([]*gh.User, 50)
					for i := 0; i < 50; i++ {
						membersPage2[i] = &gh.User{Login: gh.Ptr(fmt.Sprintf("user%d", i+101))}
					}

					mockGitHubClient.EXPECT().ListTeamMembersBySlug(mock.Anything, org, teamSlug, mock.Anything).Return(membersPage1, nil).Once()
					mockGitHubClient.EXPECT().ListTeamMembersBySlug(mock.Anything, org, teamSlug, mock.Anything).Return(membersPage2, nil).Once()

					for i := 1; i <= 150; i++ {
						mockGitHubClient.EXPECT().ListUserKeys(mock.Anything, fmt.Sprintf("user%d", i)).Return([]*gh.Key{{Key: gh.Ptr(fmt.Sprintf("ssh-rsa AAAUSER%d...", i))}}, nil).Once()
					}

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
