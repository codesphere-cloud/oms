// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build integration
// +build integration

package cmd_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("K0s Install-Config Integration", func() {
	var (
		tempDir      string
		configPath   string
		k0sConfigOut string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "k0s-integration-test-*")
		Expect(err).NotTo(HaveOccurred())

		configPath = filepath.Join(tempDir, "install-config.yaml")
		k0sConfigOut = filepath.Join(tempDir, "k0s-config.yaml")
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	Describe("Config Generation Workflow", func() {
		It("should generate valid k0s config from install-config", func() {
			// Create a minimal install-config using RootConfig
			installConfig := &files.RootConfig{
				Datacenter: files.DatacenterConfig{
					ID:          1,
					Name:        "test-dc",
					City:        "Test City",
					CountryCode: "US",
				},
				Kubernetes: files.KubernetesConfig{
					ManagedByCodesphere: true,
					ControlPlanes: []files.K8sNode{
						{
							IPAddress: "192.168.1.100",
						},
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

			// Write install-config to file
			configData, err := yaml.Marshal(installConfig)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(configPath, configData, 0644)
			Expect(err).NotTo(HaveOccurred())

			// Load the config back using InstallConfigManager
			icg := installer.NewInstallConfigManager()
			err = icg.LoadInstallConfigFromFile(configPath)
			Expect(err).NotTo(HaveOccurred())

			loadedConfig := icg.GetInstallConfig()
			Expect(loadedConfig).NotTo(BeNil())
			Expect(loadedConfig.Kubernetes.ManagedByCodesphere).To(BeTrue())

			// Generate k0s config
			k0sConfig, err := installer.GenerateK0sConfig(loadedConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(k0sConfig).NotTo(BeNil())

			// Verify k0s config structure
			Expect(k0sConfig.APIVersion).To(Equal("k0s.k0sproject.io/v1beta1"))
			Expect(k0sConfig.Kind).To(Equal("ClusterConfig"))
			Expect(k0sConfig.Metadata.Name).To(Equal("codesphere-test-dc"))
			Expect(k0sConfig.Spec.API).NotTo(BeNil())
			Expect(k0sConfig.Spec.API.Address).To(Equal("192.168.1.100"))
			Expect(k0sConfig.Spec.API.ExternalAddress).To(Equal("api.test.example.com"))

			// Write k0s config to file
			k0sData, err := k0sConfig.Marshal()
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(k0sConfigOut, k0sData, 0644)
			Expect(err).NotTo(HaveOccurred())

			// Verify file was created and is valid YAML
			Expect(k0sConfigOut).To(BeAnExistingFile())
			data, err := os.ReadFile(k0sConfigOut)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(data)).To(BeNumerically(">", 0))

			// Verify we can unmarshal it back
			var verifyConfig installer.K0sConfig
			err = yaml.Unmarshal(data, &verifyConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(verifyConfig.APIVersion).To(Equal("k0s.k0sproject.io/v1beta1"))
		})

		It("should handle multi-control-plane configuration", func() {
			installConfig := &files.RootConfig{
				Datacenter: files.DatacenterConfig{
					ID:   1,
					Name: "multi-dc",
				},
				Kubernetes: files.KubernetesConfig{
					ManagedByCodesphere: true,
					ControlPlanes: []files.K8sNode{
						{IPAddress: "10.0.0.10"},
						{IPAddress: "10.0.0.11"},
						{IPAddress: "10.0.0.12"},
					},
					APIServerHost: "api.cluster.test",
				},
				Codesphere: files.CodesphereConfig{
					Domain:   "cluster.test",
					PublicIP: "10.0.0.10",
					DeployConfig: files.DeployConfig{
						Images: map[string]files.ImageConfig{},
					},
					Plans: files.PlansConfig{
						HostingPlans:   map[int]files.HostingPlan{},
						WorkspacePlans: map[int]files.WorkspacePlan{},
					},
				},
			}

			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			Expect(err).NotTo(HaveOccurred())

			// Verify primary IP is used
			Expect(k0sConfig.Spec.API.Address).To(Equal("10.0.0.10"))
			// Verify all IPs are in SANs
			Expect(k0sConfig.Spec.API.SANs).To(ContainElement("10.0.0.10"))
			Expect(k0sConfig.Spec.API.SANs).To(ContainElement("10.0.0.11"))
			Expect(k0sConfig.Spec.API.SANs).To(ContainElement("10.0.0.12"))
			Expect(k0sConfig.Spec.API.SANs).To(ContainElement("api.cluster.test"))
		})

		It("should preserve network configuration", func() {
			installConfig := &files.RootConfig{
				Datacenter: files.DatacenterConfig{
					ID:   1,
					Name: "network-test",
				},
				Kubernetes: files.KubernetesConfig{
					ManagedByCodesphere: true,
					ControlPlanes: []files.K8sNode{
						{IPAddress: "192.168.1.100"},
					},
					PodCIDR:     "10.244.0.0/16",
					ServiceCIDR: "10.96.0.0/12",
				},
				Codesphere: files.CodesphereConfig{
					Domain:   "network.test",
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

			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			Expect(err).NotTo(HaveOccurred())

			// Verify network settings
			Expect(k0sConfig.Spec.Network).NotTo(BeNil())
			Expect(k0sConfig.Spec.Network.PodCIDR).To(Equal("10.244.0.0/16"))
			Expect(k0sConfig.Spec.Network.ServiceCIDR).To(Equal("10.96.0.0/12"))
		})

		It("should handle storage configuration", func() {
			installConfig := &files.RootConfig{
				Datacenter: files.DatacenterConfig{
					ID:   1,
					Name: "storage-test",
				},
				Kubernetes: files.KubernetesConfig{
					ManagedByCodesphere: true,
					ControlPlanes: []files.K8sNode{
						{IPAddress: "192.168.1.100"},
					},
				},
				Codesphere: files.CodesphereConfig{
					Domain:   "storage.test",
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

			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			Expect(err).NotTo(HaveOccurred())

			// Verify storage/etcd settings
			Expect(k0sConfig.Spec.Storage).NotTo(BeNil())
			Expect(k0sConfig.Spec.Storage.Type).To(Equal("etcd"))
			Expect(k0sConfig.Spec.Storage.Etcd).NotTo(BeNil())
			Expect(k0sConfig.Spec.Storage.Etcd.PeerAddress).To(Equal("192.168.1.100"))
		})
	})

	Describe("Error Handling", func() {
		It("should fail gracefully on missing install-config file", func() {
			nonExistentPath := filepath.Join(tempDir, "does-not-exist.yaml")
			icg := installer.NewInstallConfigManager()
			err := icg.LoadInstallConfigFromFile(nonExistentPath)
			Expect(err).To(HaveOccurred())
		})

		It("should handle nil config gracefully", func() {
			_, err := installer.GenerateK0sConfig(nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot be nil"))
		})

		It("should handle invalid YAML gracefully", func() {
			invalidYAML := []byte("invalid: [unclosed bracket")
			err := os.WriteFile(configPath, invalidYAML, 0644)
			Expect(err).NotTo(HaveOccurred())

			icg := installer.NewInstallConfigManager()
			err = icg.LoadInstallConfigFromFile(configPath)
			Expect(err).To(HaveOccurred())
		})

		It("should fail for external Kubernetes", func() {
			installConfig := &files.RootConfig{
				Datacenter: files.DatacenterConfig{
					ID:   1,
					Name: "external-k8s",
				},
				Kubernetes: files.KubernetesConfig{
					ManagedByCodesphere: false, // External K8s
				},
				Codesphere: files.CodesphereConfig{
					Domain:   "external.test",
					PublicIP: "10.0.0.1",
					DeployConfig: files.DeployConfig{
						Images: map[string]files.ImageConfig{},
					},
					Plans: files.PlansConfig{
						HostingPlans:   map[int]files.HostingPlan{},
						WorkspacePlans: map[int]files.WorkspacePlan{},
					},
				},
			}

			// GenerateK0sConfig should still work (doesn't validate ManagedByCodesphere)
			// The validation happens in the CLI command
			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(k0sConfig).NotTo(BeNil())
		})
	})

	Describe("YAML Marshalling", func() {
		It("should produce valid k0s YAML output", func() {
			installConfig := &files.RootConfig{
				Datacenter: files.DatacenterConfig{
					ID:   1,
					Name: "yaml-test",
				},
				Kubernetes: files.KubernetesConfig{
					ManagedByCodesphere: true,
					ControlPlanes: []files.K8sNode{
						{IPAddress: "10.20.30.40"},
					},
				},
				Codesphere: files.CodesphereConfig{
					Domain:   "yaml.test",
					PublicIP: "10.20.30.40",
					DeployConfig: files.DeployConfig{
						Images: map[string]files.ImageConfig{},
					},
					Plans: files.PlansConfig{
						HostingPlans:   map[int]files.HostingPlan{},
						WorkspacePlans: map[int]files.WorkspacePlan{},
					},
				},
			}

			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			Expect(err).NotTo(HaveOccurred())

			yamlData, err := k0sConfig.Marshal()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(yamlData)).To(ContainSubstring("k0s.k0sproject.io/v1beta1"))
			Expect(string(yamlData)).To(ContainSubstring("ClusterConfig"))
			Expect(string(yamlData)).To(ContainSubstring("10.20.30.40"))
		})

		It("should round-trip marshal and unmarshal correctly", func() {
			installConfig := &files.RootConfig{
				Datacenter: files.DatacenterConfig{
					ID:   1,
					Name: "roundtrip-test",
				},
				Kubernetes: files.KubernetesConfig{
					ManagedByCodesphere: true,
					ControlPlanes: []files.K8sNode{
						{IPAddress: "172.16.0.1"},
					},
					PodCIDR:     "10.244.0.0/16",
					ServiceCIDR: "10.96.0.0/12",
				},
				Codesphere: files.CodesphereConfig{
					Domain:   "roundtrip.test",
					PublicIP: "172.16.0.1",
					DeployConfig: files.DeployConfig{
						Images: map[string]files.ImageConfig{},
					},
					Plans: files.PlansConfig{
						HostingPlans:   map[int]files.HostingPlan{},
						WorkspacePlans: map[int]files.WorkspacePlan{},
					},
				},
			}

			original, err := installer.GenerateK0sConfig(installConfig)
			Expect(err).NotTo(HaveOccurred())

			// Marshal to YAML
			yamlData, err := original.Marshal()
			Expect(err).NotTo(HaveOccurred())

			// Unmarshal back
			var restored installer.K0sConfig
			err = yaml.Unmarshal(yamlData, &restored)
			Expect(err).NotTo(HaveOccurred())

			// Verify they match
			Expect(restored.APIVersion).To(Equal(original.APIVersion))
			Expect(restored.Kind).To(Equal(original.Kind))
			Expect(restored.Metadata.Name).To(Equal(original.Metadata.Name))
			Expect(restored.Spec.API.Address).To(Equal(original.Spec.API.Address))
			Expect(restored.Spec.Network.PodCIDR).To(Equal(original.Spec.Network.PodCIDR))
			Expect(restored.Spec.Network.ServiceCIDR).To(Equal(original.Spec.Network.ServiceCIDR))
		})
	})

	Describe("Full Workflow Integration", func() {
		It("should complete full config generation workflow", func() {
			// Step 1: Create install-config
			installConfig := &files.RootConfig{
				Datacenter: files.DatacenterConfig{
					ID:          1,
					Name:        "integration-dc",
					City:        "Integration City",
					CountryCode: "US",
				},
				Kubernetes: files.KubernetesConfig{
					ManagedByCodesphere: true,
					ControlPlanes: []files.K8sNode{
						{IPAddress: "203.0.113.10"},
					},
					APIServerHost: "api.integration.test",
					PodCIDR:       "10.244.0.0/16",
					ServiceCIDR:   "10.96.0.0/12",
				},
				Codesphere: files.CodesphereConfig{
					Domain:   "integration.test",
					PublicIP: "203.0.113.10",
					DeployConfig: files.DeployConfig{
						Images: map[string]files.ImageConfig{},
					},
					Plans: files.PlansConfig{
						HostingPlans:   map[int]files.HostingPlan{},
						WorkspacePlans: map[int]files.WorkspacePlan{},
					},
				},
			}

			// Step 2: Write install-config
			configData, err := yaml.Marshal(installConfig)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(configPath, configData, 0644)
			Expect(err).NotTo(HaveOccurred())

			// Step 3: Load install-config
			icg := installer.NewInstallConfigManager()
			err = icg.LoadInstallConfigFromFile(configPath)
			Expect(err).NotTo(HaveOccurred())

			// Step 4: Generate k0s config
			loadedConfig := icg.GetInstallConfig()
			k0sConfig, err := installer.GenerateK0sConfig(loadedConfig)
			Expect(err).NotTo(HaveOccurred())

			// Step 5: Marshal k0s config
			k0sData, err := k0sConfig.Marshal()
			Expect(err).NotTo(HaveOccurred())

			// Step 6: Write k0s config
			err = os.WriteFile(k0sConfigOut, k0sData, 0644)
			Expect(err).NotTo(HaveOccurred())

			// Step 7: Verify complete workflow
			Expect(k0sConfigOut).To(BeAnExistingFile())

			// Step 8: Load and verify k0s config
			readData, err := os.ReadFile(k0sConfigOut)
			Expect(err).NotTo(HaveOccurred())

			var finalK0sConfig installer.K0sConfig
			err = yaml.Unmarshal(readData, &finalK0sConfig)
			Expect(err).NotTo(HaveOccurred())

			// Step 9: Validate all fields
			Expect(finalK0sConfig.APIVersion).To(Equal("k0s.k0sproject.io/v1beta1"))
			Expect(finalK0sConfig.Kind).To(Equal("ClusterConfig"))
			Expect(finalK0sConfig.Metadata.Name).To(Equal("codesphere-integration-dc"))
			Expect(finalK0sConfig.Spec.API.Address).To(Equal("203.0.113.10"))
			Expect(finalK0sConfig.Spec.API.ExternalAddress).To(Equal("api.integration.test"))
			Expect(finalK0sConfig.Spec.Network.PodCIDR).To(Equal("10.244.0.0/16"))
			Expect(finalK0sConfig.Spec.Network.ServiceCIDR).To(Equal("10.96.0.0/12"))
			Expect(finalK0sConfig.Spec.Storage.Type).To(Equal("etcd"))
			Expect(finalK0sConfig.Spec.Storage.Etcd.PeerAddress).To(Equal("203.0.113.10"))
		})
	})
})
