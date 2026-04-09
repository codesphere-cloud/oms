// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"bytes"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

type MockFileIO struct {
	files         map[string][]byte
	createError   error
	openError     error
	writeError    error
	existsResult  bool
	isDirResult   bool
	mkdirAllError error
}

func NewMockFileIO() *MockFileIO {
	return &MockFileIO{
		files: make(map[string][]byte),
	}
}

func (m *MockFileIO) Create(filename string) (*os.File, error) {
	if m.createError != nil {
		return nil, m.createError
	}
	return nil, nil
}

func (m *MockFileIO) CreateAndWrite(filePath string, data []byte, fileType string) error {
	if m.writeError != nil {
		return m.writeError
	}
	m.files[filePath] = data
	return nil
}

func (m *MockFileIO) Open(filename string) (*os.File, error) {
	if m.openError != nil {
		return nil, m.openError
	}
	return nil, nil
}

func (m *MockFileIO) OpenAppend(filename string) (*os.File, error) {
	return nil, nil
}

func (m *MockFileIO) Exists(path string) bool {
	_, exists := m.files[path]
	return exists || m.existsResult
}

func (m *MockFileIO) IsDirectory(path string) (bool, error) {
	return m.isDirResult, nil
}

func (m *MockFileIO) MkdirAll(path string, perm os.FileMode) error {
	return m.mkdirAllError
}

func (m *MockFileIO) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return nil, nil
}

func (m *MockFileIO) WriteFile(filename string, data []byte, perm os.FileMode) error {
	if m.writeError != nil {
		return m.writeError
	}
	m.files[filename] = data
	return nil
}

func (m *MockFileIO) ReadDir(dirname string) ([]os.DirEntry, error) {
	return nil, nil
}

func (m *MockFileIO) ReadFile(filename string) ([]byte, error) {
	if data, ok := m.files[filename]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (m *MockFileIO) Remove(path string) error {
	delete(m.files, path)
	return nil
}

func (m *MockFileIO) Chmod(name string, mode os.FileMode) error {
	return nil
}

func (m *MockFileIO) GetFileContent(path string) []byte {
	return m.files[path]
}

type MockFile struct {
	*bytes.Buffer
	closed bool
}

func (m *MockFile) Close() error {
	m.closed = true
	return nil
}

var _ = Describe("ConfigManager", func() {
	var (
		configManager *installer.InstallConfig
	)

	BeforeEach(func() {
		configManager = &installer.InstallConfig{
			Config: &files.RootConfig{},
		}
	})

	Describe("NewInstallConfigManager", func() {
		It("should create a new config manager", func() {
			manager := installer.NewInstallConfigManager()
			Expect(manager).ToNot(BeNil())
		})
	})

	Describe("IsValidIP", func() {
		It("should validate correct IPv4 addresses", func() {
			Expect(installer.IsValidIP("192.168.1.1")).To(BeTrue())
			Expect(installer.IsValidIP("10.0.0.1")).To(BeTrue())
			Expect(installer.IsValidIP("127.0.0.1")).To(BeTrue())
		})

		It("should validate correct IPv6 addresses", func() {
			Expect(installer.IsValidIP("::1")).To(BeTrue())
			Expect(installer.IsValidIP("2001:db8::1")).To(BeTrue())
		})

		It("should reject invalid IP addresses", func() {
			Expect(installer.IsValidIP("256.1.1.1")).To(BeFalse())
			Expect(installer.IsValidIP("not-an-ip")).To(BeFalse())
			Expect(installer.IsValidIP("")).To(BeFalse())
			Expect(installer.IsValidIP("192.168.1")).To(BeFalse())
		})
	})

	Describe("ValidateInstallConfig", func() {
		Context("with nil config", func() {
			It("should return error", func() {
				configManager.Config = nil
				errors := configManager.ValidateInstallConfig()
				Expect(errors).To(HaveLen(1))
				Expect(errors[0]).To(ContainSubstring("config not set"))
			})
		})

		Context("with empty config", func() {
			It("should return multiple validation errors", func() {
				configManager.Config = &files.RootConfig{}
				errors := configManager.ValidateInstallConfig()
				Expect(errors).ToNot(BeEmpty())
			})
		})

		Context("datacenter validation", func() {
			It("should require datacenter ID", func() {
				configManager.Config.Datacenter.ID = 0
				errors := configManager.ValidateInstallConfig()
				Expect(errors).To(ContainElement(ContainSubstring("datacenter ID is required")))
			})

			It("should require datacenter name", func() {
				configManager.Config.Datacenter.Name = ""
				errors := configManager.ValidateInstallConfig()
				Expect(errors).To(ContainElement(ContainSubstring("datacenter name is required")))
			})
		})

		Context("postgres validation", func() {
			It("should require postgres mode", func() {
				configManager.Config.Postgres.Mode = ""
				errors := configManager.ValidateInstallConfig()
				Expect(errors).To(ContainElement(ContainSubstring("postgres mode is required")))
			})

			It("should reject invalid postgres mode", func() {
				configManager.Config.Postgres.Mode = "invalid"
				errors := configManager.ValidateInstallConfig()
				Expect(errors).To(ContainElement(ContainSubstring("invalid postgres mode")))
			})

			Context("install mode", func() {
				BeforeEach(func() {
					configManager.Config.Postgres.Mode = "install"
				})

				It("should require primary configuration", func() {
					configManager.Config.Postgres.Primary = nil
					errors := configManager.ValidateInstallConfig()
					Expect(errors).To(ContainElement(ContainSubstring("postgres primary configuration is required")))
				})

				It("should require primary IP", func() {
					configManager.Config.Postgres.Mode = "install"
					configManager.Config.Postgres.Primary = &files.PostgresPrimaryConfig{
						IP:       "",
						Hostname: "pg-primary",
					}
					errors := configManager.ValidateInstallConfig()
					Expect(errors).To(ContainElement(ContainSubstring("postgres primary IP is required")))
				})

				It("should require primary hostname", func() {
					configManager.Config.Postgres.Mode = "install"
					configManager.Config.Postgres.Primary = &files.PostgresPrimaryConfig{
						IP:       "10.50.0.2",
						Hostname: "",
					}
					errors := configManager.ValidateInstallConfig()
					Expect(errors).To(ContainElement(ContainSubstring("postgres primary hostname is required")))
				})
			})

			Context("external mode", func() {
				It("should require server address", func() {
					configManager.Config.Postgres.Mode = "external"
					configManager.Config.Postgres.ServerAddress = ""
					errors := configManager.ValidateInstallConfig()
					Expect(errors).To(ContainElement(ContainSubstring("postgres server address is required")))
				})
			})

		})

		Context("openBao validation", func() {
			BeforeEach(func() {
				configManager.Config.Codesphere.OpenBao = &files.OpenBaoConfig{
					URI:    "https://openbao.example.com",
					Engine: "openbao-engine",
					User:   "fake-user",
				}
			})

			It("should require OpenBao URI", func() {
				configManager.Config.Codesphere.OpenBao.URI = ""
				errors := configManager.ValidateInstallConfig()
				Expect(errors).To(ContainElement(ContainSubstring("OpenBao URI is required")))
			})

			It("should require OpenBao engine", func() {
				configManager.Config.Codesphere.OpenBao.Engine = ""
				errors := configManager.ValidateInstallConfig()
				Expect(errors).To(ContainElement(ContainSubstring("OpenBao engine name is required")))
			})

			It("should require OpenBao user", func() {
				configManager.Config.Codesphere.OpenBao.User = ""
				errors := configManager.ValidateInstallConfig()
				Expect(errors).To(ContainElement(ContainSubstring("OpenBao username is required")))
			})

			It("should validate OpenBao URI format", func() {
				configManager.Config.Codesphere.OpenBao.URI = "not-a-valid-url"
				errors := configManager.ValidateInstallConfig()
				Expect(errors).To(ContainElement(ContainSubstring("OpenBao URI must be a valid URL")))
			})

			It("should require OpenBao password in vault", func() {
				configManager.Vault = &files.InstallVault{
					Secrets: []files.SecretEntry{
						{Name: "cephSshPrivateKey"},
					},
				}
				errors := configManager.ValidateVault()
				Expect(errors).To(ContainElement(ContainSubstring("required OpenBao secret missing: openBaoPassword")))
			})
		})

		Context("ceph validation", func() {
			It("should require at least one Ceph host", func() {
				configManager.Config.Ceph.Hosts = []files.CephHost{}
				errors := configManager.ValidateInstallConfig()
				Expect(errors).To(ContainElement(ContainSubstring("at least one Ceph host is required")))
			})

			It("should validate Ceph host IPs", func() {
				configManager.Config.Ceph.Hosts = []files.CephHost{
					{Hostname: "ceph-0", IPAddress: "invalid-ip", IsMaster: true},
				}
				errors := configManager.ValidateInstallConfig()
				Expect(errors).To(ContainElement(ContainSubstring("invalid Ceph host IP")))
			})
		})

		Context("kubernetes validation", func() {
			Context("managed by Codesphere", func() {
				BeforeEach(func() {
					configManager.Config.Kubernetes.ManagedByCodesphere = true
				})

				It("should require at least one control plane node", func() {
					configManager.Config.Kubernetes.ControlPlanes = []files.K8sNode{}
					errors := configManager.ValidateInstallConfig()
					Expect(errors).To(ContainElement(ContainSubstring("at least one K8s control plane node is required")))
				})
			})

			Context("external cluster", func() {
				BeforeEach(func() {
					configManager.Config.Kubernetes.ManagedByCodesphere = false
				})

				It("should require pod CIDR", func() {
					configManager.Config.Kubernetes.PodCIDR = ""
					errors := configManager.ValidateInstallConfig()
					Expect(errors).To(ContainElement(ContainSubstring("pod CIDR is required")))
				})

				It("should require service CIDR", func() {
					configManager.Config.Kubernetes.ServiceCIDR = ""
					errors := configManager.ValidateInstallConfig()
					Expect(errors).To(ContainElement(ContainSubstring("service CIDR is required")))
				})
			})
		})

		Context("codesphere validation", func() {
			It("should require Codesphere domain", func() {
				configManager.Config.Codesphere.Domain = ""
				errors := configManager.ValidateInstallConfig()
				Expect(errors).To(ContainElement(ContainSubstring("Codesphere domain is required")))
			})
		})

		Context("with valid configuration", func() {
			BeforeEach(func() {
				configManager.Config = &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Postgres: files.PostgresConfig{
						Mode: "install",
						Primary: &files.PostgresPrimaryConfig{
							IP:       "10.50.0.2",
							Hostname: "pg-primary",
						},
					},
					Ceph: files.CephConfig{
						Hosts: []files.CephHost{
							{Hostname: "ceph-0", IPAddress: "10.53.101.2", IsMaster: true},
						},
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes:       []files.K8sNode{{IPAddress: "10.50.0.2"}},
					},
					Codesphere: files.CodesphereConfig{
						Domain: "codesphere.example.com",
					},
				}
			})

			It("should return no errors", func() {
				errors := configManager.ValidateInstallConfig()
				Expect(errors).To(BeEmpty())
			})
		})
	})

	Describe("ValidateVault", func() {
		Context("with nil vault", func() {
			It("should return error", func() {
				configManager.Vault = nil
				errors := configManager.ValidateVault()
				Expect(errors).To(HaveLen(1))
				Expect(errors[0]).To(ContainSubstring("vault not set"))
			})
		})

		Context("with empty vault", func() {
			It("should return errors for missing required secrets", func() {
				configManager.Vault = &files.InstallVault{
					Secrets: []files.SecretEntry{},
				}
				errors := configManager.ValidateVault()
				Expect(errors).ToNot(BeEmpty())
				Expect(errors).To(ContainElement(ContainSubstring("cephSshPrivateKey")))
				Expect(errors).To(ContainElement(ContainSubstring("selfSignedCaKeyPem")))
				Expect(errors).To(ContainElement(ContainSubstring("domainAuthPrivateKey")))
				Expect(errors).To(ContainElement(ContainSubstring("domainAuthPublicKey")))
			})
		})

		Context("with valid vault", func() {
			BeforeEach(func() {
				configManager.Vault = &files.InstallVault{
					Secrets: []files.SecretEntry{
						{Name: "cephSshPrivateKey"},
						{Name: "selfSignedCaKeyPem"},
						{Name: "domainAuthPrivateKey"},
						{Name: "domainAuthPublicKey"},
					},
				}
			})

			It("should return no errors", func() {
				errors := configManager.ValidateVault()
				Expect(errors).To(BeEmpty())
			})
		})

		Context("with partial vault", func() {
			It("should return errors for missing secrets only", func() {
				configManager.Vault = &files.InstallVault{
					Secrets: []files.SecretEntry{
						{Name: "cephSshPrivateKey"},
						{Name: "selfSignedCaKeyPem"},
					},
				}
				errors := configManager.ValidateVault()
				Expect(errors).To(HaveLen(2))
				Expect(errors).To(ContainElement(ContainSubstring("domainAuthPrivateKey")))
				Expect(errors).To(ContainElement(ContainSubstring("domainAuthPublicKey")))
			})
		})
	})

	Describe("GetInstallConfig", func() {
		It("should return the current config", func() {
			testConfig := &files.RootConfig{
				Datacenter: files.DatacenterConfig{
					ID:   42,
					Name: "test",
				},
			}
			configManager.Config = testConfig

			result := configManager.GetInstallConfig()
			Expect(result).To(Equal(testConfig))
			Expect(result.Datacenter.ID).To(Equal(42))
		})
	})

	Describe("AddConfigComments", func() {
		It("should add header comments to YAML data", func() {
			yamlData := []byte("datacenter:\n  id: 1\n")
			result := installer.AddConfigComments(yamlData)

			resultStr := string(result)
			Expect(resultStr).To(ContainSubstring("Codesphere Installer Configuration"))
			Expect(resultStr).To(ContainSubstring("Generated by OMS CLI"))
			Expect(resultStr).To(ContainSubstring("datacenter:"))
		})
	})

	Describe("AddVaultComments", func() {
		It("should add security warning header to vault YAML", func() {
			yamlData := []byte("secrets:\n  - name: test\n")
			result := installer.AddVaultComments(yamlData)

			resultStr := string(result)
			Expect(resultStr).To(ContainSubstring("Codesphere Installer Secrets"))
			Expect(resultStr).To(ContainSubstring("IMPORTANT: This file contains sensitive information!"))
			Expect(resultStr).To(ContainSubstring("SOPS"))
			Expect(resultStr).To(ContainSubstring("Age"))
			Expect(resultStr).To(ContainSubstring("secrets:"))
		})
	})

	Describe("WriteInstallConfig", func() {
		It("should return error if config is nil", func() {
			configManager.Config = nil
			err := configManager.WriteInstallConfig("/tmp/config.yaml", false)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no configuration provided"))
		})
	})

	Describe("WriteVault", func() {
		It("should return error if config is nil", func() {
			configManager.Config = nil
			err := configManager.WriteVault("/tmp/vault.yaml", false)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no configuration provided"))
		})
	})

	Describe("Integration Tests", func() {
		Context("full configuration lifecycle", func() {
			It("should apply profile, validate, and prepare for write", func() {
				err := configManager.ApplyProfile("dev")
				Expect(err).ToNot(HaveOccurred())

				errors := configManager.ValidateInstallConfig()
				Expect(errors).To(BeEmpty())

				config := configManager.GetInstallConfig()
				Expect(config).ToNot(BeNil())
				Expect(config.Datacenter.Name).To(Equal("dev"))
			})

			It("should generate secrets and create valid vault", func() {
				err := configManager.ApplyProfile("prod")
				Expect(err).ToNot(HaveOccurred())

				err = configManager.GenerateSecrets()
				Expect(err).ToNot(HaveOccurred())

				vault := configManager.Config.ExtractVault()
				configManager.Vault = vault

				errors := configManager.ValidateVault()
				Expect(errors).To(BeEmpty())
			})
		})

		Context("vault deduplication on re-write", func() {
			It("should not produce duplicate vault entries when WriteVault is called after loading existing vault", func() {
				err := configManager.ApplyProfile("prod")
				Expect(err).ToNot(HaveOccurred())

				err = configManager.GenerateSecrets()
				Expect(err).ToNot(HaveOccurred())

				// Simulate first write: extract vault from config
				vault1 := configManager.Config.ExtractVault()
				firstEntryCount := len(vault1.Secrets)
				Expect(firstEntryCount).To(BeNumerically(">", 0))

				// Simulate loading that vault back
				configManager.Vault = vault1

				// Write vault again
				vault2 := configManager.Config.ExtractVault()
				Expect(len(vault2.Secrets)).To(Equal(firstEntryCount), "vault should have same number of entries after re-write, no duplicates")

				// Verify no duplicate secret names
				nameCount := make(map[string]int)
				for _, secret := range vault2.Secrets {
					nameCount[secret.Name]++
				}
				for name, count := range nameCount {
					Expect(count).To(Equal(1), "secret %s appears %d times, expected 1", name, count)
				}
			})
		})

		Context("full bootstrap re-run simulation", func() {
			It("should preserve matching cert/key pairs across write → load → merge → re-write cycle", func() {
				mockIO := NewMockFileIO()
				configManager.SetFileIO(mockIO)

				// --- First run: generate everything from scratch ---
				err := configManager.ApplyProfile("prod")
				Expect(err).ToNot(HaveOccurred())

				err = configManager.GenerateSecrets()
				Expect(err).ToNot(HaveOccurred())

				// Verify cert/key pair matches after initial generation
				err = installer.ValidateCertKeyPair(
					configManager.Config.Postgres.Primary.SSLConfig.ServerCertPem,
					configManager.Config.Postgres.Primary.PrivateKey,
				)
				Expect(err).ToNot(HaveOccurred(), "cert/key should match after initial generation")

				// Save the original cert and key for later comparison
				origCert := configManager.Config.Postgres.Primary.SSLConfig.ServerCertPem
				origKey := configManager.Config.Postgres.Primary.PrivateKey
				Expect(origCert).ToNot(BeEmpty())
				Expect(origKey).ToNot(BeEmpty())

				// Write config and vault
				err = configManager.WriteInstallConfig("/tmp/config.yaml", false)
				Expect(err).ToNot(HaveOccurred())
				err = configManager.WriteVault("/tmp/vault.yaml", false)
				Expect(err).ToNot(HaveOccurred())

				// --- Second run: simulate loading existing files ---
				configManager2 := &installer.InstallConfig{}
				configManager2.SetFileIO(mockIO)

				// Reload config from written YAML
				configBytes := mockIO.GetFileContent("/tmp/config.yaml")
				Expect(configBytes).ToNot(BeNil())
				config2 := files.NewRootConfig()
				err = config2.Unmarshal(configBytes)
				Expect(err).ToNot(HaveOccurred())
				configManager2.Config = &config2

				Expect(configManager2.Config.Postgres.Primary.PrivateKey).To(BeEmpty(),
					"private key should NOT be in config.yaml (it has yaml:\"-\" tag)")
				Expect(configManager2.Config.Postgres.Primary.SSLConfig.ServerCertPem).To(Equal(origCert),
					"cert should be in config.yaml")

				// Reload vault from written YAML
				vaultBytes := mockIO.GetFileContent("/tmp/vault.yaml")
				Expect(vaultBytes).ToNot(BeNil())
				vault2 := &files.InstallVault{}
				err = vault2.Unmarshal(vaultBytes)
				Expect(err).ToNot(HaveOccurred())
				configManager2.Vault = vault2

				// Merge vault into config
				err = configManager2.MergeVaultIntoConfig()
				Expect(err).ToNot(HaveOccurred())

				// After merge, the private key should be restored from vault
				Expect(configManager2.Config.Postgres.Primary.PrivateKey).To(Equal(origKey),
					"private key should be restored from vault after merge")

				// Cert/key should still match
				err = installer.ValidateCertKeyPair(
					configManager2.Config.Postgres.Primary.SSLConfig.ServerCertPem,
					configManager2.Config.Postgres.Primary.PrivateKey,
				)
				Expect(err).ToNot(HaveOccurred(), "cert/key should match after load + merge")

				// Write vault again
				err = configManager2.WriteVault("/tmp/vault2.yaml", false)
				Expect(err).ToNot(HaveOccurred())

				// Verify no duplicates in re-written vault
				vault3 := &files.InstallVault{}
				vaultBytes2 := mockIO.GetFileContent("/tmp/vault2.yaml")
				err = vault3.Unmarshal(vaultBytes2)
				Expect(err).ToNot(HaveOccurred())

				nameCount := make(map[string]int)
				for _, secret := range vault3.Secrets {
					nameCount[secret.Name]++
				}
				for name, count := range nameCount {
					Expect(count).To(Equal(1), "secret '%s' has %d entries (expected 1) — duplication bug!", name, count)
				}

				// Verify the key in re-written vault still matches the cert
				var rewrittenKey string
				for _, secret := range vault3.Secrets {
					if secret.Name == "postgresPrimaryServerKeyPem" && secret.File != nil {
						rewrittenKey = secret.File.Content
					}
				}
				Expect(rewrittenKey).To(Equal(origKey),
					"re-written vault should contain the same key")
				err = installer.ValidateCertKeyPair(
					configManager2.Config.Postgres.Primary.SSLConfig.ServerCertPem,
					rewrittenKey,
				)
				Expect(err).ToNot(HaveOccurred(), "cert/key should match in re-written vault")
			})
		})

	})
})
