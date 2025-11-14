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

	})
})
