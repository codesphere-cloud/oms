// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("K0sctlConfig", func() {
	Describe("GenerateK0sctlConfig", func() {
		Context("with valid install-config", func() {
			It("should generate k0sctl config with control plane nodes", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						APIServerHost:       "k8s.example.com",
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.1.10"},
							{IPAddress: "10.0.1.11"},
							{IPAddress: "10.0.1.12"},
						},
						PodCIDR:     "10.244.0.0/16",
						ServiceCIDR: "10.96.0.0/12",
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "/path/to/k0s")
				Expect(err).ToNot(HaveOccurred())
				Expect(k0sctlConfig).ToNot(BeNil())

				// Check basic structure
				Expect(k0sctlConfig.APIVersion).To(Equal("k0sctl.k0sproject.io/v1beta1"))
				Expect(k0sctlConfig.Kind).To(Equal("Cluster"))
				Expect(k0sctlConfig.Metadata.Name).To(Equal("codesphere-test-dc"))

				// Check k0s version
				Expect(k0sctlConfig.Spec.K0s.Version).To(Equal("v1.30.0+k0s.0"))

				// Check hosts count matches control planes
				Expect(k0sctlConfig.Spec.Hosts).To(HaveLen(3))
			})

			It("should assign controller+worker role to control plane nodes", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.1.10"},
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())

				Expect(k0sctlConfig.Spec.Hosts).To(HaveLen(1))
				Expect(k0sctlConfig.Spec.Hosts[0].Role).To(Equal("controller+worker"))
				Expect(k0sctlConfig.Spec.Hosts[0].InstallFlags).To(ContainElements("--enable-worker", "--no-taints"))
			})

			It("should assign worker role to dedicated worker nodes", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.1.10"},
						},
						Workers: []files.K8sNode{
							{IPAddress: "10.0.2.10"},
							{IPAddress: "10.0.2.11"},
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())

				Expect(k0sctlConfig.Spec.Hosts).To(HaveLen(3))

				// First host should be controller+worker
				Expect(k0sctlConfig.Spec.Hosts[0].Role).To(Equal("controller+worker"))

				// Worker nodes should have worker role with no install flags
				Expect(k0sctlConfig.Spec.Hosts[1].Role).To(Equal("worker"))
				Expect(k0sctlConfig.Spec.Hosts[1].InstallFlags).To(BeNil())
				Expect(k0sctlConfig.Spec.Hosts[2].Role).To(Equal("worker"))
			})

			It("should use SSHAddress when specified", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{
								IPAddress:  "10.0.1.10",
								SSHAddress: "ssh.example.com",
							},
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())

				Expect(k0sctlConfig.Spec.Hosts[0].SSH.Address).To(Equal("ssh.example.com"))
				Expect(k0sctlConfig.Spec.Hosts[0].PrivateAddress).To(Equal("10.0.1.10"))
			})

			It("should default SSHAddress to IPAddress when not specified", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.1.10"},
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())

				Expect(k0sctlConfig.Spec.Hosts[0].SSH.Address).To(Equal("10.0.1.10"))
			})

			It("should use SSHPort when specified", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{
								IPAddress: "10.0.1.10",
								SSHPort:   2222,
							},
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())

				Expect(k0sctlConfig.Spec.Hosts[0].SSH.Port).To(Equal(2222))
			})

			It("should default SSHPort to 22 when not specified", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.1.10"},
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())

				Expect(k0sctlConfig.Spec.Hosts[0].SSH.Port).To(Equal(22))
			})

			It("should skip duplicate IPs between control planes and workers", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.1.10"},
						},
						Workers: []files.K8sNode{
							{IPAddress: "10.0.1.10"}, // Duplicate
							{IPAddress: "10.0.2.10"}, // Unique
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())

				// Should only have 2 hosts: 1 control plane + 1 unique worker
				Expect(k0sctlConfig.Spec.Hosts).To(HaveLen(2))
				Expect(k0sctlConfig.Spec.Hosts[0].SSH.Address).To(Equal("10.0.1.10"))
				Expect(k0sctlConfig.Spec.Hosts[0].Role).To(Equal("controller+worker"))
				Expect(k0sctlConfig.Spec.Hosts[1].SSH.Address).To(Equal("10.0.2.10"))
				Expect(k0sctlConfig.Spec.Hosts[1].Role).To(Equal("worker"))
			})

			It("should enable UploadBinary when k0sBinaryPath is provided", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.1.10"},
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "/path/to/k0s")
				Expect(err).ToNot(HaveOccurred())

				Expect(k0sctlConfig.Spec.Hosts[0].UploadBinary).To(BeTrue())
				Expect(k0sctlConfig.Spec.Hosts[0].K0sBinaryPath).To(Equal("/path/to/k0s"))
			})

			It("should not enable UploadBinary when k0sBinaryPath is empty", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.1.10"},
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())

				Expect(k0sctlConfig.Spec.Hosts[0].UploadBinary).To(BeFalse())
				Expect(k0sctlConfig.Spec.Hosts[0].K0sBinaryPath).To(BeEmpty())
			})

			It("should set SSH key path correctly", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.1.10"},
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/home/user/.ssh/id_rsa", "")
				Expect(err).ToNot(HaveOccurred())

				Expect(k0sctlConfig.Spec.Hosts[0].SSH.KeyPath).To(Equal("/home/user/.ssh/id_rsa"))
			})

			It("should set SSH user to root", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.1.10"},
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())

				Expect(k0sctlConfig.Spec.Hosts[0].SSH.User).To(Equal("root"))
			})

			It("should set KUBELET_EXTRA_ARGS environment variable", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.1.10"},
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())

				Expect(k0sctlConfig.Spec.Hosts[0].Environment).To(HaveKeyWithValue("KUBELET_EXTRA_ARGS", "--node-ip=10.0.1.10"))
			})

			It("should generate valid YAML", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.1.10"},
						},
						PodCIDR:     "10.244.0.0/16",
						ServiceCIDR: "10.96.0.0/12",
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())

				yamlData, err := k0sctlConfig.Marshal()
				Expect(err).ToNot(HaveOccurred())
				Expect(yamlData).ToNot(BeEmpty())

				// Verify it can be unmarshalled back
				var parsedConfig installer.K0sctlConfig
				err = yaml.Unmarshal(yamlData, &parsedConfig)
				Expect(err).ToNot(HaveOccurred())
				Expect(parsedConfig.Metadata.Name).To(Equal("codesphere-test-dc"))
			})
		})

		Context("with invalid input", func() {
			It("should return error for nil install-config", func() {
				k0sctlConfig, err := installer.GenerateK0sctlConfig(nil, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("installConfig cannot be nil"))
				Expect(k0sctlConfig).To(BeNil())
			})

			It("should return error for non-managed Kubernetes", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: false,
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("k0sctl is only supported for Codesphere-managed Kubernetes"))
				Expect(k0sctlConfig).To(BeNil())
			})
		})

		Context("edge cases", func() {
			It("should handle empty control plane list", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes:       []files.K8sNode{},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())
				Expect(k0sctlConfig.Spec.Hosts).To(BeEmpty())
			})

			It("should handle nil control plane list", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes:       nil,
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())
				Expect(k0sctlConfig.Spec.Hosts).To(BeEmpty())
			})

			It("should handle only worker nodes (no control planes)", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes:       []files.K8sNode{},
						Workers: []files.K8sNode{
							{IPAddress: "10.0.2.10"},
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())
				// Workers should still be added even without control planes
				Expect(k0sctlConfig.Spec.Hosts).To(HaveLen(1))
				Expect(k0sctlConfig.Spec.Hosts[0].Role).To(Equal("worker"))
			})

			It("should handle empty SSH key path", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.1.10"},
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "", "")
				Expect(err).ToNot(HaveOccurred())

				Expect(k0sctlConfig.Spec.Hosts[0].SSH.KeyPath).To(BeEmpty())
			})

			It("should set PrivateAddress for all host types", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.1.10", SSHAddress: "public1.example.com"},
						},
						Workers: []files.K8sNode{
							{IPAddress: "10.0.2.10", SSHAddress: "public2.example.com"},
						},
					},
				}

				k0sctlConfig, err := installer.GenerateK0sctlConfig(installConfig, "v1.30.0+k0s.0", "/path/to/key", "")
				Expect(err).ToNot(HaveOccurred())

				// Both hosts should have PrivateAddress set to the internal IP
				Expect(k0sctlConfig.Spec.Hosts[0].PrivateAddress).To(Equal("10.0.1.10"))
				Expect(k0sctlConfig.Spec.Hosts[1].PrivateAddress).To(Equal("10.0.2.10"))
			})
		})
	})
})
