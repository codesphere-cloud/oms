// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/codesphere-cloud/cs-go/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/codesphere/teststeps"
)

func mockFullTestRun(mockClient *codesphere.MockClient, teamId, planId, workspaceId int) {
	// Expect the rest of the steps to run with the fetched plan ID
	mockClient.EXPECT().CreateWorkspace(
		teamId,                        // teamID
		planId,                        // fetched planID
		mock.AnythingOfType("string"), // workspace name is timestamped
		(*string)(nil),                // empty workspace
	).Return(workspaceId, nil).Once()

	mockClient.EXPECT().SetEnvVar(
		workspaceId,
		"TEST_VAR",
		"smoketest",
	).Return(nil).Once()

	mockClient.EXPECT().ExecuteCommand(
		workspaceId,
		mock.MatchedBy(func(cmd string) bool {
			return strings.Contains(cmd, "> ci.yml")
		}),
	).Return(nil).Once()

	mockClient.EXPECT().ExecuteCommand(
		workspaceId,
		mock.MatchedBy(func(cmd string) bool {
			return strings.Contains(cmd, "> index.html")
		}),
	).Return(nil).Once()

	mockClient.EXPECT().SyncLandscape(
		workspaceId,
		"ci.yml",
	).Return(nil).Once()

	mockClient.EXPECT().StartPipeline(
		workspaceId,
		"ci.yml",
		"run",
	).Return(nil).Once()

	mockClient.EXPECT().DeleteWorkspace(
		workspaceId,
	).Return(nil).Once()
}

var _ = Describe("SmoketestCodesphereCmd", func() {
	var (
		mockClient  *codesphere.MockClient
		c           cmd.SmoketestCodesphereCmd
		opts        *teststeps.SmoketestCodesphereOpts
		planId      string
		teamId      string
		planIdInt   int
		teamIdInt   int
		workspaceId int
	)

	BeforeEach(func() {
		planId = "456"
		teamId = "123"
		workspaceId = 789
	})

	JustBeforeEach(func() {
		planIdInt, _ = strconv.Atoi(planId)
		teamIdInt, _ = strconv.Atoi(teamId)
		mockClient = codesphere.NewMockClient(GinkgoT())
		opts = &teststeps.SmoketestCodesphereOpts{
			BaseURL: "https://test.codesphere.com/api",
			Token:   "test-token",
			TeamID:  teamId,
			PlanID:  planId,
			Quiet:   true, // Suppress log output in tests
			Timeout: 10 * time.Minute,
			Profile: "ci.yml",
			Steps:   []string{},
			Client:  mockClient,
		}
		c = cmd.SmoketestCodesphereCmd{
			Opts: opts,
		}
	})

	AfterEach(func() {
		mockClient.AssertExpectations(GinkgoT())
	})

	Context("RunSmoketest", func() {
		Context("when no teamId is provided", func() {
			BeforeEach(func() {
				teamId = ""
			})
			Context("when no teams are returned by the API", func() {
				It("returns an error indicating no teams are available", func() {
					mockClient.EXPECT().ListTeams().Return([]api.Team{}, nil).Once()

					err := c.RunSmoketest()
					Expect(err).To(MatchError(ContainSubstring("no teams available")))
				})
			})
			Context("when no primary team is returned by the API", func() {
				It("uses the first team", func() {
					falseVal := false
					mockClient.EXPECT().ListTeams().Return([]api.Team{
						{Id: 99, Name: "other team", IsFirst: &falseVal},
						{Id: 21, Name: "primary team", IsFirst: &falseVal},
					}, nil).Once()

					mockFullTestRun(mockClient, 99, 456, 789)

					err := c.RunSmoketest()
					Expect(err).To(BeNil())
				})
			})
			It("uses the primary team of the user", func() {
				falseVal := false
				trueVal := true
				mockClient.EXPECT().ListTeams().Return([]api.Team{
					{Id: 99, Name: "other team", IsFirst: &falseVal},
					{Id: 21, Name: "primary team", IsFirst: &trueVal},
				}, nil).Once()

				mockFullTestRun(mockClient, 21, 456, 789)

				err := c.RunSmoketest()
				Expect(err).To(BeNil())
			})
		})
		Context("when no planId is provided", func() {
			BeforeEach(func() {
				planId = ""
			})
			Context("when no plans are returned by the API", func() {
				It("returns an error indicating no workspace plans are available", func() {
					mockClient.EXPECT().ListWorkspacePlans().Return([]api.WorkspacePlan{}, nil).Once()

					err := c.RunSmoketest()
					Expect(err).To(MatchError(ContainSubstring("no workspace plans available")))
				})
			})
			It("fetches the first available plan ID", func() {
				mockClient.EXPECT().ListWorkspacePlans().Return([]api.WorkspacePlan{
					{Id: 42, Title: "small"},
					{Id: 1000, Title: "big"},
				}, nil).Once()

				mockFullTestRun(mockClient, teamIdInt, 42, 789)

				err := c.RunSmoketest()
				Expect(err).To(BeNil())
			})
		})
		It("completes successfully with all steps", func() {
			mockFullTestRun(mockClient, teamIdInt, planIdInt, 789)
			err := c.RunSmoketest()
			Expect(err).To(BeNil())
		})

		It("deletes workspace even on CreateWorkspace failure", func() {
			mockClient.EXPECT().CreateWorkspace(
				teamIdInt, // teamID
				planIdInt, // planID
				mock.AnythingOfType("string"),
				(*string)(nil), // empty workspace
			).Return(0, fmt.Errorf("create failed")).Once()

			err := c.RunSmoketest()
			Expect(err).To(MatchError(ContainSubstring("failed to create workspace")))
		})

		It("deletes workspace on SetEnvVar failure", func() {
			workspaceID := 789

			mockClient.EXPECT().CreateWorkspace(
				teamIdInt, // teamID
				planIdInt, // planID
				mock.AnythingOfType("string"),
				(*string)(nil), // empty workspace
			).Return(workspaceID, nil).Once()

			mockClient.EXPECT().SetEnvVar(
				workspaceID,
				"TEST_VAR",
				"smoketest",
			).Return(fmt.Errorf("setenv failed")).Once()

			mockClient.EXPECT().DeleteWorkspace(
				workspaceID,
			).Return(nil).Once()

			err := c.RunSmoketest()
			Expect(err).To(MatchError(ContainSubstring("failed to set environment variable")))
		})

		It("deletes workspace on ExecuteCommand failure", func() {
			mockClient.EXPECT().CreateWorkspace(
				teamIdInt, // teamID
				planIdInt, // planID
				mock.AnythingOfType("string"),
				(*string)(nil), // empty workspace
			).Return(workspaceId, nil).Once()

			mockClient.EXPECT().SetEnvVar(
				workspaceId,
				"TEST_VAR",
				"smoketest",
			).Return(nil).Once()

			// Create ci.yml fails
			mockClient.EXPECT().ExecuteCommand(
				workspaceId,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> ci.yml")
				}),
			).Return(fmt.Errorf("exec failed")).Once()

			mockClient.EXPECT().DeleteWorkspace(
				workspaceId,
			).Return(nil).Once()

			err := c.RunSmoketest()
			Expect(err).To(MatchError(ContainSubstring("failed to create ci.yml")))
		})

		It("deletes workspace on SyncLandscape failure", func() {
			mockClient.EXPECT().CreateWorkspace(
				teamIdInt, // teamID
				planIdInt, // planID
				mock.AnythingOfType("string"),
				(*string)(nil), // empty workspace
			).Return(workspaceId, nil).Once()

			mockClient.EXPECT().SetEnvVar(
				workspaceId,
				"TEST_VAR",
				"smoketest",
			).Return(nil).Once()

			// Create ci.yml
			mockClient.EXPECT().ExecuteCommand(
				workspaceId,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> ci.yml")
				}),
			).Return(nil).Once()

			// Create index.html
			mockClient.EXPECT().ExecuteCommand(
				workspaceId,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> index.html")
				}),
			).Return(nil).Once()

			mockClient.EXPECT().SyncLandscape(
				workspaceId,
				"ci.yml",
			).Return(fmt.Errorf("sync failed")).Once()

			mockClient.EXPECT().DeleteWorkspace(
				workspaceId,
			).Return(nil).Once()

			err := c.RunSmoketest()
			Expect(err).To(MatchError(ContainSubstring("failed to sync landscape")))
		})

		It("deletes workspace on StartPipeline failure", func() {
			mockClient.EXPECT().CreateWorkspace(
				teamIdInt, // teamID
				planIdInt, // planID
				mock.AnythingOfType("string"),
				(*string)(nil), // empty workspace
			).Return(workspaceId, nil).Once()

			mockClient.EXPECT().SetEnvVar(
				workspaceId,
				"TEST_VAR",
				"smoketest",
			).Return(nil).Once()

			// Create ci.yml
			mockClient.EXPECT().ExecuteCommand(
				workspaceId,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> ci.yml")
				}),
			).Return(nil).Once()

			// Create index.html
			mockClient.EXPECT().ExecuteCommand(
				workspaceId,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> index.html")
				}),
			).Return(nil).Once()

			mockClient.EXPECT().SyncLandscape(
				workspaceId,
				"ci.yml",
			).Return(nil).Once()

			mockClient.EXPECT().StartPipeline(
				workspaceId,
				"ci.yml",
				"run",
			).Return(fmt.Errorf("pipeline failed")).Once()

			mockClient.EXPECT().DeleteWorkspace(
				workspaceId,
			).Return(nil).Once()

			err := c.RunSmoketest()
			Expect(err).To(MatchError(ContainSubstring("failed to start pipeline")))
		})

		It("returns cleanup error when DeleteWorkspace fails", func() {
			mockClient.EXPECT().CreateWorkspace(
				teamIdInt, // teamID
				planIdInt, // planID
				mock.AnythingOfType("string"),
				(*string)(nil), // empty workspace
			).Return(workspaceId, nil).Once()

			mockClient.EXPECT().SetEnvVar(
				workspaceId,
				"TEST_VAR",
				"smoketest",
			).Return(nil).Once()

			// Create ci.yml
			mockClient.EXPECT().ExecuteCommand(
				workspaceId,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> ci.yml")
				}),
			).Return(nil).Once()

			// Create index.html
			mockClient.EXPECT().ExecuteCommand(
				workspaceId,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> index.html")
				}),
			).Return(nil).Once()

			mockClient.EXPECT().SyncLandscape(
				workspaceId,
				"ci.yml",
			).Return(nil).Once()

			mockClient.EXPECT().StartPipeline(
				workspaceId,
				"ci.yml",
				"run",
			).Return(nil).Once()

			mockClient.EXPECT().DeleteWorkspace(
				workspaceId,
			).Return(fmt.Errorf("delete failed")).Once()

			err := c.RunSmoketest()
			Expect(err).To(MatchError(ContainSubstring("failed to delete workspace")))
		})

		It("runs only specified steps when steps flag is set", func() {
			opts.Steps = []string{"createWorkspace", "setEnvVar"}

			mockClient.EXPECT().CreateWorkspace(
				teamIdInt, // teamID
				planIdInt, // planID
				mock.AnythingOfType("string"),
				(*string)(nil),
			).Return(workspaceId, nil).Once()

			mockClient.EXPECT().SetEnvVar(
				workspaceId,
				"TEST_VAR",
				"smoketest",
			).Return(nil).Once()

			err := c.RunSmoketest()
			Expect(err).To(BeNil())
		})
	})
})

var _ = Describe("AddSmoketestCodesphereCmd", func() {
	It("adds the smoketest codesphere command to the parent", func() {
		parent := &cobra.Command{}
		opts := &cmd.GlobalOptions{}
		cmd.AddSmoketestCodesphereCmd(parent, opts)
		found := false
		for _, c := range parent.Commands() {
			if c.Use == "codesphere" {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue())
	})
})
