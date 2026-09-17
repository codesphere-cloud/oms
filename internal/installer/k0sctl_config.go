// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"slices"

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
	Files            []K0sctlFile      `yaml:"files,omitempty"`
	Hooks            *K0sctlHooks      `yaml:"hooks,omitempty"`
}

type K0sctlFile struct {
	Src    string `yaml:"src"`
	DstDir string `yaml:"dstDir"`
	Perm   string `yaml:"perm,omitempty"`
}

type k0sctlHostOptions struct {
	SSHKeyPath    string
	K0sBinaryPath string
	Airgap        AirgapOptions
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
	Version string     `yaml:"version"`
	Config  *K0sConfig `yaml:"config,omitempty"`
}

type K0sctlHooks struct {
	Apply *K0sctlApplyHooks `yaml:"apply,omitempty"`
}

type K0sctlApplyHooks struct {
	Before []string `yaml:"before,omitempty"`
	After  []string `yaml:"after,omitempty"`
}

// addUniqueK0sctlHost appends a host to the cluster config unless its address is
// already present. isWorker marks hosts that run the k0s worker role, either
// dedicated workers or control planes installed with --enable-worker.
func (k *K0sctlSpec) addUniqueK0sctlHost(node files.K8sNode, role string, installFlags []string, isWorker bool, options k0sctlHostOptions) {
	for _, host := range k.Hosts {
		if host.PrivateAddress == node.IPAddress {
			return
		}
	}

	host := K0sctlHost{
		Role: role,
		SSH: K0sctlSSH{
			Address: node.IPAddress,
			User:    "root",
			Port:    22,
			KeyPath: options.SSHKeyPath,
		},
		InstallFlags:   installFlags,
		PrivateAddress: node.IPAddress,
		Environment: map[string]string{
			"KUBELET_EXTRA_ARGS": fmt.Sprintf("--node-ip=%s", node.IPAddress),
		},
	}

	if options.K0sBinaryPath != "" {
		host.UploadBinary = true
		host.K0sBinaryPath = options.K0sBinaryPath
	}

	if isWorker && options.Airgap.Enabled && options.Airgap.BundlePath != "" {
		host.Files = []K0sctlFile{{
			Src:    options.Airgap.BundlePath,
			DstDir: AirgapImagesDir,
			Perm:   "0644",
		}}
	}

	k.Hosts = append(k.Hosts, host)
}

// GenerateK0sctlConfig generates a k0sctl configuration from a Codesphere install-config
func GenerateK0sctlConfig(installConfig *files.RootConfig, k0sVersion string, sshKeyPath string, k0sBinaryPath string, airgap ...AirgapOptions) (*K0sctlConfig, error) {
	if installConfig == nil {
		return nil, fmt.Errorf("installConfig cannot be nil")
	}

	if !installConfig.Kubernetes.ManagedByCodesphere {
		return nil, fmt.Errorf("k0sctl is only supported for Codesphere-managed Kubernetes")
	}

	options := k0sctlHostOptions{
		SSHKeyPath:    sshKeyPath,
		K0sBinaryPath: k0sBinaryPath,
		Airgap:        firstAirgapOption(airgap),
	}

	// Generate k0s config that will be embedded in k0sctl config
	k0sConfig, err := GenerateK0sConfig(installConfig, options.Airgap)
	if err != nil {
		return nil, fmt.Errorf("failed to generate k0s config: %w", err)
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
				Config:  k0sConfig,
			},
		},
	}

	// Add control-plane nodes, enabling their worker role when configured.
	for _, cp := range installConfig.Kubernetes.ControlPlanes {
		var installFlags []string
		// A node may intentionally be listed as both a control plane and a worker.
		isWorker := slices.Contains(installConfig.Kubernetes.Workers, cp)
		if isWorker {
			installFlags = []string{"--enable-worker", "--no-taints=true"}
		}

		k0sctlConfig.Spec.addUniqueK0sctlHost(cp, "controller", installFlags, isWorker, options)
	}

	// Add dedicated worker nodes if present
	for _, worker := range installConfig.Kubernetes.Workers {
		k0sctlConfig.Spec.addUniqueK0sctlHost(worker, "worker", nil, true, options)
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
