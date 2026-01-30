// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"gopkg.in/yaml.v3"
)

type K0sConfig struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Metadata   K0sMetadata `yaml:"metadata"`
	Spec       K0sSpec     `yaml:"spec"`
}

type K0sMetadata struct {
	Name string `yaml:"name"`
}

type K0sSpec struct {
	API          *K0sAPI          `yaml:"api,omitempty"`
	Network      *K0sNetwork      `yaml:"network,omitempty"`
	Storage      *K0sStorage      `yaml:"storage,omitempty"`
	Images       *K0sImages       `yaml:"images,omitempty"`
	Telemetry    *K0sTelemetry    `yaml:"telemetry,omitempty"`
	Konnectivity *K0sKonnectivity `yaml:"konnectivity,omitempty"`
}

type K0sAPI struct {
	Address         string   `yaml:"address,omitempty"`
	ExternalAddress string   `yaml:"externalAddress,omitempty"`
	SANs            []string `yaml:"sans,omitempty"`
	Port            int      `yaml:"port,omitempty"`
}

type K0sNetwork struct {
	PodCIDR     string `yaml:"podCIDR,omitempty"`
	ServiceCIDR string `yaml:"serviceCIDR,omitempty"`
	Provider    string `yaml:"provider,omitempty"`
}

type K0sStorage struct {
	Type string   `yaml:"type,omitempty"`
	Etcd *K0sEtcd `yaml:"etcd,omitempty"`
}

type K0sEtcd struct {
	PeerAddress string `yaml:"peerAddress,omitempty"`
}

type K0sImages struct {
	DefaultPullPolicy string `yaml:"default_pull_policy,omitempty"`
}

type K0sTelemetry struct {
	Enabled bool `yaml:"enabled"`
}

type K0sKonnectivity struct {
	AdminPort int `yaml:"adminPort,omitempty"`
	AgentPort int `yaml:"agentPort,omitempty"`
}

func GenerateK0sConfig(installConfig *files.RootConfig) (*K0sConfig, error) {
	if installConfig == nil {
		return nil, fmt.Errorf("installConfig cannot be nil")
	}

	k0sConfig := &K0sConfig{
		APIVersion: "k0s.k0sproject.io/v1beta1",
		Kind:       "ClusterConfig",
		Metadata: K0sMetadata{
			Name: fmt.Sprintf("codesphere-%s", installConfig.Datacenter.Name),
		},
		Spec: K0sSpec{},
	}

	if installConfig.Kubernetes.ManagedByCodesphere {
		if len(installConfig.Kubernetes.ControlPlanes) > 0 {
			firstControlPlane := installConfig.Kubernetes.ControlPlanes[0]
			k0sConfig.Spec.API = &K0sAPI{
				Address: firstControlPlane.IPAddress,
				Port:    6443,
			}

			if installConfig.Kubernetes.APIServerHost != "" {
				k0sConfig.Spec.API.ExternalAddress = installConfig.Kubernetes.APIServerHost
			}

			sans := make([]string, 0, len(installConfig.Kubernetes.ControlPlanes))
			for _, cp := range installConfig.Kubernetes.ControlPlanes {
				sans = append(sans, cp.IPAddress)
			}
			if installConfig.Kubernetes.APIServerHost != "" {
				sans = append(sans, installConfig.Kubernetes.APIServerHost)
			}
			k0sConfig.Spec.API.SANs = sans
		}

		k0sConfig.Spec.Network = &K0sNetwork{
			Provider: "calico",
		}

		if installConfig.Kubernetes.PodCIDR != "" {
			k0sConfig.Spec.Network.PodCIDR = installConfig.Kubernetes.PodCIDR
		} else {
			k0sConfig.Spec.Network.PodCIDR = "100.96.0.0/11"
		}
		if installConfig.Kubernetes.ServiceCIDR != "" {
			k0sConfig.Spec.Network.ServiceCIDR = installConfig.Kubernetes.ServiceCIDR
		} else {
			k0sConfig.Spec.Network.ServiceCIDR = "100.64.0.0/13"
		}

		k0sConfig.Spec.Images = &K0sImages{
			DefaultPullPolicy: "Never",
		}

		k0sConfig.Spec.Telemetry = &K0sTelemetry{
			Enabled: false,
		}

		k0sConfig.Spec.Konnectivity = &K0sKonnectivity{
			AdminPort: 8133,
			AgentPort: 8132,
		}

		if len(installConfig.Kubernetes.ControlPlanes) > 0 {
			k0sConfig.Spec.Storage = &K0sStorage{
				Type: "etcd",
				Etcd: &K0sEtcd{
					PeerAddress: installConfig.Kubernetes.ControlPlanes[0].IPAddress,
				},
			}
		}
	}

	return k0sConfig, nil
}

func (c *K0sConfig) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}

func (c *K0sConfig) Unmarshal(data []byte) error {
	return yaml.Unmarshal(data, c)
}
