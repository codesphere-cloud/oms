// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
)

// Helper function to create a test tar.gz file with binary data
func createTestTarGz(filename string, files map[string][]byte) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Size: int64(len(content)),
			Mode: 0644,
		}
		err := tarWriter.WriteHeader(header)
		if err != nil {
			return fmt.Errorf("failed to write header for file %q to tar: %w", name, err)
		}

		_, err = tarWriter.Write(content)
		if err != nil {
			return fmt.Errorf("failed to write file %q to tar: %w", name, err)
		}
	}

	return nil
}

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
		It("fails when package extraction fails due to missing package file", func() {
			pkg := &installer.Package{
				OmsWorkdir: "/test/workdir",
				Filename:   "non-existent-package.tar.gz",
				FileIO:     &util.FilesystemWriter{},
			}

			err := c.ExtendBaseimage(pkg, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to extract package to workdir"))
		})

		It("successfully extracts mocked package file", func() {
			tempDir, err := os.MkdirTemp("", "oms-test-*")
			Expect(err).To(BeNil())
			defer os.RemoveAll(tempDir)

			origWd, err := os.Getwd()
			Expect(err).To(BeNil())
			err = os.Chdir(tempDir)
			Expect(err).To(BeNil())
			defer os.Chdir(origWd)

			depsFile := "deps.tar.gz"
			depsFiles := map[string][]byte{
				"codesphere/images/workspace-agent-24.04.tar": []byte("fake container image content"),
			}
			err = createTestTarGz(depsFile, depsFiles)
			Expect(err).To(BeNil())
			depsContent, err := os.ReadFile(depsFile)
			Expect(err).To(BeNil())

			testPackageFile := "test-package.tar.gz"
			packageFiles := map[string][]byte{
				"deps.tar.gz": depsContent,
			}
			err = createTestTarGz(testPackageFile, packageFiles)
			Expect(err).To(BeNil())

			c.Opts.Force = true

			pkg := &installer.Package{
				OmsWorkdir: tempDir,
				Filename:   testPackageFile,
				FileIO:     &util.FilesystemWriter{},
			}
			err = c.ExtendBaseimage(pkg, []string{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read image tags"))
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
