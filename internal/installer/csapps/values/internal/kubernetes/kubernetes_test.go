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

		Expect(values).To(Equal(map[string]any{
			"networkPolicies": map[string]any{
				"workspace": map[string]any{
					"namespace":     "workspaces",
					"podSubnet":     "100.96.0.0/11",
					"serviceSubnet": "100.64.0.0/13",
				},
			},
		}))
	})

	It("external Kubernetes with CIDRs -> configured subnets", func() {
		values := kubernetes.Values(files.KubernetesConfig{
			PodCIDR:     "10.0.0.0/16",
			ServiceCIDR: "10.1.0.0/16",
		})

		Expect(values).To(Equal(map[string]any{
			"networkPolicies": map[string]any{
				"workspace": map[string]any{
					"namespace":     "workspaces",
					"podSubnet":     "10.0.0.0/16",
					"serviceSubnet": "10.1.0.0/16",
				},
			},
		}))
	})

	It("external Kubernetes without CIDRs -> no default subnets", func() {
		values := kubernetes.Values(files.KubernetesConfig{})

		workspace := values["networkPolicies"].(map[string]any)["workspace"].(map[string]any)
		Expect(workspace["podSubnet"]).To(BeEmpty())
		Expect(workspace["serviceSubnet"]).To(BeEmpty())
	})
})
