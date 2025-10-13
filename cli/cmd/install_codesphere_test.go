// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"fmt"
	"os"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("InstallCodesphereCmd", func() {
	var (
		c          cmd.InstallCodesphereCmd
		opts       *cmd.InstallCodesphereOpts
		globalOpts cmd.GlobalOptions
		mockEnv    *env.MockEnv
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		globalOpts = cmd.GlobalOptions{}
		opts = &cmd.InstallCodesphereOpts{
			GlobalOptions: &globalOpts,
			Package:       "codesphere-v1.66.0-installer.tar.gz",
			Force:         false,
		}
		c = cmd.InstallCodesphereCmd{
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

		It("calls GetOmsWorkdir and fails on non-linux platform", func() {
			c.Opts.Package = "test-package.tar.gz"
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir")

			err := c.RunE(nil, []string{})

			// On non-Linux platforms, should fail with platform error
			if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
			}
		})
	})

	Context("ExtractAndInstall method", func() {
		It("fails on non-Linux amd64 platforms", func() {
			pkg := &installer.Package{
				OmsWorkdir: "/test/workdir",
				Filename:   "test-package.tar.gz",
				FileIO:     &util.FilesystemWriter{},
			}

			err := c.ExtractAndInstall(pkg, []string{})

			// Should always fail on non-Linux amd64 platforms
			if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("Current platform: %s/%s", runtime.GOOS, runtime.GOARCH)))
			}
		})

		Context("when on Linux amd64 (mocked)", func() {
			BeforeEach(func() {
				// Skip these tests if not on Linux amd64 since we can't easily mock runtime.GOOS
				if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
					Skip("Skipping Linux-specific tests on non-Linux platform")
				}
			})

			It("fails when package extraction fails", func() {
				pkg := &installer.Package{
					OmsWorkdir: "/test/workdir",
					Filename:   "non-existent-package.tar.gz",
					FileIO:     &util.FilesystemWriter{},
				}

				err := c.ExtractAndInstall(pkg, []string{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to extract package to workdir"))
			})
		})
	})

	Context("listPackageContents method", func() {
		It("fails when work directory doesn't exist", func() {
			mockFileIO := util.NewMockFileIO(GinkgoT())
			pkg := &installer.Package{
				OmsWorkdir: "/test/workdir",
				Filename:   "test-package.tar.gz",
				FileIO:     mockFileIO,
			}

			mockFileIO.EXPECT().Exists("/test/workdir/test-package").Return(false)

			filenames, err := c.ListPackageContents(pkg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("work dir not found"))
			Expect(filenames).To(BeNil())
			mockFileIO.AssertExpectations(GinkgoT())
		})

		It("fails when ReadDir fails", func() {
			mockFileIO := util.NewMockFileIO(GinkgoT())
			pkg := &installer.Package{
				OmsWorkdir: "/test/workdir",
				Filename:   "test-package.tar.gz",
				FileIO:     mockFileIO,
			}

			mockFileIO.EXPECT().Exists("/test/workdir/test-package").Return(true)
			mockFileIO.EXPECT().ReadDir("/test/workdir/test-package").Return(nil, os.ErrPermission)

			filenames, err := c.ListPackageContents(pkg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read directory contents"))
			Expect(filenames).To(BeNil())
			mockFileIO.AssertExpectations(GinkgoT())
		})

		It("successfully lists package contents", func() {
			mockFileIO := util.NewMockFileIO(GinkgoT())
			pkg := &installer.Package{
				OmsWorkdir: "/test/workdir",
				Filename:   "test-package.tar.gz",
				FileIO:     mockFileIO,
			}

			// Create mock directory entries
			mockEntries := []os.DirEntry{
				&MockDirEntry{name: "deps.tar.gz", isDir: false},
				&MockDirEntry{name: "node", isDir: false},
				&MockDirEntry{name: "private-cloud-installer.js", isDir: false},
				&MockDirEntry{name: "kubectl", isDir: false},
			}

			mockFileIO.EXPECT().Exists("/test/workdir/test-package").Return(true)
			mockFileIO.EXPECT().ReadDir("/test/workdir/test-package").Return(mockEntries, nil)

			filenames, err := c.ListPackageContents(pkg)
			Expect(err).To(BeNil())
			Expect(filenames).To(HaveLen(4))
			Expect(filenames).To(ContainElement("deps.tar.gz"))
			Expect(filenames).To(ContainElement("node"))
			Expect(filenames).To(ContainElement("private-cloud-installer.js"))
			Expect(filenames).To(ContainElement("kubectl"))
			mockFileIO.AssertExpectations(GinkgoT())
		})
	})
})

var _ = Describe("AddInstallCodesphereCmd", func() {
	var (
		parentCmd  *cobra.Command
		globalOpts *cmd.GlobalOptions
	)

	BeforeEach(func() {
		parentCmd = &cobra.Command{Use: "install"}
		globalOpts = &cmd.GlobalOptions{}
	})

	It("adds the codesphere command with correct properties and flags", func() {
		cmd.AddInstallCodesphereCmd(parentCmd, globalOpts)

		var codesphereCmd *cobra.Command
		for _, c := range parentCmd.Commands() {
			if c.Use == "codesphere" {
				codesphereCmd = c
				break
			}
		}

		Expect(codesphereCmd).NotTo(BeNil())
		Expect(codesphereCmd.Use).To(Equal("codesphere"))
		Expect(codesphereCmd.Short).To(Equal("Coming soon: Install a Codesphere instance"))
		Expect(codesphereCmd.Long).To(ContainSubstring("Coming soon: Install a Codesphere instance"))
		Expect(codesphereCmd.RunE).NotTo(BeNil())

		// Check flags
		packageFlag := codesphereCmd.Flags().Lookup("package")
		Expect(packageFlag).NotTo(BeNil())
		Expect(packageFlag.Shorthand).To(Equal("p"))

		forceFlag := codesphereCmd.Flags().Lookup("force")
		Expect(forceFlag).NotTo(BeNil())
		Expect(forceFlag.Shorthand).To(Equal("f"))
		Expect(forceFlag.DefValue).To(Equal("false"))
	})
})

// MockDirEntry implements os.DirEntry for testing
type MockDirEntry struct {
	name  string
	isDir bool
}

func (m *MockDirEntry) Name() string               { return m.name }
func (m *MockDirEntry) IsDir() bool                { return m.isDir }
func (m *MockDirEntry) Type() os.FileMode          { return 0 }
func (m *MockDirEntry) Info() (os.FileInfo, error) { return nil, nil }
