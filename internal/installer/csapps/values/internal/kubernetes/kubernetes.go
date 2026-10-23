// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package kubernetes builds the values of the kubernetes key.
package kubernetes

import (
	"cmp"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// WorkspaceNamespace is the namespace workspaces run in.
const WorkspaceNamespace = "workspaces"

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
func Values(kubernetes files.KubernetesConfig) Kubernetes {
	podCIDR, serviceCIDR := kubernetes.PodCIDR, kubernetes.ServiceCIDR
	if kubernetes.ManagedByCodesphere {
		podCIDR = cmp.Or(podCIDR, files.DefaultKubernetesPodCIDR)
		serviceCIDR = cmp.Or(serviceCIDR, files.DefaultKubernetesServiceCIDR)
	}

	return Kubernetes{NetworkPolicies: NetworkPolicies{Workspace: WorkspaceNetworkPolicy{
		Namespace:     WorkspaceNamespace,
		PodSubnet:     podCIDR,
		ServiceSubnet: serviceCIDR,
	}}}
}
