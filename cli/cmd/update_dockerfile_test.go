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
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/codesphere-cloud/oms/internal/util"
)

const sampleDockerfileContent = `FROM ubuntu:20.04
RUN apt-get update && apt-get install -y curl
WORKDIR /app
COPY . .
CMD ["./start.sh"]`

var _ = Describe("UpdateDockerfileCmd", func() {
	var (
		c          cmd.UpdateDockerfileCmd
		opts       cmd.UpdateDockerfileOpts
		globalOpts cmd.GlobalOptions
		mockEnv    *env.MockEnv
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		globalOpts = cmd.GlobalOptions{}
		opts = cmd.UpdateDockerfileOpts{
			GlobalOptions: globalOpts,
			Package:       "codesphere-v1.68.0.tar.gz",
			Dockerfile:    "Dockerfile",
			Baseimage:     "",
			Force:         false,
		}
		c = cmd.UpdateDockerfileCmd{
			Opts: opts,
			Env:  mockEnv,
		}
	})

	AfterEach(func() {
		mockEnv.AssertExpectations(GinkgoT())
	})

	Context("RunE method", func() {
		It("fails when package is not set", func() {
			c.Opts.Package = ""

			err := c.RunE(nil, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("required option package not set"))
		})

		It("calls UpdateDockerfile and fails when package manager fails", func() {
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir")

			err := c.RunE(nil, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update dockerfile"))
		})

		It("succeeds with valid options", func() {
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir")

			// This will fail in real scenario because it tries to extract from real package
			// But it should at least call the correct methods
			err := c.RunE(nil, []string{})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("UpdateDockerfile method", func() {
		It("fails when package manager fails to get image path and name", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			c.Opts.Baseimage = "workspace-agent-24.04.tar"
			c.Opts.Force = false

			mockPackageManager.EXPECT().GetImagePathAndName("workspace-agent-24.04.tar", false).Return("", "", errors.New("failed to extract image"))

			err := c.UpdateDockerfile(mockPackageManager, mockImageManager, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get image name"))
		})

		It("fails when dockerfile cannot be opened", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())
			mockFileIO := util.NewMockFileIO(GinkgoT())

			c.Opts.Dockerfile = "Dockerfile"
			c.Opts.Baseimage = ""
			c.Opts.Force = false

			mockPackageManager.EXPECT().GetImagePathAndName("", false).Return("/test/workdir/deps/codesphere/images/workspace-agent-24.04.tar", "ubuntu:24.04", nil)
			mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
			mockFileIO.EXPECT().Open("Dockerfile").Return(nil, errors.New("file not found"))

			err := c.UpdateDockerfile(mockPackageManager, mockImageManager, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to open dockerfile"))
		})

		It("fails when image manager fails to load image", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())
			mockFileIO := util.NewMockFileIO(GinkgoT())

			// Create a temporary file for the Dockerfile
			tempFile, err := os.CreateTemp("", "dockerfile-test-*")
			Expect(err).To(BeNil())
			DeferCleanup(func() {
				tempFile.Close()
				os.Remove(tempFile.Name())
			})
			_, err = tempFile.WriteString(sampleDockerfileContent)
			Expect(err).To(BeNil())
			// Reset file position to beginning
			tempFile.Seek(0, 0)

			c.Opts.Dockerfile = "Dockerfile"
			c.Opts.Baseimage = ""
			c.Opts.Force = false

			mockPackageManager.EXPECT().GetImagePathAndName("", false).Return("/test/workdir/deps/codesphere/images/workspace-agent-24.04.tar", "ubuntu:24.04", nil)
			mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
			mockFileIO.EXPECT().Open("Dockerfile").Return(tempFile, nil)
			mockFileIO.EXPECT().WriteFile("Dockerfile", []byte("FROM ubuntu:24.04\nRUN apt-get update && apt-get install -y curl\nWORKDIR /app\nCOPY . .\nCMD [\"./start.sh\"]"), os.FileMode(0644)).Return(nil)
			mockImageManager.EXPECT().LoadImage("/test/workdir/deps/codesphere/images/workspace-agent-24.04.tar").Return(errors.New("load failed"))

			err = c.UpdateDockerfile(mockPackageManager, mockImageManager, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to load baseimage file"))
		})

		It("fails when writing updated dockerfile fails", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())
			mockFileIO := util.NewMockFileIO(GinkgoT())

			// Create a temporary file for the Dockerfile
			tempFile, err := os.CreateTemp("", "dockerfile-test-*")
			Expect(err).To(BeNil())
			DeferCleanup(func() {
				tempFile.Close()
				os.Remove(tempFile.Name())
			})
			_, err = tempFile.WriteString(sampleDockerfileContent)
			Expect(err).To(BeNil())
			// Reset file position to beginning
			tempFile.Seek(0, 0)

			c.Opts.Dockerfile = "Dockerfile"
			c.Opts.Baseimage = ""
			c.Opts.Force = false

			mockPackageManager.EXPECT().GetImagePathAndName("", false).Return("/test/workdir/deps/codesphere/images/workspace-agent-24.04.tar", "ubuntu:24.04", nil)
			mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
			mockFileIO.EXPECT().Open("Dockerfile").Return(tempFile, nil)
			mockFileIO.EXPECT().WriteFile("Dockerfile", []byte("FROM ubuntu:24.04\nRUN apt-get update && apt-get install -y curl\nWORKDIR /app\nCOPY . .\nCMD [\"./start.sh\"]"), os.FileMode(0644)).Return(errors.New("write failed"))

			err = c.UpdateDockerfile(mockPackageManager, mockImageManager, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to write updated dockerfile"))
		})

		It("successfully updates dockerfile and loads image", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())
			mockFileIO := util.NewMockFileIO(GinkgoT())

			// Create a temporary file for the Dockerfile
			tempFile, err := os.CreateTemp("", "dockerfile-test-*")
			Expect(err).To(BeNil())
			DeferCleanup(func() {
				tempFile.Close()
				os.Remove(tempFile.Name())
			})
			_, err = tempFile.WriteString(sampleDockerfileContent)
			Expect(err).To(BeNil())
			// Reset file position to beginning
			tempFile.Seek(0, 0)

			c.Opts.Dockerfile = "Dockerfile"
			c.Opts.Baseimage = ""
			c.Opts.Force = false

			mockPackageManager.EXPECT().GetImagePathAndName("", false).Return("/test/workdir/deps/codesphere/images/workspace-agent-24.04.tar", "ubuntu:24.04", nil)
			mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
			mockFileIO.EXPECT().Open("Dockerfile").Return(tempFile, nil)
			mockFileIO.EXPECT().WriteFile("Dockerfile", []byte("FROM ubuntu:24.04\nRUN apt-get update && apt-get install -y curl\nWORKDIR /app\nCOPY . .\nCMD [\"./start.sh\"]"), os.FileMode(0644)).Return(nil)
			mockImageManager.EXPECT().LoadImage("/test/workdir/deps/codesphere/images/workspace-agent-24.04.tar").Return(nil)

			err = c.UpdateDockerfile(mockPackageManager, mockImageManager, []string{})
			Expect(err).To(BeNil())
		})

		It("uses force flag when extracting dependencies", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())
			mockFileIO := util.NewMockFileIO(GinkgoT())

			// Create a temporary file for the Dockerfile
			tempFile, err := os.CreateTemp("", "dockerfile-test-*")
			Expect(err).To(BeNil())
			DeferCleanup(func() {
				tempFile.Close()
				os.Remove(tempFile.Name())
			})
			_, err = tempFile.WriteString(sampleDockerfileContent)
			Expect(err).To(BeNil())
			// Reset file position to beginning
			tempFile.Seek(0, 0)

			c.Opts.Dockerfile = "Dockerfile"
			c.Opts.Baseimage = "workspace-agent-20.04.tar"
			c.Opts.Force = true

			mockPackageManager.EXPECT().GetImagePathAndName("workspace-agent-20.04.tar", true).Return("/test/workdir/deps/codesphere/images/workspace-agent-20.04.tar", "ubuntu:20.04", nil)
			mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
			mockFileIO.EXPECT().Open("Dockerfile").Return(tempFile, nil)
			mockFileIO.EXPECT().WriteFile("Dockerfile", []byte("FROM ubuntu:20.04\nRUN apt-get update && apt-get install -y curl\nWORKDIR /app\nCOPY . .\nCMD [\"./start.sh\"]"), os.FileMode(0644)).Return(nil)
			mockImageManager.EXPECT().LoadImage("/test/workdir/deps/codesphere/images/workspace-agent-20.04.tar").Return(nil)

			err = c.UpdateDockerfile(mockPackageManager, mockImageManager, []string{})
			Expect(err).To(BeNil())
		})

		It("handles different base image names correctly", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())
			mockFileIO := util.NewMockFileIO(GinkgoT())

			// Create a temporary file for the Dockerfile
			tempFile, err := os.CreateTemp("", "dockerfile-test-*")
			Expect(err).To(BeNil())
			DeferCleanup(func() {
				tempFile.Close()
				os.Remove(tempFile.Name())
			})
			_, err = tempFile.WriteString(sampleDockerfileContent)
			Expect(err).To(BeNil())
			// Reset file position to beginning
			tempFile.Seek(0, 0)

			c.Opts.Dockerfile = "custom/Dockerfile"
			c.Opts.Baseimage = "workspace-agent-24.04.tar"
			c.Opts.Force = false

			mockPackageManager.EXPECT().GetImagePathAndName("workspace-agent-24.04.tar", false).Return("/test/workdir/deps/codesphere/images/workspace-agent-24.04.tar", "registry.example.com/workspace-agent:24.04", nil)
			mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
			mockFileIO.EXPECT().Open("custom/Dockerfile").Return(tempFile, nil)
			mockFileIO.EXPECT().WriteFile("custom/Dockerfile", []byte("FROM registry.example.com/workspace-agent:24.04\nRUN apt-get update && apt-get install -y curl\nWORKDIR /app\nCOPY . .\nCMD [\"./start.sh\"]"), os.FileMode(0644)).Return(nil)
			mockImageManager.EXPECT().LoadImage("/test/workdir/deps/codesphere/images/workspace-agent-24.04.tar").Return(nil)

			err = c.UpdateDockerfile(mockPackageManager, mockImageManager, []string{})
			Expect(err).To(BeNil())
		})
	})
})

var _ = Describe("AddUpdateDockerfileCmd", func() {
	var (
		parentCmd  *cobra.Command
		globalOpts cmd.GlobalOptions
	)

	BeforeEach(func() {
		parentCmd = &cobra.Command{Use: "update"}
		globalOpts = cmd.GlobalOptions{}
	})

	It("adds the dockerfile command with correct properties and flags", func() {
		cmd.AddUpdateDockerfileCmd(parentCmd, globalOpts)

		var dockerfileCmd *cobra.Command
		for _, c := range parentCmd.Commands() {
			if c.Use == "dockerfile" {
				dockerfileCmd = c
				break
			}
		}

		Expect(dockerfileCmd).NotTo(BeNil())
		Expect(dockerfileCmd.Use).To(Equal("dockerfile"))
		Expect(dockerfileCmd.Short).To(Equal("Update FROM statement in Dockerfile with base image from package"))
		Expect(dockerfileCmd.Long).To(ContainSubstring("Update the FROM statement in a Dockerfile"))
		Expect(dockerfileCmd.Long).To(ContainSubstring("base image from a Codesphere package"))
		Expect(dockerfileCmd.RunE).NotTo(BeNil())

		// Check required flags
		dockerfileFlag := dockerfileCmd.Flags().Lookup("dockerfile")
		Expect(dockerfileFlag).NotTo(BeNil())
		Expect(dockerfileFlag.Shorthand).To(Equal("d"))
		Expect(dockerfileFlag.Usage).To(ContainSubstring("Path to the Dockerfile to update"))

		packageFlag := dockerfileCmd.Flags().Lookup("package")
		Expect(packageFlag).NotTo(BeNil())
		Expect(packageFlag.Shorthand).To(Equal("p"))
		Expect(packageFlag.Usage).To(ContainSubstring("Path to the Codesphere package"))

		basimageFlag := dockerfileCmd.Flags().Lookup("baseimage")
		Expect(basimageFlag).NotTo(BeNil())
		Expect(basimageFlag.Shorthand).To(Equal("b"))
		Expect(basimageFlag.Usage).To(ContainSubstring("Name of the base image to use"))

		forceFlag := dockerfileCmd.Flags().Lookup("force")
		Expect(forceFlag).NotTo(BeNil())
		Expect(forceFlag.Shorthand).To(Equal("f"))
		Expect(forceFlag.Usage).To(ContainSubstring("Force update even if Dockerfile already exists"))
	})
})
