// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/cluster"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Values", func() {
	It("every key configured -> each key's values", func() {
		conf := files.CodesphereConfig{
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
			Plans: files.PlansConfig{HostingPlans: map[int]files.HostingPlan{
				8: {PooledInstances: ptr.To(2)},
				9: {GPUParts: 1, PooledInstances: ptr.To(1)},
			}},
			WorkspaceImages: &files.WorkspaceImagesConfig{
				Agent:    &files.ImageRef{BomRef: "workspace-agent-24.04", Dockerfile: "Dockerfile.agent"},
				AgentGpu: &files.ImageRef{ImageName: "registry.example.com/custom/workspace-agent-gpu"},
				Server:   &files.ImageRef{ImageName: "registry.example.com/custom/workspace-server"},
				VPN:      &files.ImageRef{BomRef: "vpn"},
			},
			CertIssuer: &files.CertIssuerConfig{Type: files.CertIssuerTypeACME, Acme: &files.ACMEConfig{
				Server:                "https://acme.example.com/directory",
				Email:                 "ops@example.com",
				EABKeyID:              "cluster-key",
				CustomDomainsEABKeyID: "custom-domains-key",
			}},
			CustomDomains: files.CustomDomainsConfig{
				CNameBaseDomain:                 "custom.example.com",
				InsecureSkipDomainControlVerify: ptr.To(true),
				Cert:                            &files.CustomDomainsCertConfig{Algorithm: "ECDSA", Size: 256},
			},
			IPService: &files.IPServiceConfig{LoadBalancerKind: "metallb", AddressPools: []string{"public"}},
		}
		gateways := cluster.Gateways{
			Codesphere: cluster.GatewayAPI{Enabled: true},
			Workspaces: cluster.GatewayAPI{Enabled: true, Use: true},
		}
		metalLB := &files.MetalLBConfig{
			Enabled: true,
			Pools:   []files.MetalLBPoolDef{{Name: "public", IPAddresses: []string{"203.0.113.0/28"}}},
		}
		registry := &files.RegistryConfig{Server: "registry.example.com"}

		values, err := codesphere.Values(conf, defaultTestImages(), gateways, metalLB, registry)
		Expect(err).NotTo(HaveOccurred())

		Expect(values).To(Equal(codesphere.Codesphere{
			PublicIP: "203.0.113.10",
			CertIssuer: codesphere.CertIssuer{ACME: codesphere.ACME{Enabled: true, ACMEAccount: &codesphere.ACMEAccount{
				Server:        "https://acme.example.com/directory",
				Email:         "ops@example.com",
				EABKeyID:      "custom-domains-key",
				EABMacKeyName: "acmeCustomDomainsEabMacKey",
			}}},
			WorkspaceHostingBaseDomain: "ws.example.com",
			SSHWorkspaceProxy:          codesphere.SSHWorkspaceProxy{BaseDomain: "ssh.example.com"},
			VPN:                        codesphere.VPN{LeftPeerID: "vpn.example.com"},
			DNSServers:                 []string{"1.1.1.1", "9.9.9.9"},
			Hosts:                      []string{"codesphere.example.com"},
			PrePullImages:              false,
			CustomDomains: codesphere.CustomDomains{
				CNameBaseDomain:                 "custom.example.com",
				GatewayAPI:                      codesphere.CustomDomainsGatewayAPI{UseGateway: true},
				InsecureSkipDomainControlVerify: ptr.To(true),
				Cert:                            &codesphere.CustomDomainsCert{Algorithm: "ECDSA", Size: 256},
			},
			DeployConfig: codesphere.DeployConfig{Images: map[string]codesphere.DeployImage{
				"ubuntu-24.04": {
					Name:           "Ubuntu 24.04",
					SupportedUntil: "2028-05-31T00:00:00.000Z",
					Flavors: map[string]codesphere.Flavor{
						"default": {Image: "registry.example.com/ubuntu-24.04-default", Pool: map[string]int{"8": 2}},
						"gpu":     {Image: "registry.example.com/custom/workspace-agent-gpu", Pool: map[string]int{"9": 1}},
					},
				},
			}},
			WorkspaceImages: codesphere.WorkspaceImages{
				Server: "registry.example.com/custom/workspace-server",
				VPN:    imagePrefix + "vpn",
			},
			FrontendGateway: codesphere.FrontendGateway{
				Issuer:        "acme",
				RedirectToIde: true,
				Image:         codesphere.Image{Name: "ghcr.io/codesphere-cloud/docker/nginx-gateway", Tag: "1.31.5"},
				GatewayAPI:    codesphere.GatewayAPI{Enabled: true},
			},
			IPService:             &codesphere.IPService{LoadBalancerKind: "metallb", AddressPools: []string{"public"}},
			TLSCertificate:        &codesphere.TLSCertificate{Cert: "cert-pem", Key: "key-pem"},
			UnderprovisionFactors: &codesphere.UnderprovisionFactors{CPU: 0.5, Memory: 0.75},
			ExtraWorkspaceFiles: []codesphere.ExtraWorkspaceFile{
				{Name: ".npmrc", Content: "cmVnaXN0cnk=", Encoded: true},
			},
			ExtraCAPem:            "ca-pem",
			ExtraWorkspaceEnvVars: map[string]string{"FOO": "bar"},
		}))
	})

	It("no keys configured -> default DNS server and pre-pulling, optional keys left out", func() {
		values, err := codesphere.Values(files.CodesphereConfig{}, defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		Expect(values.DNSServers).To(Equal([]string{"8.8.8.8"}))
		Expect(values.PrePullImages).To(BeTrue())
		Expect(values.SSHWorkspaceProxy).To(Equal(codesphere.SSHWorkspaceProxy{}))
		Expect(values.TLSCertificate).To(BeNil())
		Expect(values.UnderprovisionFactors).To(BeNil())
		Expect(values.ExtraWorkspaceFiles).To(BeNil())
		Expect(values.ExtraWorkspaceEnvVars).To(BeNil())
	})

	It("empty DNS server list -> kept empty", func() {
		conf := files.CodesphereConfig{DNSServers: []string{}}

		values, err := codesphere.Values(conf, defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		Expect(values.DNSServers).To(Equal([]string{}))
	})

	It("disabled gateway APIs -> frontend gateway and custom domains without them", func() {
		values, err := codesphere.Values(files.CodesphereConfig{}, defaultTestImages(), cluster.Gateways{}, noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		Expect(values.FrontendGateway.GatewayAPI.Enabled).To(BeFalse())
		Expect(values.CustomDomains.GatewayAPI.UseGateway).To(BeFalse())
	})
})
