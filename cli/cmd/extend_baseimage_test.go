// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"errors"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("ExtendBaseimageCmd", func() {
	var (
		c          cmd.ExtendBaseimageCmd
		opts       *cmd.ExtendBaseimageOpts
		globalOpts cmd.GlobalOptions
		mockEnv    *env.MockEnv
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		globalOpts = cmd.GlobalOptions{}
		opts = &cmd.ExtendBaseimageOpts{
			GlobalOptions: &globalOpts,
			Dockerfile:    "Dockerfile",
			Force:         false,
		}
		c = cmd.ExtendBaseimageCmd{
			Opts: opts,
			Env:  mockEnv,
		}
	})

	AfterEach(func() {
		mockEnv.AssertExpectations(GinkgoT())
	})

	Context("RunE method", func() {
		It("fails when package is empty", func() {
			c.Opts.Package = ""
			err := c.RunE(nil, []string{})
			Expect(err).To(MatchError("required option package not set"))
		})

		It("calls GetOmsWorkdir and fails on package operations", func() {
			c.Opts.Package = "non-existent-package.tar.gz"
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir")

			err := c.RunE(nil, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to extend baseimage"))
		})
	})

	Context("ExtendBaseimage method", func() {
		It("fails when package manager extraction fails", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			mockPackageManager.EXPECT().ExtractDependency("codesphere/images/workspace-agent-24.04.tar", false).Return(errors.New("extraction failed"))

			err := c.ExtendBaseimage(mockPackageManager, mockConfigManager, mockImageManager, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to extract package to workdir"))
		})

		It("fails when config manager fails to extract OCI image index", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			mockPackageManager.EXPECT().ExtractDependency("codesphere/images/workspace-agent-24.04.tar", false).Return(nil)
			mockPackageManager.EXPECT().GetDependencyPath("codesphere/images/workspace-agent-24.04.tar").Return("/test/workdir/deps/codesphere/images/workspace-agent-24.04.tar")
			mockConfigManager.EXPECT().ExtractOciImageIndex("codesphere/images/workspace-agent-24.04.tar").Return(files.OCIImageIndex{}, errors.New("failed to extract index"))

			err := c.ExtendBaseimage(mockPackageManager, mockConfigManager, mockImageManager, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to extract OCI image index"))
		})

		It("fails when OCI image index has no image names", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			// Create empty OCI index that will return no image names
			ociIndex := files.OCIImageIndex{
				Manifests: []files.ManifestEntry{},
			}
			mockPackageManager.EXPECT().ExtractDependency("codesphere/images/workspace-agent-24.04.tar", false).Return(nil)
			mockPackageManager.EXPECT().GetDependencyPath("codesphere/images/workspace-agent-24.04.tar").Return("/test/workdir/deps/codesphere/images/workspace-agent-24.04.tar")
			mockConfigManager.EXPECT().ExtractOciImageIndex("codesphere/images/workspace-agent-24.04.tar").Return(ociIndex, nil)

			err := c.ExtendBaseimage(mockPackageManager, mockConfigManager, mockImageManager, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read image tags"))
		})

		It("fails when image manager fails to load image", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())
			mockFileIO := util.NewMockFileIO(GinkgoT())

			// Create OCI index with valid image names
			ociIndex := files.OCIImageIndex{
				Manifests: []files.ManifestEntry{
					{
						Annotations: map[string]string{
							"io.containerd.image.name": "ubuntu:24.04-base",
						},
					},
				},
			}
			mockPackageManager.EXPECT().ExtractDependency("codesphere/images/workspace-agent-24.04.tar", false).Return(nil)
			mockPackageManager.EXPECT().GetDependencyPath("codesphere/images/workspace-agent-24.04.tar").Return("/test/workdir/deps/codesphere/images/workspace-agent-24.04.tar")
			mockConfigManager.EXPECT().ExtractOciImageIndex("codesphere/images/workspace-agent-24.04.tar").Return(ociIndex, nil)
			mockPackageManager.EXPECT().FileIO().Return(mockFileIO)

			// Create a temporary file for the Dockerfile generation to work with
			tempFile, err := os.CreateTemp("", "dockerfile-test-*")
			Expect(err).To(BeNil())
			defer os.Remove(tempFile.Name())
			defer tempFile.Close()

			// Mock Dockerfile generation
			mockFileIO.EXPECT().Create("Dockerfile").Return(tempFile, nil)
			mockImageManager.EXPECT().LoadImage("/test/workdir/deps/codesphere/images/workspace-agent-24.04.tar").Return(errors.New("load failed"))

			err = c.ExtendBaseimage(mockPackageManager, mockConfigManager, mockImageManager, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to load baseimage file"))
		})

		It("uses force flag when extracting dependencies", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			c.Opts.Force = true
			mockPackageManager.EXPECT().ExtractDependency("codesphere/images/workspace-agent-24.04.tar", true).Return(errors.New("extraction failed"))

			err := c.ExtendBaseimage(mockPackageManager, mockConfigManager, mockImageManager, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to extract package to workdir"))
		})

		It("successfully completes workflow until dockerfile generation", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())
			mockFileIO := util.NewMockFileIO(GinkgoT())

			// Create OCI index with valid image names
			ociIndex := files.OCIImageIndex{
				Manifests: []files.ManifestEntry{
					{
						Annotations: map[string]string{
							"io.containerd.image.name": "ubuntu:24.04-base",
						},
					},
				},
			}
			mockPackageManager.EXPECT().ExtractDependency("codesphere/images/workspace-agent-24.04.tar", false).Return(nil)
			mockPackageManager.EXPECT().GetDependencyPath("codesphere/images/workspace-agent-24.04.tar").Return("/test/workdir/deps/codesphere/images/workspace-agent-24.04.tar")
			mockConfigManager.EXPECT().ExtractOciImageIndex("codesphere/images/workspace-agent-24.04.tar").Return(ociIndex, nil)
			mockPackageManager.EXPECT().FileIO().Return(mockFileIO)

			// Create a temporary file for the Dockerfile generation to work with
			tempFile, err := os.CreateTemp("", "dockerfile-test-*")
			Expect(err).To(BeNil())
			defer os.Remove(tempFile.Name())
			defer tempFile.Close()

			// Mock Dockerfile generation
			mockFileIO.EXPECT().Create("Dockerfile").Return(tempFile, nil)
			mockImageManager.EXPECT().LoadImage("/test/workdir/deps/codesphere/images/workspace-agent-24.04.tar").Return(nil)

			err = c.ExtendBaseimage(mockPackageManager, mockConfigManager, mockImageManager, []string{})
			Expect(err).To(BeNil())
		})
	})
})

var _ = Describe("AddExtendBaseimageCmd", func() {
	var (
		parentCmd  *cobra.Command
		globalOpts *cmd.GlobalOptions
	)

	BeforeEach(func() {
		parentCmd = &cobra.Command{Use: "extend"}
		globalOpts = &cmd.GlobalOptions{}
	})

	It("adds the baseimage command with correct properties and flags", func() {
		cmd.AddExtendBaseimageCmd(parentCmd, globalOpts)

		var baseimagCmd *cobra.Command
		for _, c := range parentCmd.Commands() {
			if c.Use == "baseimage" {
				baseimagCmd = c
				break
			}
		}

		Expect(baseimagCmd).NotTo(BeNil())
		Expect(baseimagCmd.Use).To(Equal("baseimage"))
		Expect(baseimagCmd.Short).To(Equal("Extend Codesphere's workspace base image for customization"))
		Expect(baseimagCmd.Long).To(ContainSubstring("Loads the baseimage from Codesphere package"))
		Expect(baseimagCmd.RunE).NotTo(BeNil())

		// Check flags
		packageFlag := baseimagCmd.Flags().Lookup("package")
		Expect(packageFlag).NotTo(BeNil())
		Expect(packageFlag.Shorthand).To(Equal("p"))

		dockerfileFlag := baseimagCmd.Flags().Lookup("dockerfile")
		Expect(dockerfileFlag).NotTo(BeNil())
		Expect(dockerfileFlag.Shorthand).To(Equal("d"))
		Expect(dockerfileFlag.DefValue).To(Equal("Dockerfile"))

		forceFlag := baseimagCmd.Flags().Lookup("force")
		Expect(forceFlag).NotTo(BeNil())
		Expect(forceFlag.Shorthand).To(Equal("f"))
		Expect(forceFlag.DefValue).To(Equal("false"))
	})
})
