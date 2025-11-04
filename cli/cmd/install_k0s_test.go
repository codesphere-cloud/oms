// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"errors"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("InstallK0sCmd", func() {
	var (
		installK0sCmd  *cmd.InstallK0sCmd
		mockEnv        *env.MockEnv
		mockFileWriter *util.MockFileIO
		mockK0sManager *installer.MockK0sManager
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		mockFileWriter = util.NewMockFileIO(GinkgoT())
		mockK0sManager = installer.NewMockK0sManager(GinkgoT())

		installK0sCmd = &cmd.InstallK0sCmd{
			Opts: cmd.InstallK0sOpts{
				GlobalOptions: &cmd.GlobalOptions{},
				Config:        "",
				Force:         false,
			},
			Env:        mockEnv,
			FileWriter: mockFileWriter,
		}
	})

	AfterEach(func() {
		mockEnv.AssertExpectations(GinkgoT())
		mockFileWriter.AssertExpectations(GinkgoT())
		if mockK0sManager != nil {
			mockK0sManager.AssertExpectations(GinkgoT())
		}
	})

	Context("RunE", func() {
		It("should successfully handle k0s install integration", func() {
			// Add mock expectations for the new BinaryExists and download functionality, intentionally causing create to fail
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir").Maybe()
			mockFileWriter.EXPECT().Exists("/test/workdir/k0s").Return(false).Maybe()
			mockFileWriter.EXPECT().Create("/test/workdir/k0s").Return(nil, errors.New("mock create error")).Maybe()

			err := installK0sCmd.RunE(nil, nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download k0s"))
			if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
				// Should fail with platform error on non-Linux amd64 platforms
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
			} else {
				// On Linux amd64, it should fail on file creation or network/version fetch since we don't have real network access
				Expect(err.Error()).To(ContainSubstring("mock create error"))
			}
		})

		It("should download k0s when binary doesn't exist", func() {
			// Add mock expectations for the download functionality, intentionally causing create to fail
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir").Maybe()
			mockFileWriter.EXPECT().Exists("/test/workdir/k0s").Return(false).Maybe()
			mockFileWriter.EXPECT().Create("/test/workdir/k0s").Return(nil, errors.New("mock create error")).Maybe()

			err := installK0sCmd.RunE(nil, nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download k0s"))
			if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
				// Should fail with platform error on non-Linux amd64 platforms
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
			} else {
				// On Linux amd64, it should fail on file creation or network/version fetch since we don't have real network access
				Expect(err.Error()).To(ContainSubstring("mock create error"))
			}
		})

		It("should skip download when binary exists and force is false", func() {
			// Set up the test so that binary exists
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir").Maybe()
			mockFileWriter.EXPECT().Exists("/test/workdir/k0s").Return(true).Maybe()

			err := installK0sCmd.RunE(nil, nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install k0s"))
			if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
				// Should fail with platform error on non-Linux amd64 platforms
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
			} else {
				// On Linux amd64, it should fail on file creation or network/version fetch since we don't have real network access
				Expect(err.Error()).To(ContainSubstring("no such file or directory"))
			}
		})
	})
})

var _ = Describe("AddInstallK0sCmd", func() {
	var (
		parentCmd  *cobra.Command
		globalOpts *cmd.GlobalOptions
	)

	BeforeEach(func() {
		parentCmd = &cobra.Command{Use: "install"}
		globalOpts = &cmd.GlobalOptions{}
	})

	It("adds the k0s command with correct properties and flags", func() {
		cmd.AddInstallK0sCmd(parentCmd, globalOpts)

		var k0sCmd *cobra.Command
		for _, c := range parentCmd.Commands() {
			if c.Use == "k0s" {
				k0sCmd = c
				break
			}
		}

		Expect(k0sCmd).NotTo(BeNil())
		Expect(k0sCmd.Use).To(Equal("k0s"))
		Expect(k0sCmd.Short).To(Equal("Install k0s Kubernetes distribution"))
		Expect(k0sCmd.Long).To(ContainSubstring("Install k0s, a zero friction Kubernetes distribution"))
		Expect(k0sCmd.RunE).NotTo(BeNil())

		Expect(k0sCmd.Parent()).To(Equal(parentCmd))
		Expect(parentCmd.Commands()).To(ContainElement(k0sCmd))

		// Check flags
		configFlag := k0sCmd.Flags().Lookup("config")
		Expect(configFlag).NotTo(BeNil())
		Expect(configFlag.Shorthand).To(Equal("c"))
		Expect(configFlag.DefValue).To(Equal(""))
		Expect(configFlag.Usage).To(Equal("Path to k0s configuration file"))

		forceFlag := k0sCmd.Flags().Lookup("force")
		Expect(forceFlag).NotTo(BeNil())
		Expect(forceFlag.Shorthand).To(Equal("f"))
		Expect(forceFlag.DefValue).To(Equal("false"))
		Expect(forceFlag.Usage).To(Equal("Force new download and installation"))

		// Check examples
		Expect(k0sCmd).NotTo(BeNil())
		Expect(k0sCmd.Example).NotTo(BeEmpty())
		Expect(k0sCmd.Example).To(ContainSubstring("oms-cli install k0s"))
		Expect(k0sCmd.Example).To(ContainSubstring("--config"))
		Expect(k0sCmd.Example).To(ContainSubstring("--force"))
	})
})
