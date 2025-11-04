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
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("DownloadK0sCmd", func() {
	var (
		downloadK0sCmd *cmd.DownloadK0sCmd
		mockEnv        *env.MockEnv
		mockFileWriter *util.MockFileIO
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		mockFileWriter = util.NewMockFileIO(GinkgoT())

		downloadK0sCmd = &cmd.DownloadK0sCmd{
			Opts: cmd.DownloadK0sOpts{
				GlobalOptions: &cmd.GlobalOptions{},
				Force:         false,
				Quiet:         false,
			},
			Env:        mockEnv,
			FileWriter: mockFileWriter,
		}
	})

	AfterEach(func() {
		mockEnv.AssertExpectations(GinkgoT())
		mockFileWriter.AssertExpectations(GinkgoT())
	})

	Context("RunE", func() {
		It("should successfully handle k0s download integration", func() {
			// Add mock expectations for the download functionality, intentionally causing create to fail
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir").Maybe()
			mockFileWriter.EXPECT().Exists("/test/workdir/k0s").Return(false).Maybe()
			mockFileWriter.EXPECT().Create("/test/workdir/k0s").Return(nil, errors.New("mock create error")).Maybe()

			err := downloadK0sCmd.RunE(nil, nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download k0s"))
			if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
				// Should fail with platform error on non-Linux amd64 platforms
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
			} else {
				// On Linux amd64, it should fail on network/version fetch since we don't have real network access
				Expect(err.Error()).To(ContainSubstring("mock create error"))
			}
		})
	})
})

var _ = Describe("AddDownloadK0sCmd", func() {
	var (
		parentCmd  *cobra.Command
		globalOpts *cmd.GlobalOptions
	)

	BeforeEach(func() {
		parentCmd = &cobra.Command{Use: "download"}
		globalOpts = &cmd.GlobalOptions{}
	})

	It("adds the k0s command with correct properties and flags", func() {
		cmd.AddDownloadK0sCmd(parentCmd, globalOpts)

		var k0sCmd *cobra.Command
		for _, c := range parentCmd.Commands() {
			if c.Use == "k0s" {
				k0sCmd = c
				break
			}
		}

		Expect(k0sCmd).NotTo(BeNil())
		Expect(k0sCmd.Use).To(Equal("k0s"))
		Expect(k0sCmd.Short).To(Equal("Download k0s Kubernetes distribution"))
		Expect(k0sCmd.Long).To(ContainSubstring("Download k0s, a zero friction Kubernetes distribution"))
		Expect(k0sCmd.Long).To(ContainSubstring("using a Go-native implementation"))
		Expect(k0sCmd.RunE).NotTo(BeNil())

		Expect(k0sCmd.Parent()).To(Equal(parentCmd))
		Expect(parentCmd.Commands()).To(ContainElement(k0sCmd))

		// Check flags
		forceFlag := k0sCmd.Flags().Lookup("force")
		Expect(forceFlag).NotTo(BeNil())
		Expect(forceFlag.Shorthand).To(Equal("f"))
		Expect(forceFlag.DefValue).To(Equal("false"))
		Expect(forceFlag.Usage).To(Equal("Force download even if k0s binary exists"))

		quietFlag := k0sCmd.Flags().Lookup("quiet")
		Expect(quietFlag).NotTo(BeNil())
		Expect(quietFlag.Shorthand).To(Equal("q"))
		Expect(quietFlag.DefValue).To(Equal("false"))
		Expect(quietFlag.Usage).To(Equal("Suppress progress output during download"))

		// Check examples
		Expect(k0sCmd.Example).NotTo(BeEmpty())
		Expect(k0sCmd.Example).To(ContainSubstring("oms-cli download k0s"))
		Expect(k0sCmd.Example).To(ContainSubstring("--quiet"))
		Expect(k0sCmd.Example).To(ContainSubstring("--force"))
	})
})
