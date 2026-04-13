// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Installconfig & Secrets - Unexported", func() {
	Describe("buildCephHostsConfig", func() {
		FIt("builds a valid Ceph hosts config with a single node as master", func() {
			nodes := []*node.Node{
				{
					Name:       "ceph-node-1",
					InternalIP: "10.0.0.1",
				},
			}
			expected := []files.CephHost{
				{
					Hostname:  "ceph-node-1",
					IsMaster:  true,
					IPAddress: "10.0.0.1",
				},
			}
			actual := buildCephHostsConfig(nodes)
			Expect(actual).To(Equal(expected))
		})

		FIt("builds a valid Ceph hosts config with three nodes", func() {
			nodes := []*node.Node{
				{
					Name:       "ceph-node-1",
					InternalIP: "10.0.0.1",
				},
				{
					Name:       "ceph-node-2",
					InternalIP: "10.0.0.2",
				},
				{
					Name:       "ceph-node-3",
					InternalIP: "10.0.0.3",
				},
			}
			expected := []files.CephHost{
				{
					Hostname:  "ceph-node-1",
					IsMaster:  true,
					IPAddress: "10.0.0.1",
				},
				{
					Hostname:  "ceph-node-2",
					IsMaster:  false,
					IPAddress: "10.0.0.2",
				},
				{
					Hostname:  "ceph-node-3",
					IsMaster:  false,
					IPAddress: "10.0.0.3",
				},
			}
			actual := buildCephHostsConfig(nodes)
			Expect(actual).To(Equal(expected))
		})

		FIt("builds an empty Ceph hosts config with no nodes", func() {
			nodes := []*node.Node{}
			expected := []files.CephHost{}
			actual := buildCephHostsConfig(nodes)
			Expect(actual).To(Equal(expected))
		})
	})

	Describe("buildKubernetesConfig", func() {
		FIt("builds a valid Kubernetes config with a single node as worker and control plane", func() {
			nodes := []*node.Node{
				{
					Name:       "node-1",
					InternalIP: "10.0.0.1",
				},
			}
			expected := files.KubernetesConfig{
				ManagedByCodesphere: true,
				APIServerHost:       "10.0.0.1",
				ControlPlanes: []files.K8sNode{
					{
						IPAddress: "10.0.0.1",
					},
				},
				Workers: []files.K8sNode{
					{
						IPAddress: "10.0.0.1",
					},
				},
			}
			actual := buildKubernetesConfig(nodes)
			Expect(actual).To(Equal(expected))
		})

		FIt("builds a valid Kubernetes config with three nodes", func() {
			nodes := []*node.Node{
				{
					Name:       "node-1",
					InternalIP: "10.0.0.1",
				},
				{
					Name:       "node-2",
					InternalIP: "10.0.0.2",
				},
				{
					Name:       "node-3",
					InternalIP: "10.0.0.3",
				},
			}
			expected := files.KubernetesConfig{
				ManagedByCodesphere: true,
				APIServerHost:       "10.0.0.1",
				ControlPlanes: []files.K8sNode{
					{
						IPAddress: "10.0.0.1",
					},
				},
				Workers: []files.K8sNode{
					{
						IPAddress: "10.0.0.1",
					},
					{
						IPAddress: "10.0.0.2",
					},
					{
						IPAddress: "10.0.0.3",
					},
				},
			}
			actual := buildKubernetesConfig(nodes)
			Expect(actual).To(Equal(expected))
		})

		FIt("builds an empty Kubernetes config with no nodes", func() {
			nodes := []*node.Node{}
			expected := files.KubernetesConfig{
				APIServerHost: "",
				ControlPlanes: []files.K8sNode{},
				Workers:       []files.K8sNode{},
			}
			actual := buildKubernetesConfig(nodes)
			Expect(actual).To(Equal(expected))
		})
	})
})
