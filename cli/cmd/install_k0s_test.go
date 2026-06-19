// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

func execCmd(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}

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

		writeTestConfig := func(config *files.RootConfig) string {
			configPath := filepath.Join(tempDir, "install-config.yaml")
			configData, err := yaml.Marshal(config)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(configPath, configData, 0644)
			Expect(err).NotTo(HaveOccurred())
			return configPath
		}

		It("fails when install-config file does not exist", func() {
			c.Opts.InstallConfig = "/nonexistent/install-config.yaml"

			err := c.InstallK0s(mockPM, mockK0s, mockK0sctl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to load install-config"))
		})

		It("fails when install-config specifies external Kubernetes", func() {
			c.Opts.InstallConfig = writeTestConfig(createTestConfig(false))

			err := c.InstallK0s(mockPM, mockK0s, mockK0sctl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("external Kubernetes"))
		})

		It("successfully installs k0s with valid config using k0sctl", func() {
			c.Opts.InstallConfig = writeTestConfig(createTestConfig(true))
			c.Opts.Package = "test-package.tar.gz"
			c.Opts.Version = "v1.30.0+k0s.0"
			c.Opts.Force = true

			mockEnv.EXPECT().GetOmsWorkdir().Return(tempDir)
			mockPM.EXPECT().ExtractDependency("kubernetes/files/k0s", true).Return(nil)
			mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
			mockK0sctl.EXPECT().Download("", true, false).Return("/tmp/k0sctl", nil)
			mockFileWriter.EXPECT().WriteFile(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mockK0sctl.EXPECT().Apply(mock.Anything, "/tmp/k0sctl", true).Return(nil)

			err := c.InstallK0s(mockPM, mockK0s, mockK0sctl)
			Expect(err).NotTo(HaveOccurred())
		})

		It("downloads k0s when package is not specified", func() {
			c.Opts.InstallConfig = writeTestConfig(createTestConfig(true))
			c.Opts.Package = ""
			c.Opts.Version = "v1.29.0+k0s.0"

			mockEnv.EXPECT().GetOmsWorkdir().Return(tempDir)
			mockK0s.EXPECT().Download("v1.29.0+k0s.0", false, false).Return("/downloaded/k0s", nil)
			mockK0sctl.EXPECT().Download("", false, false).Return("/tmp/k0sctl", nil)
			mockFileWriter.EXPECT().WriteFile(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mockK0sctl.EXPECT().Apply(mock.Anything, "/tmp/k0sctl", false).Return(nil)

			err := c.InstallK0s(mockPM, mockK0s, mockK0sctl)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails when k0s download fails", func() {
			c.Opts.InstallConfig = writeTestConfig(createTestConfig(true))
			c.Opts.Package = ""

			mockK0s.EXPECT().GetLatestVersion().Return("v1.30.0+k0s.0", nil)
			mockK0s.EXPECT().Download("v1.30.0+k0s.0", false, false).Return("", os.ErrNotExist)

			err := c.InstallK0s(mockPM, mockK0s, mockK0sctl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download k0s"))
		})

		It("fails when k0sctl download fails", func() {
			c.Opts.InstallConfig = writeTestConfig(createTestConfig(true))
			c.Opts.Package = "test-package.tar.gz"
			c.Opts.Version = "v1.30.0+k0s.0"

			mockPM.EXPECT().ExtractDependency("kubernetes/files/k0s", false).Return(nil)
			mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
			mockK0sctl.EXPECT().Download("", false, false).Return("", os.ErrPermission)

			err := c.InstallK0s(mockPM, mockK0s, mockK0sctl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to download k0sctl"))
		})

		It("fails when k0sctl apply fails", func() {
			c.Opts.InstallConfig = writeTestConfig(createTestConfig(true))
			c.Opts.Package = "test-package.tar.gz"
			c.Opts.Version = "v1.30.0+k0s.0"

			mockEnv.EXPECT().GetOmsWorkdir().Return(tempDir)
			mockPM.EXPECT().ExtractDependency("kubernetes/files/k0s", false).Return(nil)
			mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
			mockK0sctl.EXPECT().Download("", false, false).Return("/tmp/k0sctl", nil)
			mockFileWriter.EXPECT().WriteFile(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mockK0sctl.EXPECT().Apply(mock.Anything, "/tmp/k0sctl", false).Return(os.ErrPermission)

			err := c.InstallK0s(mockPM, mockK0s, mockK0sctl)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to apply k0sctl config"))
		})

		setupCommonMocks := func() {
			mockEnv.EXPECT().GetOmsWorkdir().Return(tempDir)
			mockPM.EXPECT().ExtractDependency("kubernetes/files/k0s", false).Return(nil)
			mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
			mockK0sctl.EXPECT().Download("", false, false).Return("/tmp/k0sctl", nil)
			mockFileWriter.EXPECT().WriteFile(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mockK0sctl.EXPECT().Apply(mock.Anything, "/tmp/k0sctl", false).Return(nil)
		}

		Context("with --vault flag", func() {
			It("saves kubeconfig to a new vault", func() {
				c.Opts.InstallConfig = writeTestConfig(createTestConfig(true))
				c.Opts.Package = "test-package.tar.gz"
				c.Opts.Version = "v1.30.0+k0s.0"
				c.Opts.Vault = filepath.Join(tempDir, "prod.vault.yaml")

				setupCommonMocks()
				mockK0sctl.EXPECT().GetKubeconfig(mock.Anything, "/tmp/k0sctl").Return("apiVersion: v1\nkind: Config\n", nil)
				mockFileWriter.EXPECT().Exists(c.Opts.Vault).Return(false)
				mockFileWriter.EXPECT().WriteFile(c.Opts.Vault, mock.Anything, os.FileMode(0600)).Return(nil)

				err := c.InstallK0s(mockPM, mockK0s, mockK0sctl)
				Expect(err).NotTo(HaveOccurred())
			})

			It("saves kubeconfig to an existing vault", func() {
				c.Opts.InstallConfig = writeTestConfig(createTestConfig(true))
				c.Opts.Package = "test-package.tar.gz"
				c.Opts.Version = "v1.30.0+k0s.0"
				c.Opts.Vault = filepath.Join(tempDir, "prod.vault.yaml")

				// Create an existing vault with a different secret
				existingVault := &files.InstallVault{
					Secrets: []files.SecretEntry{
						{
							Name: "domainAuthPrivateKey",
							File: &files.SecretFile{
								Name:    "key.pem",
								Content: "existing-key-content",
							},
						},
					},
				}
				vaultYAML, err := existingVault.Marshal()
				Expect(err).NotTo(HaveOccurred())
				err = os.WriteFile(c.Opts.Vault, vaultYAML, 0600)
				Expect(err).NotTo(HaveOccurred())

				setupCommonMocks()
				mockK0sctl.EXPECT().GetKubeconfig(mock.Anything, "/tmp/k0sctl").Return("apiVersion: v1\nkind: Config\n", nil)
				mockFileWriter.EXPECT().Exists(c.Opts.Vault).Return(true)
				mockFileWriter.EXPECT().WriteFile(c.Opts.Vault, mock.Anything, os.FileMode(0600)).Return(nil)

				err = c.InstallK0s(mockPM, mockK0s, mockK0sctl)
				Expect(err).NotTo(HaveOccurred())
			})

			It("overwrites existing kubeConfig secret in vault", func() {
				c.Opts.InstallConfig = writeTestConfig(createTestConfig(true))
				c.Opts.Package = "test-package.tar.gz"
				c.Opts.Version = "v1.30.0+k0s.0"
				c.Opts.Vault = filepath.Join(tempDir, "prod.vault.yaml")

				// Create an existing vault that already has a kubeConfig secret with old content
				existingVault := &files.InstallVault{
					Secrets: []files.SecretEntry{
						{
							Name: "kubeConfig",
							File: &files.SecretFile{
								Name:    "kubeConfig",
								Content: "old-kubeconfig-content",
							},
						},
					},
				}
				vaultYAML, err := existingVault.Marshal()
				Expect(err).NotTo(HaveOccurred())
				err = os.WriteFile(c.Opts.Vault, vaultYAML, 0600)
				Expect(err).NotTo(HaveOccurred())

				setupCommonMocks()
				mockK0sctl.EXPECT().GetKubeconfig(mock.Anything, "/tmp/k0sctl").Return("apiVersion: v1\nkind: Config\nnew: true\n", nil)
				mockFileWriter.EXPECT().Exists(c.Opts.Vault).Return(true)
				mockFileWriter.EXPECT().WriteFile(c.Opts.Vault, mock.Anything, os.FileMode(0600)).Return(nil)

				err = c.InstallK0s(mockPM, mockK0s, mockK0sctl)
				Expect(err).NotTo(HaveOccurred())
			})

			It("fails when GetKubeconfig fails", func() {
				c.Opts.InstallConfig = writeTestConfig(createTestConfig(true))
				c.Opts.Package = "test-package.tar.gz"
				c.Opts.Version = "v1.30.0+k0s.0"
				c.Opts.Vault = filepath.Join(tempDir, "prod.vault.yaml")

				setupCommonMocks()
				mockK0sctl.EXPECT().GetKubeconfig(mock.Anything, "/tmp/k0sctl").Return("", os.ErrPermission)

				err := c.InstallK0s(mockPM, mockK0s, mockK0sctl)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to retrieve kubeconfig from k0sctl"))
			})

			It("re-encrypts vault after saving kubeconfig when vault was SOPS-encrypted", func() {
				if !sopsAndAgeAvailable() {
					Skip("sops and age-keygen not available")
				}

				c.FileWriter = util.NewFilesystemWriter()

				ageKeyPath := filepath.Join(tempDir, "age_key.txt")
				out, err := execCmd("age-keygen", "-o", ageKeyPath)
				Expect(err).NotTo(HaveOccurred(), string(out))

				recipientOut, err := execCmd("age-keygen", "-y", ageKeyPath)
				Expect(err).NotTo(HaveOccurred(), string(recipientOut))
				recipient := strings.TrimSpace(string(recipientOut))

				vaultPath := filepath.Join(tempDir, "prod.vault.yaml")
				existingVault := &files.InstallVault{
					Secrets: []files.SecretEntry{
						{
							Name: "domainAuthPrivateKey",
							File: &files.SecretFile{
								Name:    "key.pem",
								Content: "existing-key-content",
							},
						},
					},
				}
				vaultYAML, err := existingVault.Marshal()
				Expect(err).NotTo(HaveOccurred())
				plainPath := vaultPath + ".plain"
				err = os.WriteFile(plainPath, vaultYAML, 0600)
				Expect(err).NotTo(HaveOccurred())

				encryptOut, err := execCmd("sops", "--encrypt", "--age", recipient, "--output", vaultPath, plainPath)
				Expect(err).NotTo(HaveOccurred(), string(encryptOut))
				Expect(os.Remove(plainPath)).To(Succeed())

				encrypted, err := installer.IsSOPSEncryptedFile(vaultPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(encrypted).To(BeTrue())

				c.Opts.InstallConfig = writeTestConfig(createTestConfig(true))
				c.Opts.Package = "test-package.tar.gz"
				c.Opts.Version = "v1.30.0+k0s.0"
				c.Opts.Vault = vaultPath
				c.Opts.VaultPrivKey = ageKeyPath

				mockEnv.EXPECT().GetOmsWorkdir().Return(tempDir)
				mockPM.EXPECT().ExtractDependency("kubernetes/files/k0s", false).Return(nil)
				mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
				mockK0sctl.EXPECT().Download("", false, false).Return("/tmp/k0sctl", nil)
				mockK0sctl.EXPECT().Apply(mock.Anything, "/tmp/k0sctl", false).Return(nil)
				mockK0sctl.EXPECT().GetKubeconfig(mock.Anything, "/tmp/k0sctl").Return("apiVersion: v1\nkind: Config\n", nil)

				err = c.InstallK0s(mockPM, mockK0s, mockK0sctl)
				Expect(err).NotTo(HaveOccurred())

				// Verify the vault was re-encrypted after saving kubeconfig.
				encrypted, err = installer.IsSOPSEncryptedFile(vaultPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(encrypted).To(BeTrue(), "vault should be re-encrypted after saving kubeconfig")

				// Verify the temporary file was cleaned up.
				tmpPath := vaultPath + ".tmp"
				Expect(tmpPath).NotTo(BeAnExistingFile())
			})

			It("leaves the vault untouched when re-encryption fails", func() {
				if !sopsAndAgeAvailable() {
					Skip("sops and age-keygen not available")
				}

				c.FileWriter = util.NewFilesystemWriter()

				ageKeyPath := filepath.Join(tempDir, "age_key.txt")
				out, err := execCmd("age-keygen", "-o", ageKeyPath)
				Expect(err).NotTo(HaveOccurred(), string(out))

				recipientOut, err := execCmd("age-keygen", "-y", ageKeyPath)
				Expect(err).NotTo(HaveOccurred(), string(recipientOut))
				recipient := strings.TrimSpace(string(recipientOut))

				vaultPath := filepath.Join(tempDir, "prod.vault.yaml")
				existingVault := &files.InstallVault{
					Secrets: []files.SecretEntry{
						{
							Name: "domainAuthPrivateKey",
							File: &files.SecretFile{
								Name:    "key.pem",
								Content: "existing-key-content",
							},
						},
					},
				}
				vaultYAML, err := existingVault.Marshal()
				Expect(err).NotTo(HaveOccurred())
				plainPath := vaultPath + ".plain"
				err = os.WriteFile(plainPath, vaultYAML, 0600)
				Expect(err).NotTo(HaveOccurred())

				encryptOut, err := execCmd("sops", "--encrypt", "--age", recipient, "--output", vaultPath, plainPath)
				Expect(err).NotTo(HaveOccurred(), string(encryptOut))
				Expect(os.Remove(plainPath)).To(Succeed())

				// Remember the original encrypted vault content.
				origData, err := os.ReadFile(vaultPath)
				Expect(err).NotTo(HaveOccurred())

				c.Opts.InstallConfig = writeTestConfig(createTestConfig(true))
				c.Opts.Package = "test-package.tar.gz"
				c.Opts.Version = "v1.30.0+k0s.0"
				c.Opts.Vault = vaultPath
				// Provide a non-existent key file so ResolveAgeKey fails.
				c.Opts.VaultPrivKey = filepath.Join(tempDir, "missing_key.txt")

				mockEnv.EXPECT().GetOmsWorkdir().Return(tempDir)
				mockPM.EXPECT().ExtractDependency("kubernetes/files/k0s", false).Return(nil)
				mockPM.EXPECT().GetDependencyPath("kubernetes/files/k0s").Return("/test/path/k0s")
				mockK0sctl.EXPECT().Download("", false, false).Return("/tmp/k0sctl", nil)
				mockK0sctl.EXPECT().Apply(mock.Anything, "/tmp/k0sctl", false).Return(nil)
				mockK0sctl.EXPECT().GetKubeconfig(mock.Anything, "/tmp/k0sctl").Return("apiVersion: v1\nkind: Config\n", nil)

				err = c.InstallK0s(mockPM, mockK0s, mockK0sctl)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load vault"))

				// Verify the vault file is unchanged.
				currentData, err := os.ReadFile(vaultPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(currentData)).To(Equal(string(origData)),
					"vault should be untouched when re-encryption fails")

				// Verify no tmp file is left behind.
				tmpPath := vaultPath + ".tmp"
				Expect(tmpPath).NotTo(BeAnExistingFile())
			})
		})
	})
})
