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

var _ = Describe("K0sConfig", func() {
	Describe("GenerateK0sConfig", func() {
		Context("with valid install-config", func() {
			It("should generate k0s config with control plane settings", func() {
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

				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).ToNot(HaveOccurred())
				Expect(k0sConfig).ToNot(BeNil())

				// Check basic structure
				Expect(k0sConfig.APIVersion).To(Equal("k0s.k0sproject.io/v1beta1"))
				Expect(k0sConfig.Kind).To(Equal("ClusterConfig"))
				Expect(k0sConfig.Metadata.Name).To(Equal("codesphere-test-dc"))

				// Check API configuration
				Expect(k0sConfig.Spec.API).ToNot(BeNil())
				Expect(k0sConfig.Spec.API.Address).To(Equal("10.0.1.10"))
				Expect(k0sConfig.Spec.API.ExternalAddress).To(Equal("k8s.example.com"))
				Expect(k0sConfig.Spec.API.Port).To(Equal(6443))
				Expect(k0sConfig.Spec.API.SANs).To(ContainElements("10.0.1.10", "10.0.1.11", "10.0.1.12", "k8s.example.com"))

				// Check Network configuration
				Expect(k0sConfig.Spec.Network).ToNot(BeNil())
				Expect(k0sConfig.Spec.Network.PodCIDR).To(Equal("10.244.0.0/16"))
				Expect(k0sConfig.Spec.Network.ServiceCIDR).To(Equal("10.96.0.0/12"))
				Expect(k0sConfig.Spec.Network.Provider).To(Equal("calico"))
				Expect(k0sConfig.Spec.Storage.Etcd).ToNot(BeNil())
				Expect(k0sConfig.Spec.Storage.Etcd.PeerAddress).To(Equal("10.0.1.10"))
			})

			It("should handle minimal configuration", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "minimal",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "192.168.1.100"},
						},
					},
				}

				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).ToNot(HaveOccurred())
				Expect(k0sConfig).ToNot(BeNil())
				Expect(k0sConfig.Metadata.Name).To(Equal("codesphere-minimal"))
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

				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).ToNot(HaveOccurred())

				yamlData, err := k0sConfig.Marshal()
				Expect(err).ToNot(HaveOccurred())
				Expect(yamlData).ToNot(BeEmpty())

				// Verify it can be unmarshalled back
				var parsedConfig installer.K0sConfig
				err = yaml.Unmarshal(yamlData, &parsedConfig)
				Expect(err).ToNot(HaveOccurred())
				Expect(parsedConfig.Metadata.Name).To(Equal("codesphere-test-dc"))
			})
		})

		Context("with invalid input", func() {
			It("should return error for nil install-config", func() {
				k0sConfig, err := installer.GenerateK0sConfig(nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("installConfig cannot be nil"))
				Expect(k0sConfig).To(BeNil())
			})
		})

		Context("with non-managed Kubernetes", func() {
			It("should not configure k0s for external kubernetes", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "external",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: false,
						PodCIDR:             "10.244.0.0/16",
						ServiceCIDR:         "10.96.0.0/12",
					},
				}

				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).ToNot(HaveOccurred())
				Expect(k0sConfig).ToNot(BeNil())
				// Should still have basic structure but no specific config
				Expect(k0sConfig.Metadata.Name).To(Equal("codesphere-external"))
			})
		})

		Context("edge cases and validation", func() {
			It("should handle empty datacenter name", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.0.1"},
						},
					},
				}

				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(k0sConfig.Metadata.Name).To(Equal("codesphere-"))
			})

			It("should handle empty control plane list", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes:       []files.K8sNode{},
					},
				}

				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).NotTo(HaveOccurred())
				// Should have basic structure but no API/Storage config
				Expect(k0sConfig.Spec.API).To(BeNil())
				Expect(k0sConfig.Spec.Storage).To(BeNil())
			})

			It("should handle nil control plane addresses", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes:       nil,
					},
				}

				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(k0sConfig.Spec.API).To(BeNil())
			})

			It("should handle missing APIServerHost", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.0.1"},
						},
						APIServerHost: "",
					},
				}

				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(k0sConfig.Spec.API.ExternalAddress).To(BeEmpty())
				Expect(k0sConfig.Spec.API.SANs).To(ConsistOf("10.0.0.1"))
			})

			It("should handle missing network CIDRs", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.0.1"},
						},
						PodCIDR:     "",
						ServiceCIDR: "",
					},
				}

				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(k0sConfig.Spec.Network).NotTo(BeNil())
				Expect(k0sConfig.Spec.Network.Provider).To(Equal("calico"))
			})

			It("should use default network provider", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.0.1"},
						},
					},
				}

				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(k0sConfig.Spec.Network.Provider).To(Equal("calico"))
			})

			It("should generate correct SANs with single control plane", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.0.1"},
						},
						APIServerHost: "api.example.com",
					},
				}

				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(k0sConfig.Spec.API.SANs).To(HaveLen(2))
				Expect(k0sConfig.Spec.API.SANs).To(ContainElements("10.0.0.1", "api.example.com"))
			})

			It("should handle special characters in datacenter name", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test-dc_01.prod",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.0.1"},
						},
					},
				}

				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(k0sConfig.Metadata.Name).To(Equal("codesphere-test-dc_01.prod"))
			})

			It("should set correct API port", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.0.1"},
						},
					},
				}

				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(k0sConfig.Spec.API.Port).To(Equal(6443))
			})

			It("should configure etcd with first control plane IP", func() {
				installConfig := &files.RootConfig{
					Datacenter: files.DatacenterConfig{
						ID:   1,
						Name: "test",
					},
					Kubernetes: files.KubernetesConfig{
						ManagedByCodesphere: true,
						ControlPlanes: []files.K8sNode{
							{IPAddress: "10.0.0.1"},
							{IPAddress: "10.0.0.2"},
						},
					},
				}

				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(k0sConfig.Spec.Storage.Type).To(Equal("etcd"))
				Expect(k0sConfig.Spec.Storage.Etcd.PeerAddress).To(Equal("10.0.0.1"))
			})
		})
	})
})
