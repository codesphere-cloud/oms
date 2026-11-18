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

	It("enables the proxy and its service when sshWorkspaceProxy is not set", func() {
		values := pcapps.SSHWorkspaceProxyPcAppsValues(configWith(nil))

		Expect(values).To(Equal(files.ChartValues{
			"applications": files.ChartValues{
				"ssh-workspace-proxy": files.ChartValues{
					"enabled":      true,
					"valuesObject": files.ChartValues{"service": files.ChartValues{"enabled": true}},
				},
			},
		}))
	})

	It("leaves the service settings to the chart when sshWorkspaceProxy has no service", func() {
		values := pcapps.SSHWorkspaceProxyPcAppsValues(configWith(&files.SSHWorkspaceProxyConfig{BaseDomain: "ssh.example.com"}))

		Expect(serviceValues(values)).To(Equal(files.ChartValues{"enabled": true}))
	})

	It("enables the proxy when a service is configured", func() {
		values := pcapps.SSHWorkspaceProxyPcAppsValues(configWith(&files.SSHWorkspaceProxyConfig{
			Service: &files.SSHWorkspaceProxyServiceConfig{Type: "ClusterIP"},
		}))

		Expect(values["applications"]).To(HaveKeyWithValue("ssh-workspace-proxy", HaveKeyWithValue("enabled", true)))
	})

	It("defaults an empty service to a local-traffic LoadBalancer on node port 30022", func() {
		values := pcapps.SSHWorkspaceProxyPcAppsValues(configWith(&files.SSHWorkspaceProxyConfig{
			Service: &files.SSHWorkspaceProxyServiceConfig{},
		}))

		Expect(serviceValues(values)).To(Equal(files.ChartValues{
			"enabled":               true,
			"type":                  "LoadBalancer",
			"nodePort":              30022,
			"externalTrafficPolicy": "Local",
		}))
	})

	It("pins the load balancer IP and adds the configured annotations", func() {
		values := pcapps.SSHWorkspaceProxyPcAppsValues(configWith(&files.SSHWorkspaceProxyConfig{
			Service: &files.SSHWorkspaceProxyServiceConfig{
				LoadBalancerIP: "203.0.113.22",
				Annotations:    map[string]string{"cloud.google.com/load-balancer-ipv4": "203.0.113.22"},
			},
		}))

		Expect(serviceValues(values)).To(Equal(files.ChartValues{
			"enabled":               true,
			"type":                  "LoadBalancer",
			"nodePort":              30022,
			"externalTrafficPolicy": "Local",
			"loadBalancerIP":        "203.0.113.22",
			"annotations":           files.ChartValues{"cloud.google.com/load-balancer-ipv4": "203.0.113.22"},
		}))
	})

	It("uses the configured type and node port without load balancer settings", func() {
		values := pcapps.SSHWorkspaceProxyPcAppsValues(configWith(&files.SSHWorkspaceProxyConfig{
			Service: &files.SSHWorkspaceProxyServiceConfig{Type: "NodePort", NodePort: 31022},
		}))

		Expect(serviceValues(values)).To(Equal(files.ChartValues{"enabled": true, "type": "NodePort", "nodePort": 31022}))
	})

	It("adds the configured annotations to a service without a load balancer", func() {
		values := pcapps.SSHWorkspaceProxyPcAppsValues(configWith(&files.SSHWorkspaceProxyConfig{
			Service: &files.SSHWorkspaceProxyServiceConfig{Type: "NodePort", Annotations: map[string]string{"team": "platform"}},
		}))

		Expect(serviceValues(values)).To(HaveKeyWithValue("annotations", files.ChartValues{"team": "platform"}))
	})

	// Kubernetes rejects a nodePort on a ClusterIP service.
	It("leaves the node port out for ClusterIP", func() {
		values := pcapps.SSHWorkspaceProxyPcAppsValues(configWith(&files.SSHWorkspaceProxyConfig{
			Service: &files.SSHWorkspaceProxyServiceConfig{Type: "ClusterIP", NodePort: 30022},
		}))

		Expect(serviceValues(values)).To(Equal(files.ChartValues{"enabled": true, "type": "ClusterIP"}))
	})
})
