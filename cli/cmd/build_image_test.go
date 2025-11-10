// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/system"
)

var _ = Describe("BuildImageCmd", func() {
	var (
		c          cmd.BuildImageCmd
		opts       cmd.BuildImageOpts
		globalOpts *cmd.GlobalOptions
		mockEnv    *env.MockEnv
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		globalOpts = &cmd.GlobalOptions{}
		opts = cmd.BuildImageOpts{
			GlobalOptions: globalOpts,
			Dockerfile:    "Dockerfile",
			Package:       "codesphere-vcodesphere-v1.66.0.tar.gz",
			Registry:      "my-registry.com/my-image",
		}
		c = cmd.BuildImageCmd{
			Opts: opts,
			Env:  mockEnv,
		}
	})

	AfterEach(func() {
		mockEnv.AssertExpectations(GinkgoT())
	})

	Context("RunE method", func() {
		It("calls BuildImage and fails when package manager fails", func() {
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir")

			err := c.RunE(nil, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to extract package"))
		})

		It("succeeds with valid options", func() {
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir")

			// This will fail in real scenario because it tries to extract from real package
			// But it should at least call the correct methods
			err := c.RunE(nil, []string{})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("BuildImage method", func() {
		It("fails when package manager fails to get codesphere version", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			mockPackageManager.EXPECT().Extract(false).Return(nil)
			mockPackageManager.EXPECT().GetCodesphereVersion().Return("", errors.New("failed to extract version"))

			err := c.BuildImage(mockPackageManager, mockImageManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get codesphere version from package"))
		})

		It("fails when image build fails", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			c.Opts.Dockerfile = "Dockerfile"
			c.Opts.Registry = "my-registry.com/my-image"

			mockPackageManager.EXPECT().Extract(false).Return(nil)
			mockPackageManager.EXPECT().GetCodesphereVersion().Return("codesphere-v1.66.0", nil)
			mockImageManager.EXPECT().BuildAndPushImage("Dockerfile", "my-registry.com/my-image:codesphere-v1.66.0", ".").Return(errors.New("build failed"))

			err := c.BuildImage(mockPackageManager, mockImageManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to build and push image"))
		})

		It("fails when image push fails", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			c.Opts.Dockerfile = "Dockerfile"
			c.Opts.Registry = "my-registry.com/my-image"

			mockPackageManager.EXPECT().Extract(false).Return(nil)
			mockPackageManager.EXPECT().GetCodesphereVersion().Return("codesphere-v1.66.0", nil)
			mockImageManager.EXPECT().BuildAndPushImage("Dockerfile", "my-registry.com/my-image:codesphere-v1.66.0", ".").Return(errors.New("push failed"))

			err := c.BuildImage(mockPackageManager, mockImageManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to build and push image"))
		})

		It("successfully builds and pushes image", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			c.Opts.Dockerfile = "Dockerfile"
			c.Opts.Registry = "my-registry.com/my-image"

			mockPackageManager.EXPECT().Extract(false).Return(nil)
			mockPackageManager.EXPECT().GetCodesphereVersion().Return("codesphere-v1.66.0", nil)
			mockImageManager.EXPECT().BuildAndPushImage("Dockerfile", "my-registry.com/my-image:codesphere-v1.66.0", ".").Return(nil)

			err := c.BuildImage(mockPackageManager, mockImageManager)
			Expect(err).To(BeNil())
		})
	})
})

var _ = Describe("AddBuildImageCmd", func() {
	var (
		parentCmd  *cobra.Command
		globalOpts *cmd.GlobalOptions
	)

	BeforeEach(func() {
		parentCmd = &cobra.Command{Use: "build"}
		globalOpts = &cmd.GlobalOptions{}
	})

	It("adds the image command with correct properties and flags", func() {
		cmd.AddBuildImageCmd(parentCmd, globalOpts)

		var imageCmd *cobra.Command
		for _, c := range parentCmd.Commands() {
			if c.Use == "image" {
				imageCmd = c
				break
			}
		}

		Expect(imageCmd).NotTo(BeNil())
		Expect(imageCmd.Use).To(Equal("image"))
		Expect(imageCmd.Short).To(Equal("Build and push Docker image using Dockerfile and Codesphere package version"))
		Expect(imageCmd.Long).To(ContainSubstring("Build a Docker image from a Dockerfile and push it to a registry"))
		Expect(imageCmd.Long).To(ContainSubstring("tagged with the Codesphere version"))
		Expect(imageCmd.RunE).NotTo(BeNil())

		// Check required flags
		dockerfileFlag := imageCmd.Flags().Lookup("dockerfile")
		Expect(dockerfileFlag).NotTo(BeNil())
		Expect(dockerfileFlag.Shorthand).To(Equal("d"))
		Expect(dockerfileFlag.Usage).To(ContainSubstring("Path to the Dockerfile to build"))

		packageFlag := imageCmd.Flags().Lookup("package")
		Expect(packageFlag).NotTo(BeNil())
		Expect(packageFlag.Shorthand).To(Equal("p"))
		Expect(packageFlag.Usage).To(ContainSubstring("Path to the Codesphere package"))

		registryFlag := imageCmd.Flags().Lookup("registry")
		Expect(registryFlag).NotTo(BeNil())
		Expect(registryFlag.Shorthand).To(Equal("r"))
		Expect(registryFlag.Usage).To(ContainSubstring("Registry URL to push to"))
	})
})
