// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package kubernetes builds the workspace network policies from the cluster CIDRs.
package kubernetes

import "github.com/codesphere-cloud/oms/internal/installer/files"

// Kubernetes are the values that depend on the cluster's network.
type Kubernetes struct {
	NetworkPolicies NetworkPolicies `yaml:"networkPolicies"`
}

// NetworkPolicies are the network policies of the workspaces.
type NetworkPolicies struct {
	Workspace WorkspaceNetworkPolicy `yaml:"workspace"`
}

// WorkspaceNetworkPolicy is the workspace namespace and the cluster subnets its policies allow.
type WorkspaceNetworkPolicy struct {
	Namespace     string `yaml:"namespace"`
	PodSubnet     string `yaml:"podSubnet"`
	ServiceSubnet string `yaml:"serviceSubnet"`
}

// Values builds the network policies of the workspaces from the cluster CIDRs.
func Values(cfg files.KubernetesConfig) Kubernetes {
	podCIDR, serviceCIDR := cfg.CIDRs()

	return Kubernetes{NetworkPolicies: NetworkPolicies{Workspace: WorkspaceNetworkPolicy{
		Namespace:     files.WorkspacesNamespace,
		PodSubnet:     podCIDR,
		ServiceSubnet: serviceCIDR,
	}}}
}
