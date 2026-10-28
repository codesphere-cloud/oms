// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Values", func() {
	It("every key configured -> each key's values", func() {
		values := codesphere.Values(files.CodesphereConfig{
			Domain:                     "codesphere.example.com",
			PublicIP:                   "203.0.113.10",
			WorkspaceHostingBaseDomain: "ws.example.com",
			DNSServers:                 []string{"1.1.1.1", "9.9.9.9"},
			SSHWorkspaceProxy:          &files.SSHWorkspaceProxyConfig{BaseDomain: "ssh.example.com"},
			VPNLeftPeerID:              "vpn.example.com",
			PrePullImages:              ptr.To(false),
			CustomCertificate:          &files.CustomCertificate{Cert: "cert-pem", Key: "key-pem"},
			UnderprovisionFactors:      &files.UnderprovisionFactors{CPU: 0.5, Memory: 0.75},
			ExtraWorkspaceFiles:        []files.ExtraWorkspaceFile{{Name: ".npmrc", Content: "cmVnaXN0cnk=", Encoded: true}},
			ExtraCAPem:                 "ca-pem",
			ExtraWorkspaceEnvVars:      map[string]string{"FOO": "bar"},
		})

		Expect(values).To(Equal(codesphere.Codesphere{
			PublicIP:                   "203.0.113.10",
			WorkspaceHostingBaseDomain: "ws.example.com",
			VPN:                        codesphere.VPN{LeftPeerID: "vpn.example.com"},
			DNSServers:                 []string{"1.1.1.1", "9.9.9.9"},
			Hosts:                      []string{"codesphere.example.com"},
			PrePullImages:              false,
			SSHWorkspaceProxy:          &files.SSHWorkspaceProxyConfig{BaseDomain: "ssh.example.com"},
			TLSCertificate:             &codesphere.TLSCertificate{Cert: "cert-pem", Key: "key-pem"},
			UnderprovisionFactors:      &files.UnderprovisionFactors{CPU: 0.5, Memory: 0.75},
			ExtraWorkspaceFiles:        []files.ExtraWorkspaceFile{{Name: ".npmrc", Content: "cmVnaXN0cnk=", Encoded: true}},
			ExtraCAPem:                 "ca-pem",
			ExtraWorkspaceEnvVars:      map[string]string{"FOO": "bar"},
		}))
	})

	It("no keys configured -> default DNS server and pre-pulling, optional keys left out", func() {
		values := codesphere.Values(files.CodesphereConfig{})

		Expect(values).To(Equal(codesphere.Codesphere{
			DNSServers:    []string{"8.8.8.8"},
			Hosts:         []string{""},
			PrePullImages: true,
		}))
	})

	It("empty DNS server list -> kept empty", func() {
		values := codesphere.Values(files.CodesphereConfig{DNSServers: []string{}})

		Expect(values.DNSServers).To(Equal([]string{}))
	})
})
