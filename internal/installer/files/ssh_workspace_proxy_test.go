// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("SSH workspace proxy service", func() {
	It("defaults to a LoadBalancer and node port 30022", func() {
		service := &files.SSHWorkspaceProxyServiceConfig{}

		Expect(service.ServiceType()).To(Equal("LoadBalancer"))
		Expect(service.ServiceNodePort()).To(Equal(30022))
	})

	It("uses the configured type and node port", func() {
		service := &files.SSHWorkspaceProxyServiceConfig{Type: "NodePort", NodePort: 31022}

		Expect(service.ServiceType()).To(Equal("NodePort"))
		Expect(service.ServiceNodePort()).To(Equal(31022))
	})

	It("serves on a node port for NodePort", func() {
		Expect((&files.SSHWorkspaceProxyServiceConfig{Type: "NodePort"}).IsNodePort()).To(BeTrue())
	})

	// The chart ignores the node port of a LoadBalancer service.
	It("does not serve on a node port for the default LoadBalancer", func() {
		Expect((&files.SSHWorkspaceProxyServiceConfig{}).IsNodePort()).To(BeFalse())
	})

	It("does not serve on a node port for ClusterIP", func() {
		Expect((&files.SSHWorkspaceProxyServiceConfig{Type: "ClusterIP"}).IsNodePort()).To(BeFalse())
	})

	It("is a load balancer by default", func() {
		Expect((&files.SSHWorkspaceProxyServiceConfig{}).IsLoadBalancer()).To(BeTrue())
	})

	It("is not a load balancer for NodePort", func() {
		Expect((&files.SSHWorkspaceProxyServiceConfig{Type: "NodePort"}).IsLoadBalancer()).To(BeFalse())
	})

	It("has no service when the proxy block is unset", func() {
		var config *files.SSHWorkspaceProxyConfig

		Expect(config.ServiceConfig()).To(BeNil())
	})

	It("accepts an unset service", func() {
		var service *files.SSHWorkspaceProxyServiceConfig

		Expect(service.Validate()).To(Succeed())
	})

	It("accepts a valid service", func() {
		Expect((&files.SSHWorkspaceProxyServiceConfig{Type: "NodePort", NodePort: 30022}).Validate()).To(Succeed())
	})

	It("rejects an unknown service type", func() {
		err := (&files.SSHWorkspaceProxyServiceConfig{Type: "ExternalName"}).Validate()

		Expect(err).To(MatchError(ContainSubstring(`invalid codesphere.sshWorkspaceProxy.service.type "ExternalName"`)))
	})

	It("rejects a node port outside the port range", func() {
		err := (&files.SSHWorkspaceProxyServiceConfig{NodePort: 70000}).Validate()

		Expect(err).To(MatchError(ContainSubstring("invalid codesphere.sshWorkspaceProxy.service.nodePort 70000")))
	})

	It("accepts a load balancer IP for a LoadBalancer", func() {
		Expect((&files.SSHWorkspaceProxyServiceConfig{LoadBalancerIP: "203.0.113.22"}).Validate()).To(Succeed())
	})

	It("rejects a load balancer IP that is not an IP", func() {
		err := (&files.SSHWorkspaceProxyServiceConfig{LoadBalancerIP: "not-an-ip"}).Validate()

		Expect(err).To(MatchError(ContainSubstring(`invalid codesphere.sshWorkspaceProxy.service.loadBalancerIP "not-an-ip"`)))
	})

	It("rejects a load balancer IP for a service without a load balancer", func() {
		err := (&files.SSHWorkspaceProxyServiceConfig{Type: "NodePort", LoadBalancerIP: "203.0.113.22"}).Validate()

		Expect(err).To(MatchError(ContainSubstring("loadBalancerIP requires service type LoadBalancer, not NodePort")))
	})

	It("creates the service block when ensuring it", func() {
		config := &files.CodesphereConfig{}

		config.EnsureSSHWorkspaceProxyService().Type = "ClusterIP"

		Expect(config.SSHWorkspaceProxy.Service.Type).To(Equal("ClusterIP"))
	})

	It("keeps the configured service when ensuring it", func() {
		config := &files.CodesphereConfig{SSHWorkspaceProxy: &files.SSHWorkspaceProxyConfig{
			BaseDomain: "ssh.example.com",
			Service:    &files.SSHWorkspaceProxyServiceConfig{NodePort: 31022},
		}}

		config.EnsureSSHWorkspaceProxyService().Type = "LoadBalancer"

		Expect(config.SSHWorkspaceProxy.BaseDomain).To(Equal("ssh.example.com"))
		Expect(config.SSHWorkspaceProxy.Service).To(Equal(&files.SSHWorkspaceProxyServiceConfig{Type: "LoadBalancer", NodePort: 31022}))
	})
})
