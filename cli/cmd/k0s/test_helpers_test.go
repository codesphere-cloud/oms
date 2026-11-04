// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// newTestInstallConfig returns a minimal install-config with a single control plane.
func newTestInstallConfig(managedByCodesphere bool) *files.RootConfig {
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

// writeTestInstallConfig writes config to install-config.yaml in dir and returns its path.
func writeTestInstallConfig(dir string, config *files.RootConfig) string {
	configPath := filepath.Join(dir, "install-config.yaml")
	configData, err := yaml.Marshal(config)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.WriteFile(configPath, configData, 0644)).To(Succeed())

	return configPath
}
