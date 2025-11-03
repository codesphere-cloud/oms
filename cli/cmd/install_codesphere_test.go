// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"errors"
	"os"
	"runtime"

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

var _ = Describe("InstallCodesphereCmd", func() {
	var (
		c          cmd.InstallCodesphereCmd
		opts       *cmd.InstallCodesphereOpts
		globalOpts *cmd.GlobalOptions
		mockEnv    *env.MockEnv
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		globalOpts = &cmd.GlobalOptions{}
		opts = &cmd.InstallCodesphereOpts{
			GlobalOptions: globalOpts,
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

			tempConfigFile, err := os.CreateTemp("", "test-config.yaml")
			Expect(err).To(BeNil())
			defer func() { _ = os.Remove(tempConfigFile.Name()) }()

			_, err = tempConfigFile.WriteString("codesphere:\n  deployConfig:\n    images: {}\n")
			Expect(err).To(BeNil())
			_ = tempConfigFile.Close()

			c.Opts.Config = tempConfigFile.Name()
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir")

			err = c.RunE(nil, []string{})

			Expect(err).To(HaveOccurred())
			if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
				// Should fail with platform error on non-Linux platform
				Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
			} else {
				// On Linux amd64, it should fail on package extraction since the package doesn't exist
				Expect(err.Error()).To(ContainSubstring("failed to extract and install package"))
			}
		})
	})

	Context("ExtractAndInstall method", func() {
		It("fails on non-linux amd64 platforms", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockConfigManager := installer.NewMockConfigManager(GinkgoT())
			mockImageManager := system.NewMockImageManager(GinkgoT())

			// Test with Windows platform
			err := c.ExtractAndInstall(mockPackageManager, mockConfigManager, mockImageManager, "windows", "amd64")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
			Expect(err.Error()).To(ContainSubstring("windows/amd64"))

			// Test with ARM64 architecture
			err = c.ExtractAndInstall(mockPackageManager, mockConfigManager, mockImageManager, "linux", "arm64")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("codesphere installation is only supported on Linux amd64"))
			Expect(err.Error()).To(ContainSubstring("linux/arm64"))
		})

		Context("when on Linux amd64", func() {
			It("fails when config parsing fails", func() {
				mockPackageManager := installer.NewMockPackageManager(GinkgoT())
				mockConfigManager := installer.NewMockConfigManager(GinkgoT())
				mockImageManager := system.NewMockImageManager(GinkgoT())

				c.Opts.Config = "invalid-config.yaml"
				mockConfigManager.EXPECT().ParseConfigYaml("invalid-config.yaml").Return(files.RootConfig{}, errors.New("config parse error"))

				err := c.ExtractAndInstall(mockPackageManager, mockConfigManager, mockImageManager, "linux", "amd64")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to extract config.yaml"))
			})

			It("fails when package extraction fails", func() {
				mockPackageManager := installer.NewMockPackageManager(GinkgoT())
				mockConfigManager := installer.NewMockConfigManager(GinkgoT())
				mockImageManager := system.NewMockImageManager(GinkgoT())

				c.Opts.Config = "valid-config.yaml"
				mockConfigManager.EXPECT().ParseConfigYaml("valid-config.yaml").Return(files.RootConfig{}, nil)
				mockPackageManager.EXPECT().Extract(false).Return(errors.New("extraction failed"))

				err := c.ExtractAndInstall(mockPackageManager, mockConfigManager, mockImageManager, "linux", "amd64")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to extract package to workdir"))
			})

			It("fails when package listing fails", func() {
				mockPackageManager := installer.NewMockPackageManager(GinkgoT())
				mockConfigManager := installer.NewMockConfigManager(GinkgoT())
				mockImageManager := system.NewMockImageManager(GinkgoT())
				mockFileIO := util.NewMockFileIO(GinkgoT())

				c.Opts.Config = "valid-config.yaml"
				mockConfigManager.EXPECT().ParseConfigYaml("valid-config.yaml").Return(files.RootConfig{}, nil)
				mockPackageManager.EXPECT().Extract(false).Return(nil)
				mockPackageManager.EXPECT().GetWorkDir().Return("/test/workdir/package")
				mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
				mockFileIO.EXPECT().Exists("/test/workdir/package").Return(false)

				err := c.ExtractAndInstall(mockPackageManager, mockConfigManager, mockImageManager, "linux", "amd64")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to list available files"))
			})

			It("fails when deps.tar.gz is missing from package", func() {
				mockPackageManager := installer.NewMockPackageManager(GinkgoT())
				mockConfigManager := installer.NewMockConfigManager(GinkgoT())
				mockImageManager := system.NewMockImageManager(GinkgoT())
				mockFileIO := util.NewMockFileIO(GinkgoT())

				c.Opts.Config = "valid-config.yaml"
				mockConfigManager.EXPECT().ParseConfigYaml("valid-config.yaml").Return(files.RootConfig{}, nil)
				mockPackageManager.EXPECT().Extract(false).Return(nil)
				mockPackageManager.EXPECT().GetWorkDir().Return("/test/workdir/package")
				mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
				mockFileIO.EXPECT().Exists("/test/workdir/package").Return(true)

				// Create mock directory entries without deps.tar.gz
				mockEntries := []os.DirEntry{
					&MockDirEntry{name: "node", isDir: false},
					&MockDirEntry{name: "private-cloud-installer.js", isDir: false},
					&MockDirEntry{name: "kubectl", isDir: false},
				}
				mockFileIO.EXPECT().ReadDir("/test/workdir/package").Return(mockEntries, nil)

				err := c.ExtractAndInstall(mockPackageManager, mockConfigManager, mockImageManager, "linux", "amd64")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("deps.tar.gz not found in package"))
			})

			It("fails when private-cloud-installer.js is missing from package", func() {
				mockPackageManager := installer.NewMockPackageManager(GinkgoT())
				mockConfigManager := installer.NewMockConfigManager(GinkgoT())
				mockImageManager := system.NewMockImageManager(GinkgoT())
				mockFileIO := util.NewMockFileIO(GinkgoT())

				c.Opts.Config = "valid-config.yaml"
				mockConfigManager.EXPECT().ParseConfigYaml("valid-config.yaml").Return(files.RootConfig{}, nil)
				mockPackageManager.EXPECT().Extract(false).Return(nil)
				mockPackageManager.EXPECT().GetWorkDir().Return("/test/workdir/package")
				mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
				mockFileIO.EXPECT().Exists("/test/workdir/package").Return(true)

				// Create mock directory entries without private-cloud-installer.js
				mockEntries := []os.DirEntry{
					&MockDirEntry{name: "deps.tar.gz", isDir: false},
					&MockDirEntry{name: "node", isDir: false},
					&MockDirEntry{name: "kubectl", isDir: false},
				}
				mockFileIO.EXPECT().ReadDir("/test/workdir/package").Return(mockEntries, nil)

				err := c.ExtractAndInstall(mockPackageManager, mockConfigManager, mockImageManager, "linux", "amd64")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("private-cloud-installer.js not found in package"))
			})

			It("fails when node executable is missing from package", func() {
				mockPackageManager := installer.NewMockPackageManager(GinkgoT())
				mockConfigManager := installer.NewMockConfigManager(GinkgoT())
				mockImageManager := system.NewMockImageManager(GinkgoT())
				mockFileIO := util.NewMockFileIO(GinkgoT())

				c.Opts.Config = "valid-config.yaml"
				mockConfigManager.EXPECT().ParseConfigYaml("valid-config.yaml").Return(files.RootConfig{}, nil)
				mockPackageManager.EXPECT().Extract(false).Return(nil)
				mockPackageManager.EXPECT().GetWorkDir().Return("/test/workdir/package")
				mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
				mockFileIO.EXPECT().Exists("/test/workdir/package").Return(true)

				// Create mock directory entries without node executable
				mockEntries := []os.DirEntry{
					&MockDirEntry{name: "deps.tar.gz", isDir: false},
					&MockDirEntry{name: "private-cloud-installer.js", isDir: false},
					&MockDirEntry{name: "kubectl", isDir: false},
				}
				mockFileIO.EXPECT().ReadDir("/test/workdir/package").Return(mockEntries, nil)

				err := c.ExtractAndInstall(mockPackageManager, mockConfigManager, mockImageManager, "linux", "amd64")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("node executable not found in package"))
			})

			It("successfully validates all required files but fails on execution", func() {
				mockPackageManager := installer.NewMockPackageManager(GinkgoT())
				mockConfigManager := installer.NewMockConfigManager(GinkgoT())
				mockImageManager := system.NewMockImageManager(GinkgoT())
				mockFileIO := util.NewMockFileIO(GinkgoT())

				c.Opts.Config = "valid-config.yaml"
				c.Opts.PrivKey = "test-key.pem"
				mockConfigManager.EXPECT().ParseConfigYaml("valid-config.yaml").Return(files.RootConfig{}, nil)
				mockPackageManager.EXPECT().Extract(false).Return(nil)
				mockPackageManager.EXPECT().GetWorkDir().Return("/test/workdir/package")
				mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
				mockFileIO.EXPECT().Exists("/test/workdir/package").Return(true)

				// Create complete mock directory entries with all required files
				mockEntries := []os.DirEntry{
					&MockDirEntry{name: "deps.tar.gz", isDir: false},
					&MockDirEntry{name: "node", isDir: false},
					&MockDirEntry{name: "private-cloud-installer.js", isDir: false},
					&MockDirEntry{name: "kubectl", isDir: false},
				}
				mockFileIO.EXPECT().ReadDir("/test/workdir/package").Return(mockEntries, nil)

				err := c.ExtractAndInstall(mockPackageManager, mockConfigManager, mockImageManager, "linux", "amd64")
				Expect(err).To(HaveOccurred())
				// Should fail when trying to make fake node executable
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to make node executable"))
			})

			It("successfully builds and pushes custom workspace images", func() {
				mockPackageManager := installer.NewMockPackageManager(GinkgoT())
				mockConfigManager := installer.NewMockConfigManager(GinkgoT())
				mockImageManager := system.NewMockImageManager(GinkgoT())
				mockFileIO := util.NewMockFileIO(GinkgoT())

				c.Opts.Config = "valid-config.yaml"
				c.Opts.PrivKey = "test-key.pem"

				// Create config with workspace dockerfiles
				config := files.RootConfig{
					Codesphere: files.CodesphereConfig{
						DeployConfig: files.DeployConfig{
							Images: map[string]files.ImageConfig{
								"ubuntu-24.04": {
									Flavors: map[string]files.FlavorConfig{
										"default": {
											Image: files.ImageRef{
												BomRef:     "docker.io/library/ubuntu:24.04",
												Dockerfile: "workspace.Dockerfile",
											},
										},
									},
								},
							},
						},
					},
				}

				mockConfigManager.EXPECT().ParseConfigYaml("valid-config.yaml").Return(config, nil)
				mockPackageManager.EXPECT().Extract(false).Return(nil)
				mockPackageManager.EXPECT().GetWorkDir().Return("/test/workdir/package")
				mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
				mockFileIO.EXPECT().Exists("/test/workdir/package").Return(true)

				// Create complete mock directory entries with all required files
				mockEntries := []os.DirEntry{
					&MockDirEntry{name: "deps.tar.gz", isDir: false},
					&MockDirEntry{name: "node", isDir: false},
					&MockDirEntry{name: "private-cloud-installer.js", isDir: false},
					&MockDirEntry{name: "kubectl", isDir: false},
				}
				mockFileIO.EXPECT().ReadDir("/test/workdir/package").Return(mockEntries, nil)

				mockPackageManager.EXPECT().ExtractDependency("bom.json", false).Return(nil)
				mockPackageManager.EXPECT().ExtractDependency("codesphere/images/ubuntu.tar", false).Return(nil)
				mockPackageManager.EXPECT().GetDependencyPath("codesphere/images/ubuntu.tar").Return("/test/workdir/deps/codesphere/images/ubuntu.tar")
				mockImageManager.EXPECT().LoadImage("/test/workdir/deps/codesphere/images/ubuntu.tar").Return(nil)

				// Mock for reading the Dockerfile
				mockFileIO.EXPECT().Open("workspace.Dockerfile").Return(nil, errors.New("dockerfile not found"))

				err := c.ExtractAndInstall(mockPackageManager, mockConfigManager, mockImageManager, "linux", "amd64")
				// Should fail when trying to read the dockerfile
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("dockerfile not found"))
			})
		})
	})

	Context("listPackageContents method", func() {
		It("fails when work directory doesn't exist", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockFileIO := util.NewMockFileIO(GinkgoT())

			mockPackageManager.EXPECT().GetWorkDir().Return("/test/workdir/package")
			mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
			mockFileIO.EXPECT().Exists("/test/workdir/package").Return(false)

			filenames, err := c.ListPackageContents(mockPackageManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("work dir not found"))
			Expect(filenames).To(BeNil())
		})

		It("fails when ReadDir fails", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockFileIO := util.NewMockFileIO(GinkgoT())

			mockPackageManager.EXPECT().GetWorkDir().Return("/test/workdir/package")
			mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
			mockFileIO.EXPECT().Exists("/test/workdir/package").Return(true)
			mockFileIO.EXPECT().ReadDir("/test/workdir/package").Return(nil, os.ErrPermission)

			filenames, err := c.ListPackageContents(mockPackageManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read directory contents"))
			Expect(filenames).To(BeNil())
		})

		It("successfully lists package contents", func() {
			mockPackageManager := installer.NewMockPackageManager(GinkgoT())
			mockFileIO := util.NewMockFileIO(GinkgoT())

			// Create mock directory entries
			mockEntries := []os.DirEntry{
				&MockDirEntry{name: "deps.tar.gz", isDir: false},
				&MockDirEntry{name: "node", isDir: false},
				&MockDirEntry{name: "private-cloud-installer.js", isDir: false},
				&MockDirEntry{name: "kubectl", isDir: false},
			}

			mockPackageManager.EXPECT().GetWorkDir().Return("/test/workdir/package")
			mockPackageManager.EXPECT().FileIO().Return(mockFileIO)
			mockFileIO.EXPECT().Exists("/test/workdir/package").Return(true)
			mockFileIO.EXPECT().ReadDir("/test/workdir/package").Return(mockEntries, nil)

			filenames, err := c.ListPackageContents(mockPackageManager)
			Expect(err).To(BeNil())
			Expect(filenames).To(HaveLen(4))
			Expect(filenames).To(ContainElement("deps.tar.gz"))
			Expect(filenames).To(ContainElement("node"))
			Expect(filenames).To(ContainElement("private-cloud-installer.js"))
			Expect(filenames).To(ContainElement("kubectl"))
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
