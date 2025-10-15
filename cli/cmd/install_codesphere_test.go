// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
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
		It("calls GetOmsWorkdir and fails on non-linux platform", func() {
			c.Opts.Package = "test-package.tar.gz"
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir")

			err := c.RunE(nil, []string{})

			Expect(err).To(HaveOccurred())
			if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
				// Should fail with platform error on non-Linux platform
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
			} else {
				// On Linux amd64, it should fail on package extraction since the package doesn't exist
				Expect(err.Error()).To(ContainSubstring("failed to extract package to workdir"))
			}
		})
	})

	Context("ExtractAndInstall method", func() {
		It("fails on non-linux amd64 platforms", func() {
			pkg := &installer.Package{
				OmsWorkdir: "/test/workdir",
				Filename:   "test-package.tar.gz",
				FileIO:     &util.FilesystemWriter{},
			}

			// Test with Windows platform
			err := c.ExtractAndInstall(pkg, "windows", "amd64")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
			Expect(err.Error()).To(ContainSubstring("windows/amd64"))

			// Test with ARM64 architecture
			err = c.ExtractAndInstall(pkg, "linux", "arm64")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
			Expect(err.Error()).To(ContainSubstring("linux/arm64"))
		})

		Context("when on Linux amd64", func() {
			It("fails when package extraction fails", func() {
				pkg := &installer.Package{
					OmsWorkdir: "/test/workdir",
					Filename:   "non-existent-package.tar.gz",
					FileIO:     &util.FilesystemWriter{},
				}

				err := c.ExtractAndInstall(pkg, "linux", "amd64")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to extract package to workdir"))
			})

			It("fails when deps.tar.gz is missing from package", func() {
				tempDir, err := os.MkdirTemp("", "oms-test-*")
				Expect(err).To(BeNil())
				defer func() {
					err := os.RemoveAll(tempDir)
					Expect(err).To(BeNil())
				}()

				origWd, err := os.Getwd()
				Expect(err).To(BeNil())
				err = os.Chdir(tempDir)
				Expect(err).To(BeNil())
				defer func() {
					err := os.Chdir(origWd)
					Expect(err).To(BeNil())
				}()

				// Create package without deps.tar.gz
				testPackageFile := "test-package.tar.gz"
				packageFiles := map[string][]byte{
					"node":                       []byte("fake node binary"),
					"private-cloud-installer.js": []byte("console.log('installer');"),
					"kubectl":                    []byte("fake kubectl binary"),
					// deps.tar.gz missing
				}
				err = createTestTarGz(testPackageFile, packageFiles)
				Expect(err).To(BeNil())

				c.Opts.Force = true
				pkg := &installer.Package{
					OmsWorkdir: tempDir,
					Filename:   testPackageFile,
					FileIO:     &util.FilesystemWriter{},
				}

				err = c.ExtractAndInstall(pkg, "linux", "amd64")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("deps.tar.gz not found in package"))
			})

			It("fails when private-cloud-installer.js is missing from package", func() {
				tempDir, err := os.MkdirTemp("", "oms-test-*")
				Expect(err).To(BeNil())
				defer func() {
					err := os.RemoveAll(tempDir)
					Expect(err).To(BeNil())
				}()

				origWd, err := os.Getwd()
				Expect(err).To(BeNil())
				err = os.Chdir(tempDir)
				Expect(err).To(BeNil())
				defer func() {
					err := os.Chdir(origWd)
					Expect(err).To(BeNil())
				}()

				// Create package without private-cloud-installer.js
				testPackageFile := "test-package.tar.gz"
				packageFiles := map[string][]byte{
					"deps.tar.gz": []byte("fake deps archive"),
					"node":        []byte("fake node binary"),
					"kubectl":     []byte("fake kubectl binary"),
					// private-cloud-installer.js missing
				}
				err = createTestTarGz(testPackageFile, packageFiles)
				Expect(err).To(BeNil())

				c.Opts.Force = true
				pkg := &installer.Package{
					OmsWorkdir: tempDir,
					Filename:   testPackageFile,
					FileIO:     &util.FilesystemWriter{},
				}

				err = c.ExtractAndInstall(pkg, "linux", "amd64")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("private-cloud-installer.js not found in package"))
			})

			It("fails when node executable is missing from package", func() {
				tempDir, err := os.MkdirTemp("", "oms-test-*")
				Expect(err).To(BeNil())
				defer func() {
					err := os.RemoveAll(tempDir)
					Expect(err).To(BeNil())
				}()

				origWd, err := os.Getwd()
				Expect(err).To(BeNil())
				err = os.Chdir(tempDir)
				Expect(err).To(BeNil())
				defer func() {
					err := os.Chdir(origWd)
					Expect(err).To(BeNil())
				}()

				// Create package without node executable
				testPackageFile := "test-package.tar.gz"
				packageFiles := map[string][]byte{
					"deps.tar.gz":                []byte("fake deps archive"),
					"private-cloud-installer.js": []byte("console.log('installer');"),
					"kubectl":                    []byte("fake kubectl binary"),
					// node missing
				}
				err = createTestTarGz(testPackageFile, packageFiles)
				Expect(err).To(BeNil())

				c.Opts.Force = true
				pkg := &installer.Package{
					OmsWorkdir: tempDir,
					Filename:   testPackageFile,
					FileIO:     &util.FilesystemWriter{},
				}

				err = c.ExtractAndInstall(pkg, "linux", "amd64")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("node executable not found in package"))
			})

			It("successfully extracts package with all required files but fails on execution", func() {
				tempDir, err := os.MkdirTemp("", "oms-test-*")
				Expect(err).To(BeNil())
				defer func() {
					err := os.RemoveAll(tempDir)
					Expect(err).To(BeNil())
				}()

				origWd, err := os.Getwd()
				Expect(err).To(BeNil())
				err = os.Chdir(tempDir)
				Expect(err).To(BeNil())
				defer func() {
					err := os.Chdir(origWd)
					Expect(err).To(BeNil())
				}()

				// Create complete package with all required files
				testPackageFile := "test-package.tar.gz"
				packageFiles := map[string][]byte{
					"deps.tar.gz":                []byte("fake deps archive"),
					"node":                       []byte("fake node binary that will fail to execute"),
					"private-cloud-installer.js": []byte("console.log('installer');"),
					"kubectl":                    []byte("fake kubectl binary"),
				}
				err = createTestTarGz(testPackageFile, packageFiles)
				Expect(err).To(BeNil())

				c.Opts.Force = true
				pkg := &installer.Package{
					OmsWorkdir: tempDir,
					Filename:   testPackageFile,
					FileIO:     &util.FilesystemWriter{},
				}

				err = c.ExtractAndInstall(pkg, "linux", "amd64")
				Expect(err).To(HaveOccurred())
				// Should fail when trying to chmod or execute the fake node binary
				Expect(err.Error()).To(SatisfyAny(
					ContainSubstring("failed to make node executable"),
					ContainSubstring("failed to run installer script"),
				))
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
		Expect(codesphereCmd.Short).To(Equal("Install a Codesphere instance"))
		Expect(codesphereCmd.Long).To(ContainSubstring("Uses the private-cloud-installer.js script included in the package to perform the installation."))
		Expect(codesphereCmd.RunE).NotTo(BeNil())

		// Check flags
		packageFlag := codesphereCmd.Flags().Lookup("package")
		Expect(packageFlag).NotTo(BeNil())
		Expect(packageFlag.Shorthand).To(Equal("p"))

		forceFlag := codesphereCmd.Flags().Lookup("force")
		Expect(forceFlag).NotTo(BeNil())
		Expect(forceFlag.Shorthand).To(Equal("f"))
		Expect(forceFlag.DefValue).To(Equal("false"))

		configFlag := codesphereCmd.Flags().Lookup("config")
		Expect(configFlag).NotTo(BeNil())
		Expect(configFlag.Shorthand).To(Equal("c"))

		privKeyFlag := codesphereCmd.Flags().Lookup("priv-key")
		Expect(privKeyFlag).NotTo(BeNil())
		Expect(privKeyFlag.Shorthand).To(Equal("k"))

		skipStepFlag := codesphereCmd.Flags().Lookup("skip-steps")
		Expect(skipStepFlag).NotTo(BeNil())
		Expect(skipStepFlag.Shorthand).To(Equal("s"))
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
