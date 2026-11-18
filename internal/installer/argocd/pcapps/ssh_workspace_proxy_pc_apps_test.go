// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/argocd/pcapps"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("SSHWorkspaceProxyPcAppsValues", func() {
	// The service block of the ssh-workspace-proxy application in the rendered values.
	serviceValues := func(values files.ChartValues) files.ChartValues {
		apps, ok := values["applications"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected an applications map")

		proxy, ok := apps["ssh-workspace-proxy"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected an ssh-workspace-proxy application")

		valuesObject, ok := proxy["valuesObject"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected a valuesObject")

		service, ok := valuesObject["service"].(files.ChartValues)
		Expect(ok).To(BeTrue(), "expected a service block")

		return service
	}

	configWith := func(proxy *files.SSHWorkspaceProxyConfig) *files.RootConfig {
		return &files.RootConfig{Codesphere: files.CodesphereConfig{SSHWorkspaceProxy: proxy}}
	}

	It("returns nil when sshWorkspaceProxy is not set", func() {
		Expect(pcapps.SSHWorkspaceProxyPcAppsValues(configWith(nil))).To(BeNil())
	})

	It("returns nil when sshWorkspaceProxy has no service", func() {
		Expect(pcapps.SSHWorkspaceProxyPcAppsValues(configWith(&files.SSHWorkspaceProxyConfig{BaseDomain: "ssh.example.com"}))).To(BeNil())
	})

	It("defaults an empty service to a LoadBalancer on node port 30022", func() {
		values := pcapps.SSHWorkspaceProxyPcAppsValues(configWith(&files.SSHWorkspaceProxyConfig{
			Service: &files.SSHWorkspaceProxyServiceConfig{},
		}))

		Expect(serviceValues(values)).To(Equal(files.ChartValues{"type": "LoadBalancer", "nodePort": 30022}))
	})

	It("uses the configured type and node port", func() {
		values := pcapps.SSHWorkspaceProxyPcAppsValues(configWith(&files.SSHWorkspaceProxyConfig{
			Service: &files.SSHWorkspaceProxyServiceConfig{Type: "NodePort", NodePort: 31022},
		}))

		Expect(serviceValues(values)).To(Equal(files.ChartValues{"type": "NodePort", "nodePort": 31022}))
	})

	// Kubernetes rejects a nodePort on a ClusterIP service.
	It("leaves the node port out for ClusterIP", func() {
		values := pcapps.SSHWorkspaceProxyPcAppsValues(configWith(&files.SSHWorkspaceProxyConfig{
			Service: &files.SSHWorkspaceProxyServiceConfig{Type: "ClusterIP", NodePort: 30022},
		}))

		Expect(serviceValues(values)).To(Equal(files.ChartValues{"type": "ClusterIP"}))
	})
})
