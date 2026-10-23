// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package kubernetes_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/kubernetes"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Values", func() {
	It("Codesphere-managed Kubernetes without CIDRs -> default subnets", func() {
		values := kubernetes.Values(files.KubernetesConfig{ManagedByCodesphere: true})

		Expect(values).To(Equal(kubernetes.Kubernetes{
			NetworkPolicies: kubernetes.NetworkPolicies{Workspace: kubernetes.WorkspaceNetworkPolicy{
				Namespace:     "workspaces",
				PodSubnet:     "100.96.0.0/11",
				ServiceSubnet: "100.64.0.0/13",
			}},
		}))
	})

	It("external Kubernetes with CIDRs -> configured subnets", func() {
		values := kubernetes.Values(files.KubernetesConfig{
			PodCIDR:     "10.0.0.0/16",
			ServiceCIDR: "10.1.0.0/16",
		})

		Expect(values).To(Equal(kubernetes.Kubernetes{
			NetworkPolicies: kubernetes.NetworkPolicies{Workspace: kubernetes.WorkspaceNetworkPolicy{
				Namespace:     "workspaces",
				PodSubnet:     "10.0.0.0/16",
				ServiceSubnet: "10.1.0.0/16",
			}},
		}))
	})

	It("external Kubernetes without CIDRs -> no default subnets", func() {
		values := kubernetes.Values(files.KubernetesConfig{})

		workspace := values.NetworkPolicies.Workspace
		Expect(workspace.PodSubnet).To(BeEmpty())
		Expect(workspace.ServiceSubnet).To(BeEmpty())
	})
})
