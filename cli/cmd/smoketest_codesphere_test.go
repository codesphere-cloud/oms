// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/codesphere"
)

var _ = Describe("SmoketestCodesphereCmd", func() {
	var (
		mockClient *codesphere.MockClient
		c          cmd.SmoketestCodesphereCmd
		opts       *cmd.SmoketestCodesphereOpts
	)

	BeforeEach(func() {
		mockClient = codesphere.NewMockClient(GinkgoT())
		opts = &cmd.SmoketestCodesphereOpts{
			BaseURL: "https://test.codesphere.com/api",
			Token:   "test-token",
			TeamID:  "123",
			PlanID:  "456",
			Quiet:   true, // Suppress log output in tests
			Timeout: 10 * time.Minute,
			Profile: "ci.yml",
		}
		c = cmd.SmoketestCodesphereCmd{
			Opts:   opts,
			Client: mockClient,
		}
	})

	AfterEach(func() {
		mockClient.AssertExpectations(GinkgoT())
	})

	Context("RunSmoketest", func() {
		It("completes successfully with all steps", func() {
			workspaceID := 789

			// Expect all API calls in order
			mockClient.EXPECT().CreateWorkspace(
				mock.Anything,
				123,                           // teamID
				456,                           // planID
				mock.AnythingOfType("string"), // workspace name is timestamped
				(*string)(nil),                // empty workspace
			).Return(workspaceID, nil).Once()

			mockClient.EXPECT().SetEnvVar(
				mock.Anything,
				workspaceID,
				"TEST_VAR",
				"smoketest",
			).Return(nil).Once()

			// Create ci.yml
			mockClient.EXPECT().ExecuteCommand(
				mock.Anything,
				workspaceID,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> ci.yml")
				}),
			).Return(nil).Once()

			// Create index.html
			mockClient.EXPECT().ExecuteCommand(
				mock.Anything,
				workspaceID,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> index.html")
				}),
			).Return(nil).Once()

			mockClient.EXPECT().SyncLandscape(
				mock.Anything,
				workspaceID,
				"ci.yml",
			).Return(nil).Once()

			mockClient.EXPECT().StartPipeline(
				mock.Anything,
				workspaceID,
				"ci.yml",
				"run",
			).Return(nil).Once()

			mockClient.EXPECT().DeleteWorkspace(
				mock.Anything,
				workspaceID,
			).Return(nil).Once()

			err := c.RunSmoketest()
			Expect(err).To(BeNil())
		})

		It("deletes workspace even on CreateWorkspace failure", func() {
			mockClient.EXPECT().CreateWorkspace(
				mock.Anything,
				123, // teamID
				456, // planID
				mock.AnythingOfType("string"),
				(*string)(nil), // empty workspace
			).Return(0, fmt.Errorf("create failed")).Once()

			err := c.RunSmoketest()
			Expect(err).To(MatchError(ContainSubstring("failed to create workspace")))
		})

		It("deletes workspace on SetEnvVar failure", func() {
			workspaceID := 789

			mockClient.EXPECT().CreateWorkspace(
				mock.Anything,
				123, // teamID
				456, // planID
				mock.AnythingOfType("string"),
				(*string)(nil), // empty workspace
			).Return(workspaceID, nil).Once()

			mockClient.EXPECT().SetEnvVar(
				mock.Anything,
				workspaceID,
				"TEST_VAR",
				"smoketest",
			).Return(fmt.Errorf("setenv failed")).Once()

			mockClient.EXPECT().DeleteWorkspace(
				mock.Anything,
				workspaceID,
			).Return(nil).Once()

			err := c.RunSmoketest()
			Expect(err).To(MatchError(ContainSubstring("failed to set environment variable")))
		})

		It("deletes workspace on ExecuteCommand failure", func() {
			workspaceID := 789

			mockClient.EXPECT().CreateWorkspace(
				mock.Anything,
				123, // teamID
				456, // planID
				mock.AnythingOfType("string"),
				(*string)(nil), // empty workspace
			).Return(workspaceID, nil).Once()

			mockClient.EXPECT().SetEnvVar(
				mock.Anything,
				workspaceID,
				"TEST_VAR",
				"smoketest",
			).Return(nil).Once()

			// Create ci.yml fails
			mockClient.EXPECT().ExecuteCommand(
				mock.Anything,
				workspaceID,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> ci.yml")
				}),
			).Return(fmt.Errorf("exec failed")).Once()

			mockClient.EXPECT().DeleteWorkspace(
				mock.Anything,
				workspaceID,
			).Return(nil).Once()

			err := c.RunSmoketest()
			Expect(err).To(MatchError(ContainSubstring("failed to create ci.yml")))
		})

		It("deletes workspace on SyncLandscape failure", func() {
			workspaceID := 789

			mockClient.EXPECT().CreateWorkspace(
				mock.Anything,
				123, // teamID
				456, // planID
				mock.AnythingOfType("string"),
				(*string)(nil), // empty workspace
			).Return(workspaceID, nil).Once()

			mockClient.EXPECT().SetEnvVar(
				mock.Anything,
				workspaceID,
				"TEST_VAR",
				"smoketest",
			).Return(nil).Once()

			// Create ci.yml
			mockClient.EXPECT().ExecuteCommand(
				mock.Anything,
				workspaceID,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> ci.yml")
				}),
			).Return(nil).Once()

			// Create index.html
			mockClient.EXPECT().ExecuteCommand(
				mock.Anything,
				workspaceID,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> index.html")
				}),
			).Return(nil).Once()

			mockClient.EXPECT().SyncLandscape(
				mock.Anything,
				workspaceID,
				"ci.yml",
			).Return(fmt.Errorf("sync failed")).Once()

			mockClient.EXPECT().DeleteWorkspace(
				mock.Anything,
				workspaceID,
			).Return(nil).Once()

			err := c.RunSmoketest()
			Expect(err).To(MatchError(ContainSubstring("failed to sync landscape")))
		})

		It("deletes workspace on StartPipeline failure", func() {
			workspaceID := 789

			mockClient.EXPECT().CreateWorkspace(
				mock.Anything,
				123, // teamID
				456, // planID
				mock.AnythingOfType("string"),
				(*string)(nil), // empty workspace
			).Return(workspaceID, nil).Once()

			mockClient.EXPECT().SetEnvVar(
				mock.Anything,
				workspaceID,
				"TEST_VAR",
				"smoketest",
			).Return(nil).Once()

			// Create ci.yml
			mockClient.EXPECT().ExecuteCommand(
				mock.Anything,
				workspaceID,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> ci.yml")
				}),
			).Return(nil).Once()

			// Create index.html
			mockClient.EXPECT().ExecuteCommand(
				mock.Anything,
				workspaceID,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> index.html")
				}),
			).Return(nil).Once()

			mockClient.EXPECT().SyncLandscape(
				mock.Anything,
				workspaceID,
				"ci.yml",
			).Return(nil).Once()

			mockClient.EXPECT().StartPipeline(
				mock.Anything,
				workspaceID,
				"ci.yml",
				"run",
			).Return(fmt.Errorf("pipeline failed")).Once()

			mockClient.EXPECT().DeleteWorkspace(
				mock.Anything,
				workspaceID,
			).Return(nil).Once()

			err := c.RunSmoketest()
			Expect(err).To(MatchError(ContainSubstring("failed to start pipeline")))
		})

		It("returns cleanup error when DeleteWorkspace fails", func() {
			workspaceID := 789

			mockClient.EXPECT().CreateWorkspace(
				mock.Anything,
				123, // teamID
				456, // planID
				mock.AnythingOfType("string"),
				(*string)(nil), // empty workspace
			).Return(workspaceID, nil).Once()

			mockClient.EXPECT().SetEnvVar(
				mock.Anything,
				workspaceID,
				"TEST_VAR",
				"smoketest",
			).Return(nil).Once()

			// Create ci.yml
			mockClient.EXPECT().ExecuteCommand(
				mock.Anything,
				workspaceID,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> ci.yml")
				}),
			).Return(nil).Once()

			// Create index.html
			mockClient.EXPECT().ExecuteCommand(
				mock.Anything,
				workspaceID,
				mock.MatchedBy(func(cmd string) bool {
					return strings.Contains(cmd, "> index.html")
				}),
			).Return(nil).Once()

			mockClient.EXPECT().SyncLandscape(
				mock.Anything,
				workspaceID,
				"ci.yml",
			).Return(nil).Once()

			mockClient.EXPECT().StartPipeline(
				mock.Anything,
				workspaceID,
				"ci.yml",
				"run",
			).Return(nil).Once()

			mockClient.EXPECT().DeleteWorkspace(
				mock.Anything,
				workspaceID,
			).Return(fmt.Errorf("delete failed")).Once()

			err := c.RunSmoketest()
			Expect(err).To(MatchError(ContainSubstring("failed to delete workspace")))
		})

		It("runs only specified steps when steps flag is set", func() {
			workspaceID := 789
			opts.Steps = "createWorkspace,setEnvVar"

			mockClient.EXPECT().CreateWorkspace(
				mock.Anything,
				123, // teamID
				456, // planID
				mock.AnythingOfType("string"),
				(*string)(nil),
			).Return(workspaceID, nil).Once()

			mockClient.EXPECT().SetEnvVar(
				mock.Anything,
				workspaceID,
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
