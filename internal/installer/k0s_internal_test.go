// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

var _ = Describe("K0s Internal Methods", func() {
	var (
		k0s           *K0s
		tempConfigDir string
	)

	BeforeEach(func() {
		k0s = &K0s{}
		var err error
		tempConfigDir, err = os.MkdirTemp("", "k0s-config-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if tempConfigDir != "" {
			_ = os.RemoveAll(tempConfigDir)
		}
	})

	createConfigFile := func(content string) string {
		configPath := filepath.Join(tempConfigDir, "test-config.yaml")
		err := os.WriteFile(configPath, []byte(content), 0644)
		Expect(err).NotTo(HaveOccurred())
		return configPath
	}

	Describe("filterConfigForK0s", func() {
		It("filters out non-k0s top-level fields", func() {
			configContent := `apiVersion: k0s.k0sproject.io/v1beta1
kind: ClusterConfig
metadata:
  name: test-cluster
spec:
  api:
    address: 192.168.1.100
extraField: should-be-removed
anotherExtra: also-removed
`
			configPath := createConfigFile(configContent)

			filteredPath, err := k0s.filterConfigForK0s(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(filteredPath).NotTo(BeEmpty())
			defer func() { _ = os.Remove(filteredPath) }()

			// Read and verify filtered content
			data, err := os.ReadFile(filteredPath)
			Expect(err).NotTo(HaveOccurred())
			content := string(data)

			Expect(content).To(ContainSubstring("apiVersion"))
			Expect(content).To(ContainSubstring("kind"))
			Expect(content).To(ContainSubstring("metadata"))
			Expect(content).To(ContainSubstring("spec"))
			Expect(content).NotTo(ContainSubstring("extraField"))
			Expect(content).NotTo(ContainSubstring("anotherExtra"))
		})

		It("preserves all expected k0s fields at top level", func() {
			configContent := `apiVersion: k0s.k0sproject.io/v1beta1
kind: ClusterConfig
metadata:
  name: test-cluster
spec:
  api:
    address: 192.168.1.100
`
			configPath := createConfigFile(configContent)

			filteredPath, err := k0s.filterConfigForK0s(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(filteredPath).NotTo(BeEmpty())
			defer func() { _ = os.Remove(filteredPath) }()

			data, err := os.ReadFile(filteredPath)
			Expect(err).NotTo(HaveOccurred())
			content := string(data)

			Expect(content).To(ContainSubstring("apiVersion: k0s.k0sproject.io/v1beta1"))
			Expect(content).To(ContainSubstring("kind: ClusterConfig"))
			Expect(content).To(ContainSubstring("metadata"))
			Expect(content).To(ContainSubstring("spec"))
		})

		It("filters out non-k0s spec fields", func() {
			configContent := `apiVersion: k0s.k0sproject.io/v1beta1
kind: ClusterConfig
spec:
  api:
    address: 192.168.1.100
  network:
    provider: calico
  customField: should-be-removed
  anotherCustom: also-removed
`
			configPath := createConfigFile(configContent)

			filteredPath, err := k0s.filterConfigForK0s(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(filteredPath).NotTo(BeEmpty())
			defer func() { _ = os.Remove(filteredPath) }()

			data, err := os.ReadFile(filteredPath)
			Expect(err).NotTo(HaveOccurred())
			content := string(data)

			Expect(content).To(ContainSubstring("api"))
			Expect(content).To(ContainSubstring("network"))
			Expect(content).NotTo(ContainSubstring("customField"))
			Expect(content).NotTo(ContainSubstring("anotherCustom"))
		})

		It("preserves all expected k0s spec fields", func() {
			configContent := `apiVersion: k0s.k0sproject.io/v1beta1
kind: ClusterConfig
spec:
  api:
    address: 192.168.1.100
  controllerManager:
    extraArgs:
      - --cluster-cidr=10.244.0.0/16
  scheduler:
    extraArgs:
      - --bind-address=0.0.0.0
  extensions:
    helm:
      repositories:
        - name: stable
  network:
    provider: calico
  storage:
    type: etcd
  telemetry:
    enabled: false
  images:
    default_pull_policy: IfNotPresent
  konnectivity:
    enabled: true
`
			configPath := createConfigFile(configContent)

			filteredPath, err := k0s.filterConfigForK0s(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(filteredPath).NotTo(BeEmpty())
			defer func() { _ = os.Remove(filteredPath) }()

			data, err := os.ReadFile(filteredPath)
			Expect(err).NotTo(HaveOccurred())
			content := string(data)

			// Verify all expected spec fields are preserved
			Expect(content).To(ContainSubstring("api"))
			Expect(content).To(ContainSubstring("controllerManager"))
			Expect(content).To(ContainSubstring("scheduler"))
			Expect(content).To(ContainSubstring("extensions"))
			Expect(content).To(ContainSubstring("network"))
			Expect(content).To(ContainSubstring("storage"))
			Expect(content).To(ContainSubstring("telemetry"))
			Expect(content).To(ContainSubstring("images"))
			Expect(content).To(ContainSubstring("konnectivity"))
		})

		It("handles config with only required fields", func() {
			configContent := `apiVersion: k0s.k0sproject.io/v1beta1
kind: ClusterConfig
spec:
  api:
    address: 192.168.1.100
`
			configPath := createConfigFile(configContent)

			filteredPath, err := k0s.filterConfigForK0s(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(filteredPath).NotTo(BeEmpty())
			defer func() { _ = os.Remove(filteredPath) }()

			data, err := os.ReadFile(filteredPath)
			Expect(err).NotTo(HaveOccurred())

			Expect(string(data)).NotTo(BeEmpty())
		})

		It("creates a temporary file with .yaml extension", func() {
			configContent := `apiVersion: k0s.k0sproject.io/v1beta1
kind: ClusterConfig
spec:
  api:
    address: 192.168.1.100
`
			configPath := createConfigFile(configContent)

			filteredPath, err := k0s.filterConfigForK0s(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(filteredPath).NotTo(BeEmpty())
			defer func() { _ = os.Remove(filteredPath) }()

			Expect(filteredPath).To(HaveSuffix(".yaml"))
			Expect(filteredPath).To(ContainSubstring("k0s-config-"))

			// Verify file exists and is readable
			_, err = os.Stat(filteredPath)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails when config file does not exist", func() {
			_, err := k0s.filterConfigForK0s("/nonexistent/config.yaml")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read config"))
		})

		It("fails when config contains invalid YAML", func() {
			configPath := createConfigFile("invalid: yaml: content: [")

			_, err := k0s.filterConfigForK0s(configPath)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse config"))
		})

		It("handles complex nested structures correctly", func() {
			configContent := `apiVersion: k0s.k0sproject.io/v1beta1
kind: ClusterConfig
metadata:
  name: production-cluster
spec:
  api:
    address: 192.168.1.100
    port: 6443
    sans:
      - api.example.com
      - 192.168.1.100
  network:
    provider: calico
    podCIDR: 10.244.0.0/16
    serviceCIDR: 10.96.0.0/12
  extensions:
    helm:
      repositories:
        - name: stable
          url: https://charts.helm.sh/stable
      charts:
        - name: metrics-server
          namespace: kube-system
customTopLevel: should-be-filtered
`
			configPath := createConfigFile(configContent)

			filteredPath, err := k0s.filterConfigForK0s(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(filteredPath).NotTo(BeEmpty())
			defer func() { _ = os.Remove(filteredPath) }()

			data, err := os.ReadFile(filteredPath)
			Expect(err).NotTo(HaveOccurred())
			content := string(data)

			// Verify nested structures are preserved
			Expect(content).To(ContainSubstring("api.example.com"))
			Expect(content).To(ContainSubstring("10.244.0.0/16"))
			Expect(content).To(ContainSubstring("metrics-server"))

			// Verify custom fields are filtered out
			Expect(content).NotTo(ContainSubstring("customTopLevel"))
		})

		It("filters custom fields within spec", func() {
			configContent := `apiVersion: k0s.k0sproject.io/v1beta1
kind: ClusterConfig
spec:
  api:
    address: 192.168.1.100
  network:
    provider: calico
  customInSpec: should-be-filtered
  anotherCustom: also-filtered
`
			configPath := createConfigFile(configContent)

			filteredPath, err := k0s.filterConfigForK0s(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(filteredPath).NotTo(BeEmpty())
			defer func() { _ = os.Remove(filteredPath) }()

			data, err := os.ReadFile(filteredPath)
			Expect(err).NotTo(HaveOccurred())
			content := string(data)

			Expect(content).To(ContainSubstring("api"))
			Expect(content).To(ContainSubstring("network"))
			Expect(content).NotTo(ContainSubstring("customInSpec"))
			Expect(content).NotTo(ContainSubstring("anotherCustom"))
		})

		It("returns valid YAML that can be parsed", func() {
			configContent := `apiVersion: k0s.k0sproject.io/v1beta1
kind: ClusterConfig
spec:
  api:
    address: 192.168.1.100
  network:
    provider: calico
extraField: removed
`
			configPath := createConfigFile(configContent)

			filteredPath, err := k0s.filterConfigForK0s(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(filteredPath).NotTo(BeEmpty())
			defer func() { _ = os.Remove(filteredPath) }()

			// Verify the output is valid YAML by parsing it
			data, err := os.ReadFile(filteredPath)
			Expect(err).NotTo(HaveOccurred())

			var result map[string]interface{}
			err = yaml.Unmarshal(data, &result)
			Expect(err).NotTo(HaveOccurred())

			// Verify expected structure
			Expect(result).To(HaveKey("apiVersion"))
			Expect(result).To(HaveKey("kind"))
			Expect(result).To(HaveKey("spec"))
			Expect(result).NotTo(HaveKey("extraField"))
		})
	})
})
