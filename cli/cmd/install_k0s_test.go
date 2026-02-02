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
			Expect(err.Error()).To(ContainSubstring("install-config"))
		})
	})

	Context("InstallK0s method", func() {
		var (
			mockPM     *installer.MockPackageManager
			mockK0s    *installer.MockK0sManager
			mockK0sctl *installer.MockK0sctlManager
			tempDir    string
		)

		BeforeEach(func() {
			mockPM = installer.NewMockPackageManager(GinkgoT())
			mockK0s = installer.NewMockK0sManager(GinkgoT())
			mockK0sctl = installer.NewMockK0sctlManager(GinkgoT())
			var err error
			tempDir, err = os.MkdirTemp("", "install-k0s-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			mockPM.AssertExpectations(GinkgoT())
			mockK0s.AssertExpectations(GinkgoT())
			mockK0sctl.AssertExpectations(GinkgoT())
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

			err := c.InstallK0s(mockPM, mockK0s, mockK0sctl)
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

			err = c.InstallK0s(mockPM, mockK0s, mockK0sctl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("external Kubernetes"))
		})

		It("successfully installs k0s with valid config using k0sctl", func() {
			config := createTestConfig(true)
			configPath := filepath.Join(tempDir, "install-config.yaml")
			configData, err := yaml.Marshal(config)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(configPath, configData, 0644)
			Expect(err).NotTo(HaveOccurred())

			c.Opts.InstallConfig = configPath
			c.Opts.Package = "test-package.tar.gz"
			c.Opts.Version = "v1.30.0+k0s.0"
			c.Opts.Force = true

			mockEnv.EXPECT().GetOmsWorkdir().Return(tempDir)
			mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
			mockK0sctl.EXPECT().Download("", true, false).Return("/tmp/k0sctl", nil)
			mockFileWriter.EXPECT().WriteFile(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mockK0sctl.EXPECT().Apply(mock.Anything, "/tmp/k0sctl", true).Return(nil)

			err = c.InstallK0s(mockPM, mockK0s, mockK0sctl)
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

			mockEnv.EXPECT().GetOmsWorkdir().Return(tempDir)
			mockK0s.EXPECT().Download("v1.29.0+k0s.0", false, false).Return("/downloaded/k0s", nil)
			mockK0sctl.EXPECT().Download("", false, false).Return("/tmp/k0sctl", nil)
			mockFileWriter.EXPECT().WriteFile(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mockK0sctl.EXPECT().Apply(mock.Anything, "/tmp/k0sctl", false).Return(nil)

			err = c.InstallK0s(mockPM, mockK0s, mockK0sctl)
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

			mockK0s.EXPECT().GetLatestVersion().Return("v1.30.0+k0s.0", nil)
			mockK0s.EXPECT().Download("v1.30.0+k0s.0", false, false).Return("", os.ErrNotExist)

			err = c.InstallK0s(mockPM, mockK0s, mockK0sctl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download k0s"))
		})

		It("fails when k0sctl download fails", func() {
			config := createTestConfig(true)
			configPath := filepath.Join(tempDir, "install-config.yaml")
			configData, err := yaml.Marshal(config)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(configPath, configData, 0644)
			Expect(err).NotTo(HaveOccurred())

			c.Opts.InstallConfig = configPath
			c.Opts.Package = "test-package.tar.gz"
			c.Opts.Version = "v1.30.0+k0s.0"

			mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
			mockK0sctl.EXPECT().Download("", false, false).Return("", os.ErrPermission)

			err = c.InstallK0s(mockPM, mockK0s, mockK0sctl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download k0sctl"))
		})

		It("fails when k0sctl apply fails", func() {
			config := createTestConfig(true)
			configPath := filepath.Join(tempDir, "install-config.yaml")
			configData, err := yaml.Marshal(config)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(configPath, configData, 0644)
			Expect(err).NotTo(HaveOccurred())

			c.Opts.InstallConfig = configPath
			c.Opts.Package = "test-package.tar.gz"
			c.Opts.Version = "v1.30.0+k0s.0"

			mockEnv.EXPECT().GetOmsWorkdir().Return(tempDir)
			mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
			mockK0sctl.EXPECT().Download("", false, false).Return("/tmp/k0sctl", nil)
			mockFileWriter.EXPECT().WriteFile(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mockK0sctl.EXPECT().Apply(mock.Anything, "/tmp/k0sctl", false).Return(os.ErrPermission)

			err = c.InstallK0s(mockPM, mockK0s, mockK0sctl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to apply k0sctl config"))
		})
	})
})
