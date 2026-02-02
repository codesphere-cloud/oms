// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"gopkg.in/yaml.v3"
)

// K0sctlConfig represents the k0sctl configuration file structure
type K0sctlConfig struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   K0sctlMeta `yaml:"metadata,omitempty"`
	Spec       K0sctlSpec `yaml:"spec"`
}

type K0sctlMeta struct {
	Name string `yaml:"name"`
}

type K0sctlSpec struct {
	Hosts []K0sctlHost `yaml:"hosts"`
	K0s   K0sctlK0s    `yaml:"k0s"`
}

type K0sctlHost struct {
	Role             string            `yaml:"role"`
	SSH              K0sctlSSH         `yaml:"ssh"`
	InstallFlags     []string          `yaml:"installFlags,omitempty"`
	PrivateInterface string            `yaml:"privateInterface,omitempty"`
	PrivateAddress   string            `yaml:"privateAddress,omitempty"`
	Environment      map[string]string `yaml:"environment,omitempty"`
	UploadBinary     bool              `yaml:"uploadBinary,omitempty"`
	K0sBinaryPath    string            `yaml:"k0sBinaryPath,omitempty"`
	Hooks            *K0sctlHooks      `yaml:"hooks,omitempty"`
}

type K0sctlSSH struct {
	Address string         `yaml:"address"`
	User    string         `yaml:"user"`
	Port    int            `yaml:"port"`
	KeyPath string         `yaml:"keyPath,omitempty"`
	Bastion *K0sctlBastion `yaml:"bastion,omitempty"`
}

type K0sctlBastion struct {
	Address string `yaml:"address"`
	User    string `yaml:"user"`
	Port    int    `yaml:"port"`
	KeyPath string `yaml:"keyPath,omitempty"`
}

type K0sctlK0s struct {
	Version string                 `yaml:"version"`
	Config  map[string]interface{} `yaml:"config,omitempty"`
}

type K0sctlHooks struct {
	Apply *K0sctlApplyHooks `yaml:"apply,omitempty"`
}

type K0sctlApplyHooks struct {
	Before []string `yaml:"before,omitempty"`
	After  []string `yaml:"after,omitempty"`
}

func createK0sctlHost(node files.K8sNode, role string, installFlags []string, sshKeyPath string, k0sBinaryPath string) K0sctlHost {
	sshPort := node.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}

	sshAddress := node.SSHAddress
	if sshAddress == "" {
		sshAddress = node.IPAddress
	}

	host := K0sctlHost{
		Role: role,
		SSH: K0sctlSSH{
			Address: sshAddress,
			User:    "root",
			Port:    sshPort,
			KeyPath: sshKeyPath,
		},
		InstallFlags:   installFlags,
		PrivateAddress: node.IPAddress,
		Environment: map[string]string{
			"KUBELET_EXTRA_ARGS": fmt.Sprintf("--node-ip=%s", node.IPAddress),
		},
	}

	if k0sBinaryPath != "" {
		host.UploadBinary = true
		host.K0sBinaryPath = k0sBinaryPath
	}

	return host
}

// GenerateK0sctlConfig generates a k0sctl configuration from a Codesphere install-config
func GenerateK0sctlConfig(installConfig *files.RootConfig, k0sVersion string, sshKeyPath string, k0sBinaryPath string) (*K0sctlConfig, error) {
	if installConfig == nil {
		return nil, fmt.Errorf("installConfig cannot be nil")
	}

	if !installConfig.Kubernetes.ManagedByCodesphere {
		return nil, fmt.Errorf("k0sctl is only supported for Codesphere-managed Kubernetes")
	}

	// Generate k0s config that will be embedded in k0sctl config
	k0sConfig, err := GenerateK0sConfig(installConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate k0s config: %w", err)
	}

	// Convert K0sConfig struct to map for k0sctl
	k0sConfigYAML, err := k0sConfig.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal k0s config: %w", err)
	}

	var k0sConfigMap map[string]interface{}
	if err := yaml.Unmarshal(k0sConfigYAML, &k0sConfigMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal k0s config to map: %w", err)
	}

	k0sctlConfig := &K0sctlConfig{
		APIVersion: "k0sctl.k0sproject.io/v1beta1",
		Kind:       "Cluster",
		Metadata: K0sctlMeta{
			Name: fmt.Sprintf("codesphere-%s", installConfig.Datacenter.Name),
		},
		Spec: K0sctlSpec{
			Hosts: []K0sctlHost{},
			K0s: K0sctlK0s{
				Version: k0sVersion,
				Config:  k0sConfigMap,
			},
		},
	}

	// Track added IPs to avoid duplicates
	addedIPs := make(map[string]bool)

	// Add controller+worker nodes from control planes
	controllerFlags := []string{"--enable-worker", "--no-taints"}
	for _, cp := range installConfig.Kubernetes.ControlPlanes {
		host := createK0sctlHost(cp, "controller+worker", controllerFlags, sshKeyPath, k0sBinaryPath)
		k0sctlConfig.Spec.Hosts = append(k0sctlConfig.Spec.Hosts, host)
		addedIPs[cp.IPAddress] = true
	}

	// Add dedicated worker nodes if present
	for _, worker := range installConfig.Kubernetes.Workers {
		if addedIPs[worker.IPAddress] {
			continue
		}
		host := createK0sctlHost(worker, "worker", nil, sshKeyPath, k0sBinaryPath)
		k0sctlConfig.Spec.Hosts = append(k0sctlConfig.Spec.Hosts, host)
		addedIPs[worker.IPAddress] = true
	}

	return k0sctlConfig, nil
}

// Marshal serializes the k0sctl config to YAML
func (c *K0sctlConfig) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}

// Unmarshal deserializes YAML to a k0sctl config
func (c *K0sctlConfig) Unmarshal(data []byte) error {
	return yaml.Unmarshal(data, c)
}
