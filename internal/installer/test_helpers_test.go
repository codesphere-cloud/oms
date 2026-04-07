// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import "github.com/codesphere-cloud/oms/internal/installer/files"

// newTestConfig creates a minimal RootConfig for testing.
// Pass control plane IPs as variadic args; omit for nil ControlPlanes.
func newTestConfig(name string, managed bool, ips ...string) *files.RootConfig {
	var nodes []files.K8sNode
	if len(ips) > 0 {
		nodes = make([]files.K8sNode, len(ips))
		for i, ip := range ips {
			nodes[i] = files.K8sNode{IPAddress: ip}
		}
	}
	return &files.RootConfig{
		Datacenter: files.DatacenterConfig{
			ID:   1,
			Name: name,
		},
		Kubernetes: files.KubernetesConfig{
			ManagedByCodesphere: managed,
			ControlPlanes:       nodes,
		},
	}
}
