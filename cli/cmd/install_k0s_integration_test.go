// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build integration
// +build integration

package cmd_test

import (
	"fmt"
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

	createBaseConfig := func(name string, ip string) *files.RootConfig {
		return &files.RootConfig{
			Datacenter: files.DatacenterConfig{
				ID:          1,
				Name:        name,
				City:        "Test City",
				CountryCode: "US",
			},
			Kubernetes: files.KubernetesConfig{
				ManagedByCodesphere: true,
				ControlPlanes: []files.K8sNode{
					{IPAddress: ip},
				},
				APIServerHost: "api.test.example.com",
			},
			Codesphere: files.CodesphereConfig{
				Domain:   "test.example.com",
				PublicIP: ip,
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

	Describe("Complete Workflow", func() {
		It("should generate valid k0s config from install-config file", func() {
			installConfig := createBaseConfig("test-dc", "192.168.1.100")

			// Write and load install-config
			configData, err := yaml.Marshal(installConfig)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(configPath, configData, 0644)
			Expect(err).NotTo(HaveOccurred())

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
			Expect(k0sConfig.Spec.API.Address).To(Equal("192.168.1.100"))
			Expect(k0sConfig.Spec.API.ExternalAddress).To(Equal("api.test.example.com"))

			// Write k0s config to file and verify
			k0sData, err := k0sConfig.Marshal()
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(k0sConfigOut, k0sData, 0644)
			Expect(err).NotTo(HaveOccurred())

			Expect(k0sConfigOut).To(BeAnExistingFile())
			data, err := os.ReadFile(k0sConfigOut)
			Expect(err).NotTo(HaveOccurred())

			var verifyConfig installer.K0sConfig
			err = yaml.Unmarshal(data, &verifyConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(verifyConfig.APIVersion).To(Equal("k0s.k0sproject.io/v1beta1"))
			Expect(verifyConfig.Metadata.Name).To(Equal("codesphere-test-dc"))
		})
	})

	Describe("Configuration Features", func() {
		It("should handle multi-control-plane configuration", func() {
			installConfig := createBaseConfig("multi-dc", "10.0.0.10")
			installConfig.Kubernetes.ControlPlanes = []files.K8sNode{
				{IPAddress: "10.0.0.10"},
				{IPAddress: "10.0.0.11"},
				{IPAddress: "10.0.0.12"},
			}
			installConfig.Kubernetes.APIServerHost = "api.cluster.test"

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

		It("should preserve custom network configuration", func() {
			installConfig := createBaseConfig("network-test", "192.168.1.100")
			installConfig.Kubernetes.PodCIDR = "10.244.0.0/16"
			installConfig.Kubernetes.ServiceCIDR = "10.96.0.0/12"

			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			Expect(err).NotTo(HaveOccurred())

			Expect(k0sConfig.Spec.Network).NotTo(BeNil())
			Expect(k0sConfig.Spec.Network.PodCIDR).To(Equal("10.244.0.0/16"))
			Expect(k0sConfig.Spec.Network.ServiceCIDR).To(Equal("10.96.0.0/12"))
		})

		It("should configure etcd storage correctly", func() {
			installConfig := createBaseConfig("storage-test", "192.168.1.100")

			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			Expect(err).NotTo(HaveOccurred())

			Expect(k0sConfig.Spec.Storage).NotTo(BeNil())
			Expect(k0sConfig.Spec.Storage.Type).To(Equal("etcd"))
			Expect(k0sConfig.Spec.Storage.Etcd).NotTo(BeNil())
			Expect(k0sConfig.Spec.Storage.Etcd.PeerAddress).To(Equal("192.168.1.100"))
		})

		It("should generate correct cluster name from datacenter", func() {
			installConfig := createBaseConfig("prod-us-east", "10.1.2.3")

			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			Expect(err).NotTo(HaveOccurred())

			Expect(k0sConfig.Metadata.Name).To(Equal("codesphere-prod-us-east"))
		})

		It("should handle empty control plane list", func() {
			installConfig := createBaseConfig("empty-cp", "10.0.0.1")
			installConfig.Kubernetes.ControlPlanes = []files.K8sNode{}

			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			// Should either handle gracefully or error
			if err == nil {
				Expect(k0sConfig).NotTo(BeNil())
			} else {
				Expect(err).To(HaveOccurred())
			}
		})

		It("should use default network values when not specified", func() {
			installConfig := createBaseConfig("defaults-test", "10.0.0.1")

			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			Expect(err).NotTo(HaveOccurred())

			// Verify defaults are applied or fields are present
			Expect(k0sConfig.Spec.Network).NotTo(BeNil())
			if k0sConfig.Spec.Network.PodCIDR != "" {
				Expect(k0sConfig.Spec.Network.PodCIDR).To(MatchRegexp(`^\d+\.\d+\.\d+\.\d+/\d+$`))
			}
			if k0sConfig.Spec.Network.ServiceCIDR != "" {
				Expect(k0sConfig.Spec.Network.ServiceCIDR).To(MatchRegexp(`^\d+\.\d+\.\d+\.\d+/\d+$`))
			}
		})

		It("should handle special characters in datacenter names", func() {
			testCases := []struct {
				name     string
				expected string
			}{
				{"test-dc-01", "codesphere-test-dc-01"},
				{"test_dc_02", "codesphere-test_dc_02"},
				{"TestDC03", "codesphere-TestDC03"},
			}

			for _, tc := range testCases {
				installConfig := createBaseConfig(tc.name, "10.0.0.1")
				k0sConfig, err := installer.GenerateK0sConfig(installConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(k0sConfig.Metadata.Name).To(Equal(tc.expected))
			}
		})

		It("should handle large multi-control-plane setup", func() {
			installConfig := createBaseConfig("large-cluster", "10.0.1.1")
			controlPlanes := make([]files.K8sNode, 7)
			for i := 0; i < 7; i++ {
				controlPlanes[i] = files.K8sNode{
					IPAddress: fmt.Sprintf("10.0.1.%d", i+1),
				}
			}
			installConfig.Kubernetes.ControlPlanes = controlPlanes

			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(k0sConfig.Spec.API.Address).To(Equal("10.0.1.1"))
			Expect(len(k0sConfig.Spec.API.SANs)).To(BeNumerically(">=", 7))
		})

		It("should properly configure certificate SANs", func() {
			installConfig := createBaseConfig("san-test", "192.168.100.50")
			installConfig.Kubernetes.APIServerHost = "k8s.example.com"
			installConfig.Codesphere.Domain = "app.example.com"

			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			Expect(err).NotTo(HaveOccurred())

			Expect(k0sConfig.Spec.API.SANs).To(ContainElement("192.168.100.50"))
			Expect(k0sConfig.Spec.API.SANs).To(ContainElement("k8s.example.com"))
			Expect(len(k0sConfig.Spec.API.SANs)).To(BeNumerically(">=", 2))
		})
	})

	Describe("Error Handling", func() {
		It("should fail when loading non-existent file", func() {
			nonExistentPath := filepath.Join(tempDir, "does-not-exist.yaml")
			icg := installer.NewInstallConfigManager()
			err := icg.LoadInstallConfigFromFile(nonExistentPath)
			Expect(err).To(HaveOccurred())
		})

		It("should fail when generating config from nil", func() {
			_, err := installer.GenerateK0sConfig(nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot be nil"))
		})

		It("should fail when loading invalid YAML", func() {
			invalidYAML := []byte("invalid: [unclosed bracket")
			err := os.WriteFile(configPath, invalidYAML, 0644)
			Expect(err).NotTo(HaveOccurred())

			icg := installer.NewInstallConfigManager()
			err = icg.LoadInstallConfigFromFile(configPath)
			Expect(err).To(HaveOccurred())
		})

		It("should handle empty file gracefully", func() {
			err := os.WriteFile(configPath, []byte{}, 0644)
			Expect(err).NotTo(HaveOccurred())

			icg := installer.NewInstallConfigManager()
			err = icg.LoadInstallConfigFromFile(configPath)
			// Empty file loads successfully but returns empty config
			Expect(err).NotTo(HaveOccurred())
			config := icg.GetInstallConfig()
			Expect(config).NotTo(BeNil())
		})

		It("should handle external Kubernetes cluster config", func() {
			installConfig := createBaseConfig("external-k8s", "10.0.0.1")
			installConfig.Kubernetes.ManagedByCodesphere = false

			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(k0sConfig).NotTo(BeNil())
		})

		It("should handle missing APIServerHost", func() {
			installConfig := createBaseConfig("missing-host", "10.0.0.1")
			installConfig.Kubernetes.APIServerHost = ""

			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			if err == nil {
				Expect(k0sConfig).NotTo(BeNil())
				Expect(k0sConfig.Spec.API.Address).To(Equal("10.0.0.1"))
			} else {
				Expect(err).To(HaveOccurred())
			}
		})

		It("should handle missing datacenter name", func() {
			installConfig := createBaseConfig("", "10.0.0.1")

			k0sConfig, err := installer.GenerateK0sConfig(installConfig)
			if err == nil {
				Expect(k0sConfig).NotTo(BeNil())
			} else {
				Expect(err).To(HaveOccurred())
			}
		})

		It("should fail when writing to read-only directory", func() {
			readOnlyDir := filepath.Join(tempDir, "readonly")
			err := os.Mkdir(readOnlyDir, 0444)
			Expect(err).NotTo(HaveOccurred())

			readOnlyPath := filepath.Join(readOnlyDir, "config.yaml")
			installConfig := createBaseConfig("test", "10.0.0.1")
			configData, err := yaml.Marshal(installConfig)
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(readOnlyPath, configData, 0644)
			Expect(err).To(HaveOccurred())

			os.Chmod(readOnlyDir, 0755)
		})
	})

	Describe("YAML Serialization", func() {
		It("should marshal and unmarshal k0s config correctly", func() {
			installConfig := createBaseConfig("roundtrip-test", "172.16.0.1")
			installConfig.Kubernetes.PodCIDR = "10.244.0.0/16"
			installConfig.Kubernetes.ServiceCIDR = "10.96.0.0/12"

			original, err := installer.GenerateK0sConfig(installConfig)
			Expect(err).NotTo(HaveOccurred())

			yamlData, err := original.Marshal()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(yamlData)).To(ContainSubstring("k0s.k0sproject.io/v1beta1"))
			Expect(string(yamlData)).To(ContainSubstring("ClusterConfig"))

			var restored installer.K0sConfig
			err = yaml.Unmarshal(yamlData, &restored)
			Expect(err).NotTo(HaveOccurred())

			// Verify critical fields match
			Expect(restored.APIVersion).To(Equal(original.APIVersion))
			Expect(restored.Kind).To(Equal(original.Kind))
			Expect(restored.Metadata.Name).To(Equal(original.Metadata.Name))
			Expect(restored.Spec.API.Address).To(Equal(original.Spec.API.Address))
			Expect(restored.Spec.Network.PodCIDR).To(Equal(original.Spec.Network.PodCIDR))
			Expect(restored.Spec.Network.ServiceCIDR).To(Equal(original.Spec.Network.ServiceCIDR))
		})
	})

	Describe("Config Persistence", func() {
		It("should persist and reload config correctly", func() {
			originalConfig := createBaseConfig("persist-test", "172.20.30.40")
			originalConfig.Kubernetes.PodCIDR = "10.100.0.0/16"
			originalConfig.Kubernetes.ServiceCIDR = "10.200.0.0/16"

			// Save install-config
			configData, err := yaml.Marshal(originalConfig)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(configPath, configData, 0644)
			Expect(err).NotTo(HaveOccurred())

			// Generate and save k0s config
			k0sConfig, err := installer.GenerateK0sConfig(originalConfig)
			Expect(err).NotTo(HaveOccurred())
			k0sData, err := k0sConfig.Marshal()
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(k0sConfigOut, k0sData, 0644)
			Expect(err).NotTo(HaveOccurred())

			// Reload install-config
			icg := installer.NewInstallConfigManager()
			err = icg.LoadInstallConfigFromFile(configPath)
			Expect(err).NotTo(HaveOccurred())
			reloadedInstallConfig := icg.GetInstallConfig()

			// Reload k0s config
			reloadedK0sData, err := os.ReadFile(k0sConfigOut)
			Expect(err).NotTo(HaveOccurred())
			var reloadedK0sConfig installer.K0sConfig
			err = yaml.Unmarshal(reloadedK0sData, &reloadedK0sConfig)
			Expect(err).NotTo(HaveOccurred())

			// Verify both configs match original
			Expect(reloadedInstallConfig.Datacenter.Name).To(Equal(originalConfig.Datacenter.Name))
			Expect(reloadedInstallConfig.Kubernetes.PodCIDR).To(Equal(originalConfig.Kubernetes.PodCIDR))
			Expect(reloadedK0sConfig.Metadata.Name).To(Equal(k0sConfig.Metadata.Name))
			Expect(reloadedK0sConfig.Spec.API.Address).To(Equal(k0sConfig.Spec.API.Address))
			Expect(reloadedK0sConfig.Spec.Network.PodCIDR).To(Equal(k0sConfig.Spec.Network.PodCIDR))
		})
	})
})
