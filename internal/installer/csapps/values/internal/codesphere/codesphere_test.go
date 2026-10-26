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
			IPService:   &files.IPServiceConfig{LoadBalancerKind: "metallb", AddressPools: []string{"public"}},
			Experiments: []string{"legacy"}, //nolint:staticcheck
			Internal:    []string{"internal-flag"},
			Preview:     map[string]bool{"preview-on": true},
			Features:    map[string]bool{"feature-on": true, "billing": false},
			GitProviders: &files.GitProvidersConfig{
				GitHub: &files.GitProviderConfig{
					Enabled: true,
					URL:     "https://github.example.com",
					API:     files.APIConfig{BaseURL: "https://api.github.example.com"},
					OAuth: files.OAuthConfig{
						Issuer:                "https://github.example.com",
						AuthorizationEndpoint: "https://github.example.com/login/oauth/authorize",
						TokenEndpoint:         "https://github.example.com/login/oauth/access_token",
						ClientAuthMethod:      "client_secret_post",
						Scope:                 "repo",
						RedirectURI:           "https://codesphere.example.com/oauth/callback",
						InstallationURI:       "https://github.example.com/apps/codesphere/installations/new",
					},
				},
				SCMManager: &files.SCMManagerConfig{
					Enabled: true,
					URL:     "https://scm.example.com",
					API:     &files.APIConfig{BaseURL: "https://scm.example.com/api"},
				},
			},
			OAuth: &files.OAuthProvidersConfig{Oidc: &files.OidcOAuthProvider{
				Type:      "oidc",
				Enabled:   true,
				Name:      "example-idp",
				IssuerURL: "https://idp.example.com",
				Scopes:    []string{"openid"},
			}},
			SignUp: &files.SignUpConfig{Email: &files.SignUpEmailConfig{AllowedDomains: []string{"example.com"}}},
			Email: &files.EmailConfig{
				Provider: "smtp",
				SMTP:     files.SMTPConfig{Host: "smtp.example.com", Port: 587, Secure: true, Username: "codesphere"},
			},
			Branding: &files.BrandingConfig{
				Desc:        "Example IDE",
				DocsURL:     "https://docs.example.com",
				FaviconHref: "https://example.com/favicon.ico",
				Title:       "Example",
				TosURL:      "https://example.com/tos",
			},
			ManagedServices: []files.ManagedServiceConfig{{
				Name:          "valkey",
				SchemaVersion: "v0",
				Backend:       map[string]any{"landscape": map[string]any{"gitUrl": "https://git.example.com/valkey"}},
				Versions:      map[string]any{"1.0.0": map[string]any{"appVersion": "8.1"}},
			}},
			ManagedServiceWorkspacePlanID: ptr.To(21),
			OpenBao:                       &files.OpenBaoConfig{Engine: "kv", URI: "https://bao.example.com", User: "codesphere"},
			OpenFga:                       &files.OpenFgaConfig{Deploy: ptr.To(false), APIURL: "https://fga.example.com"},
			TelemetryExport: &files.TelemetryExport{
				RemoteExport:   true,
				RemoteEndpoint: "https://otel.example.com",
				SpanMetrics:    true,
				Traces:         true,
				TraceEndpoint:  "tempo.example.com:4317",
			},
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
			GitProviders: codesphere.GitProviders{
				AzureDevOps: codesphere.GitProvider{Enabled: false},
				GitHub: codesphere.GitProvider{Enabled: true, GitProviderConnection: &codesphere.GitProviderConnection{
					URL: "https://github.example.com",
					API: codesphere.GitProviderAPI{BaseURL: "https://api.github.example.com"},
					OAuth: codesphere.GitProviderOAuth{
						Issuer:                "https://github.example.com",
						AuthorizationEndpoint: "https://github.example.com/login/oauth/authorize",
						TokenEndpoint:         "https://github.example.com/login/oauth/access_token",
						RedirectURI:           "https://codesphere.example.com/oauth/callback",
						InstallationURI:       "https://github.example.com/apps/codesphere/installations/new",
						ClientAuthMethod:      "client_secret_post",
						Scope:                 "repo",
					},
				}},
				GitLab:    codesphere.GitProvider{Enabled: false},
				Bitbucket: codesphere.GitProvider{Enabled: false},
				SCMManager: codesphere.SCMManager{Enabled: true, SCMManagerConnection: &codesphere.SCMManagerConnection{
					URL: "https://scm.example.com",
					API: codesphere.SCMManagerAPI{BaseURL: "https://scm.example.com/api"},
				}},
			},
			Experiments: codesphere.Flags{
				"hermetic":        {codesphere.FlagEnvProd},
				"persistent-nix":  {codesphere.FlagEnvProd},
				"gateway-domains": {codesphere.FlagEnvProd},
				"legacy":          {codesphere.FlagEnvProd},
				"internal-flag":   {codesphere.FlagEnvProd},
				"preview-on":      {codesphere.FlagEnvProd},
			},
			Features: codesphere.Flags{
				"email-signup":     {codesphere.FlagEnvProd},
				"email-signin":     {codesphere.FlagEnvProd},
				"standalone-teams": {codesphere.FlagEnvProd},
				"billing":          {},
				"feature-on":       {codesphere.FlagEnvProd},
			},
			CustomDomains: codesphere.CustomDomains{
				CNameBaseDomain:                 "custom.example.com",
				GatewayAPI:                      codesphere.CustomDomainsGatewayAPI{UseGateway: true},
				InsecureSkipDomainControlVerify: ptr.To(true),
				Cert:                            &codesphere.CustomDomainsCert{Algorithm: "ECDSA", Size: 256},
			},
			OAuth: codesphere.OAuth{
				Providers: map[string]codesphere.OAuthProvider{
					"bitbucket": {Enabled: false},
					"github":    {Enabled: false},
					"gitlab":    {Enabled: false},
					"google":    {Enabled: false},
				},
				AdditionalProviders: map[string]codesphere.OIDCProvider{
					"example-idp": {
						Type:      "oidc",
						Enabled:   true,
						Name:      "example-idp",
						IssuerURL: "https://idp.example.com",
						Scopes:    []string{"openid"},
					},
				},
			},
			SignUp:                        codesphere.SignUp{Email: codesphere.SignUpEmail{AllowedDomains: []string{"example.com"}}},
			ManagedServiceWorkspacePlanID: 21,
			Otel: codesphere.Otel{
				CentralOtelExport: codesphere.CentralOtelExport{
					Enabled:              true,
					TelemetryCredentials: codesphere.TelemetryCredentials{SecretName: "otel-collector-central-creds"},
					OTLPHTTPEndpoint:     "https://otel.example.com",
				},
				TelemetryExport: codesphere.TelemetryExport{
					SpanMetrics: codesphere.SpanMetrics{Enabled: true},
					Traces:      codesphere.Traces{Enabled: true, Endpoint: "tempo.example.com:4317"},
				},
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
				GatewayAPI:    codesphere.FrontendGatewayAPI{Enabled: true},
			},
			IPService: &codesphere.IPService{LoadBalancerKind: "metallb", AddressPools: []string{"public"}},
			ManagedServices: []files.ManagedServiceConfig{{
				Name:          "valkey",
				Scope:         "global",
				SchemaVersion: "v0",
				Backend:       map[string]any{"landscape": map[string]any{"gitUrl": "https://git.example.com/valkey"}},
				Versions:      map[string]any{"1.0.0": map[string]any{"appVersion": "8.1"}},
			}},
			Branding: &files.BrandingConfig{
				Desc:        "Example IDE",
				DocsURL:     "https://docs.example.com",
				FaviconHref: "https://example.com/favicon.ico",
				Title:       "Example",
				TosURL:      "https://example.com/tos",
			},
			TLSCertificate:        &codesphere.TLSCertificate{Cert: "cert-pem", Key: "key-pem"},
			UnderprovisionFactors: &codesphere.UnderprovisionFactors{CPU: 0.5, Memory: 0.75},
			ExtraWorkspaceFiles: []codesphere.ExtraWorkspaceFile{
				{Name: ".npmrc", Content: "cmVnaXN0cnk=", Encoded: true},
			},
			SMTP:                  &files.SMTPConfig{Host: "smtp.example.com", Port: 587, Secure: true, Username: "codesphere"},
			OpenBao:               &files.OpenBaoConfig{Engine: "kv", URI: "https://bao.example.com", User: "codesphere"},
			OpenFGA:               &codesphere.OpenFGA{APIURL: "https://fga.example.com"},
			ExtraCAPem:            "ca-pem",
			ExtraWorkspaceEnvVars: map[string]string{"FOO": "bar"},
		}))
	})

	It("no keys configured -> optional keys left out", func() {
		values, err := codesphere.Values(files.CodesphereConfig{}, defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

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

	It("disabled gateway APIs -> no gateway domains experiment", func() {
		values, err := codesphere.Values(files.CodesphereConfig{}, defaultTestImages(), cluster.Gateways{}, noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		Expect(values.FrontendGateway.GatewayAPI.Enabled).To(BeFalse())
		Expect(values.CustomDomains.GatewayAPI.UseGateway).To(BeFalse())
		Expect(values.Experiments).NotTo(HaveKey("gateway-domains"))
	})
})
