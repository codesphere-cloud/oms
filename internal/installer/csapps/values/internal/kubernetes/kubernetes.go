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

const (
	defaultKubernetesPodCIDR     = "100.96.0.0/11"
	defaultKubernetesServiceCIDR = "100.64.0.0/13"
)

// Values builds the network policies of the workspaces from the cluster CIDRs.
func Values(kubernetes files.KubernetesConfig) map[string]any {
	return map[string]any{"networkPolicies": networkPoliciesValues(kubernetes)}
}

func networkPoliciesValues(kubernetes files.KubernetesConfig) map[string]any {
	podCIDR, serviceCIDR := kubernetes.PodCIDR, kubernetes.ServiceCIDR
	if kubernetes.ManagedByCodesphere {
		podCIDR = cmp.Or(podCIDR, defaultKubernetesPodCIDR)
		serviceCIDR = cmp.Or(serviceCIDR, defaultKubernetesServiceCIDR)
	}

	return map[string]any{
		"workspace": map[string]any{
			"namespace":     WorkspaceNamespace,
			"podSubnet":     podCIDR,
			"serviceSubnet": serviceCIDR,
		},
	}
}
