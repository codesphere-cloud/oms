// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"gopkg.in/yaml.v3"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("InstallK0sCmd", func() {
	var (
		c              cmd.InstallK0sCmd
		opts           *cmd.InstallK0sOpts
		globalOpts     *cmd.GlobalOptions
		mockEnv        *env.MockEnv
		mockFileWriter *util.MockFileIO
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		mockFileWriter = util.NewMockFileIO(GinkgoT())
		globalOpts = &cmd.GlobalOptions{}
		opts = &cmd.InstallK0sOpts{
			GlobalOptions: globalOpts,
			Version:       "",
			Package:       "",
			InstallConfig: "",
			Force:         false,
		}
		c = cmd.InstallK0sCmd{
			Opts:       *opts,
			Env:        mockEnv,
			FileWriter: mockFileWriter,
		}
	})

	AfterEach(func() {
		mockEnv.AssertExpectations(GinkgoT())
		mockFileWriter.AssertExpectations(GinkgoT())
	})

	Context("RunE method", func() {
		It("fails when install-config is not provided", func() {
			c.Opts.InstallConfig = ""
			mockEnv.EXPECT().GetOmsWorkdir().Return("/test/workdir")

			err := c.RunE(nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("--install-config is required"))
		})
	})

	Context("InstallK0sFromInstallConfig method", func() {
		var (
			mockPM  *installer.MockPackageManager
			mockK0s *installer.MockK0sManager
			tempDir string
		)

		BeforeEach(func() {
			mockPM = installer.NewMockPackageManager(GinkgoT())
			mockK0s = installer.NewMockK0sManager(GinkgoT())
			var err error
			tempDir, err = os.MkdirTemp("", "install-k0s-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			mockPM.AssertExpectations(GinkgoT())
			mockK0s.AssertExpectations(GinkgoT())
			if tempDir != "" {
				_ = os.RemoveAll(tempDir)
			}
		})

		createTestConfig := func(managedByCodesphere bool) *files.RootConfig {
			return &files.RootConfig{
				Datacenter: files.DatacenterConfig{
					ID:          1,
					Name:        "test-dc",
					City:        "Test City",
					CountryCode: "US",
				},
				Kubernetes: files.KubernetesConfig{
					ManagedByCodesphere: managedByCodesphere,
					ControlPlanes: []files.K8sNode{
						{IPAddress: "192.168.1.100"},
					},
					APIServerHost: "api.test.example.com",
				},
				Codesphere: files.CodesphereConfig{
					Domain:   "test.example.com",
					PublicIP: "192.168.1.100",
					DeployConfig: files.DeployConfig{
						Images: map[string]files.ImageConfig{},
					},
					Plans: files.PlansConfig{
						HostingPlans:   map[int]files.HostingPlan{},
						WorkspacePlans: map[int]files.WorkspacePlan{},
					},
				},
			}
		}

		It("fails when install-config file does not exist", func() {
			c.Opts.InstallConfig = "/nonexistent/install-config.yaml"

			err := c.InstallK0sFromInstallConfig(mockPM, mockK0s)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to load install-config"))
		})

		It("fails when install-config specifies external Kubernetes", func() {
			config := createTestConfig(false)
			configPath := filepath.Join(tempDir, "install-config.yaml")
			configData, err := yaml.Marshal(config)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(configPath, configData, 0644)
			Expect(err).NotTo(HaveOccurred())

			c.Opts.InstallConfig = configPath

			err = c.InstallK0sFromInstallConfig(mockPM, mockK0s)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("external Kubernetes"))
		})

		It("successfully installs k0s locally with valid config", func() {
			config := createTestConfig(true)
			configPath := filepath.Join(tempDir, "install-config.yaml")
			configData, err := yaml.Marshal(config)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(configPath, configData, 0644)
			Expect(err).NotTo(HaveOccurred())

			c.Opts.InstallConfig = configPath
			c.Opts.Package = "test-package.tar.gz"
			c.Opts.Force = true

			mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
			mockK0s.EXPECT().Install(mock.Anything, "/test/path/k0s", true).Return(nil)

			err = c.InstallK0sFromInstallConfig(mockPM, mockK0s)
			Expect(err).NotTo(HaveOccurred())
		})

		It("downloads k0s when package is not specified", func() {
			config := createTestConfig(true)
			configPath := filepath.Join(tempDir, "install-config.yaml")
			configData, err := yaml.Marshal(config)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(configPath, configData, 0644)
			Expect(err).NotTo(HaveOccurred())

			c.Opts.InstallConfig = configPath
			c.Opts.Package = ""
			c.Opts.Version = "v1.29.0+k0s.0"

			mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
			mockK0s.EXPECT().Download("v1.29.0+k0s.0", false, false).Return("/downloaded/k0s", nil)
			mockK0s.EXPECT().Install(mock.Anything, "/downloaded/k0s", false).Return(nil)

			err = c.InstallK0sFromInstallConfig(mockPM, mockK0s)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails when k0s download fails", func() {
			config := createTestConfig(true)
			configPath := filepath.Join(tempDir, "install-config.yaml")
			configData, err := yaml.Marshal(config)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(configPath, configData, 0644)
			Expect(err).NotTo(HaveOccurred())

			c.Opts.InstallConfig = configPath
			c.Opts.Package = ""

			mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
			mockK0s.EXPECT().Download("", false, false).Return("", os.ErrNotExist)

			err = c.InstallK0sFromInstallConfig(mockPM, mockK0s)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download k0s"))
		})

		It("fails when k0s install fails", func() {
			config := createTestConfig(true)
			configPath := filepath.Join(tempDir, "install-config.yaml")
			configData, err := yaml.Marshal(config)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(configPath, configData, 0644)
			Expect(err).NotTo(HaveOccurred())

			c.Opts.InstallConfig = configPath
			c.Opts.Package = "test-package.tar.gz"

			mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
			mockK0s.EXPECT().Install(mock.Anything, "/test/path/k0s", false).Return(os.ErrPermission)

			err = c.InstallK0sFromInstallConfig(mockPM, mockK0s)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install k0s"))
		})

		It("handles remote installation when remote-host is specified", func() {
			config := createTestConfig(true)
			configPath := filepath.Join(tempDir, "install-config.yaml")
			configData, err := yaml.Marshal(config)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(configPath, configData, 0644)
			Expect(err).NotTo(HaveOccurred())

			c.Opts.InstallConfig = configPath
			c.Opts.Package = "test-package.tar.gz"
			c.Opts.RemoteHost = "192.168.1.50"
			c.Opts.SSHKeyPath = "/path/to/key"

			mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")

			// Remote installation will fail because we can't actually connect,
			// but we're testing that it attempts remote installation
			err = c.InstallK0sFromInstallConfig(mockPM, mockK0s)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install k0s on remote host"))
		})
	})

	Context("InstallK0sRemote method", func() {
		var (
			config *files.RootConfig
		)

		BeforeEach(func() {
			config = &files.RootConfig{
				Datacenter: files.DatacenterConfig{
					ID:   1,
					Name: "test-dc",
				},
				Kubernetes: files.KubernetesConfig{
					ManagedByCodesphere: true,
					ControlPlanes: []files.K8sNode{
						{IPAddress: "192.168.1.100"},
					},
				},
			}
		})

		It("fails when SSH connection cannot be established", func() {
			c.Opts.RemoteHost = "192.0.2.1" // TEST-NET-1, should fail to connect
			c.Opts.SSHKeyPath = "/tmp/nonexistent-key"

			err := c.InstallK0sRemote(config, "/path/to/k0s", "/path/to/config")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install k0s on remote host"))
		})
	})
})
