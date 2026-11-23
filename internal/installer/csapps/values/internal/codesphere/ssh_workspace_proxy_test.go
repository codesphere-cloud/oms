// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("SSHWorkspaceProxy", func() {
	It("no sshWorkspaceProxy block -> chart default base domain kept", func() {
		values, err := codesphere.Values(files.CodesphereConfig{}, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

		Expect(values.SSHWorkspaceProxy).To(BeNil())
	})

	// init install-config and the bootstrappers write the service block without a base domain.
	It("only a service configured -> chart default base domain kept", func() {
		conf := files.CodesphereConfig{SSHWorkspaceProxy: &files.SSHWorkspaceProxyConfig{
			Service: &files.SSHWorkspaceProxyServiceConfig{Type: "LoadBalancer", NodePort: 30022},
		}}

		values, err := codesphere.Values(conf, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

		Expect(values.SSHWorkspaceProxy).To(BeNil())
	})

	It("base domain and service configured -> only the base domain", func() {
		conf := files.CodesphereConfig{SSHWorkspaceProxy: &files.SSHWorkspaceProxyConfig{
			BaseDomain: "ssh.example.com",
			Service:    &files.SSHWorkspaceProxyServiceConfig{Type: "NodePort", NodePort: 30022},
		}}

		values, err := codesphere.Values(conf, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

		Expect(values.SSHWorkspaceProxy).To(Equal(&codesphere.SSHWorkspaceProxy{BaseDomain: "ssh.example.com"}))
	})
})
