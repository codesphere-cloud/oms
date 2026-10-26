// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package values_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	csappsvalues "github.com/codesphere-cloud/oms/internal/installer/csapps/values"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const authServiceDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var _ = Describe("Build", func() {
	It("minimal config -> installer defaults", func() {
		values := build(testConfig(), testBOM(nil))

		Expect(at(values, "global.dnsServers")).To(Equal([]any{"8.8.8.8"}))
		Expect(at(values, "global.prePullImages")).To(BeTrue())
		Expect(at(values, "global.managedServiceWorkspacePlanId")).To(Equal(20))
		Expect(at(values, "global.customDomains.gatewayApi.useGateway")).To(BeTrue())
		Expect(at(values, "global.frontendGateway.issuer")).To(Equal("self-signed"))
		Expect(at(values, "global.certIssuer")).To(Equal(map[string]any{
			"acme": map[string]any{"enabled": false},
		}))
		Expect(at(values, "global.vpn")).To(Equal(map[string]any{}))
		Expect(at(values, "global.experiments")).To(Equal(map[string]any{
			"hermetic":        []any{"prod"},
			"persistent-nix":  []any{"prod"},
			"gateway-domains": []any{"prod"},
		}))
		Expect(at(values, "global.features")).To(Equal(map[string]any{
			"email-signup":     []any{"prod"},
			"email-signin":     []any{"prod"},
			"standalone-teams": []any{"prod"},
			"billing":          []any{},
		}))
		Expect(at(values, "global.otel")).To(Equal(map[string]any{
			"centralOtelExport": map[string]any{
				"enabled":              false,
				"telemetryCredentials": map[string]any{"secretName": "otel-collector-central-creds"},
				"otlpHttpEndpoint":     "https://otel.tools.codesphere.com",
			},
			"telemetryExport": map[string]any{
				"spanMetrics": map[string]any{"enabled": false},
				"traces":      map[string]any{"enabled": false},
			},
		}))
		Expect(at(values, "global.imageTag")).To(Equal("v1.2.3"))
		Expect(at(values, "global.hostName")).To(Equal("codesphere.example.com"))
	})

	It("every section configured -> rendered under the chart's keys", func() {
		cfg := testConfig()
		cfg.Postgres = files.PostgresConfig{CACertPem: "ca", ServerAddress: "postgres.example.com", Port: 5433}
		cfg.Codesphere.VPNLeftPeerID = "peer-id.example"
		cfg.Codesphere.Experiments = []string{"foo"} //nolint:staticcheck
		cfg.Codesphere.Features = map[string]bool{"bar": true}
		cfg.Codesphere.Email = &files.EmailConfig{
			Provider: "smtp",
			SMTP:     files.SMTPConfig{Host: "smtp.example.com", Port: 465, Secure: true, Username: "username"},
		}
		cfg.Codesphere.IPService = &files.IPServiceConfig{LoadBalancerKind: "metallb", AddressPools: []string{"public"}}
		cfg.Codesphere.OAuth = &files.OAuthProvidersConfig{Oidc: &files.OidcOAuthProvider{
			Enabled:   true,
			Type:      "oidc",
			Name:      "idp",
			IssuerURL: "https://idp.example.com",
			Scopes:    []string{"openid"},
		}}
		cfg.Codesphere.GitProviders = &files.GitProvidersConfig{Bitbucket: &files.GitProviderConfig{
			Enabled: true,
			URL:     "https://bitbucket.example.com",
			API:     files.APIConfig{BaseURL: "https://api.bitbucket.example.com/2.0"},
			OAuth: files.OAuthConfig{
				Issuer:                "https://bitbucket.example.com",
				AuthorizationEndpoint: "https://bitbucket.example.com/site/oauth2/authorize",
				TokenEndpoint:         "https://bitbucket.example.com/access_token",
				ClientAuthMethod:      "client_secret_post",
			},
		}}
		cfg.Codesphere.CertIssuer = &files.CertIssuerConfig{Type: files.CertIssuerTypeACME, Acme: &files.ACMEConfig{
			Server:   "https://acme.example.com/directory",
			Email:    "certs@example.com",
			EABKeyID: "key-id",
		}}
		cfg.Codesphere.TelemetryExport = &files.TelemetryExport{
			RemoteExport:   true,
			RemoteEndpoint: "https://otel.example.com",
			SpanMetrics:    true,
			Traces:         true,
			TraceEndpoint:  "tempo.example.svc.cluster.local:4317",
		}
		values := build(cfg, testBOM(nil))

		Expect(at(values, "global.hosts")).To(Equal([]any{"codesphere.example.com"}))
		Expect(at(values, "global.frontendGateway")).To(Equal(map[string]any{
			"image":         map[string]any{"name": "ghcr.io/codesphere-cloud/docker/nginx-gateway", "tag": "1.31.5"},
			"issuer":        "acme",
			"redirectToIde": true,
			"gatewayApi":    map[string]any{"enabled": true},
		}))
		Expect(at(values, "global.customDomains")).To(Equal(map[string]any{
			"cNameBaseDomain": "custom.example.com",
			"gatewayApi":      map[string]any{"useGateway": true},
		}))
		Expect(at(values, "global.networkPolicies")).To(Equal(map[string]any{
			"workspace": map[string]any{
				"namespace":     "workspaces",
				"podSubnet":     "10.1.0.0/16",
				"serviceSubnet": "10.2.0.0/16",
			},
		}))
		Expect(at(values, "global.postgres")).To(Equal(map[string]any{
			"database":           "codesphere",
			"host":               "postgres.example.com",
			"port":               5433,
			"rejectUnauthorized": true,
			"ssl":                map[string]any{"ca": "ca", "serverName": "postgres.example.com"},
		}))
		Expect(at(values, "global.workspaceImages")).To(Equal(map[string]any{
			"server": imagePrefix + "workspace-server",
			"vpn":    imagePrefix + "vpn",
		}))
		Expect(at(values, "global.workspaceRouter")).To(Equal(map[string]any{
			"image":      map[string]any{"name": "ghcr.io/codesphere-cloud/docker/nginx", "tag": "1.31.5"},
			"haReplicas": 2,
		}))
		Expect(at(values, "global.vpn")).To(Equal(map[string]any{"leftPeerId": "peer-id.example"}))
		Expect(at(values, "global.experiments")).To(HaveKeyWithValue("foo", []any{"prod"}))
		Expect(at(values, "global.features")).To(HaveKeyWithValue("bar", []any{"prod"}))
		Expect(at(values, "global.smtp")).To(Equal(map[string]any{
			"host":     "smtp.example.com",
			"port":     465,
			"secure":   true,
			"username": "username",
		}))
		Expect(at(values, "global.ipService")).To(Equal(map[string]any{
			"loadBalancerKind": "metallb",
			"addressPools":     []any{"public"},
		}))
		Expect(at(values, "global.oauth")).To(Equal(map[string]any{
			"providers": map[string]any{
				"bitbucket": map[string]any{"enabled": false},
				"github":    map[string]any{"enabled": false},
				"gitlab":    map[string]any{"enabled": false},
				"google":    map[string]any{"enabled": false},
			},
			"additionalProviders": map[string]any{
				"idp": map[string]any{
					"enabled":   true,
					"type":      "oidc",
					"name":      "idp",
					"issuerUrl": "https://idp.example.com",
					"scopes":    []any{"openid"},
				},
			},
		}))
		Expect(at(values, "global.gitProviders")).To(Equal(map[string]any{
			"azureDevOps": map[string]any{"enabled": false},
			"bitbucket": map[string]any{
				"enabled": true,
				"url":     "https://bitbucket.example.com",
				"api":     map[string]any{"baseUrl": "https://api.bitbucket.example.com/2.0"},
				"oauth": map[string]any{
					"issuer":                "https://bitbucket.example.com",
					"authorizationEndpoint": "https://bitbucket.example.com/site/oauth2/authorize",
					"tokenEndpoint":         "https://bitbucket.example.com/access_token",
					"clientAuthMethod":      "client_secret_post",
				},
			},
			"github":     map[string]any{"enabled": false},
			"gitlab":     map[string]any{"enabled": false},
			"scmManager": map[string]any{"enabled": false},
		}))
		Expect(at(values, "global.certIssuer")).To(Equal(map[string]any{"acme": map[string]any{
			"enabled":       true,
			"server":        "https://acme.example.com/directory",
			"email":         "certs@example.com",
			"eabKeyId":      "key-id",
			"eabMacKeyName": "acmeEabMacKey",
		}}))
		Expect(at(values, "global.otel")).To(Equal(map[string]any{
			"centralOtelExport": map[string]any{
				"enabled":              true,
				"telemetryCredentials": map[string]any{"secretName": "otel-collector-central-creds"},
				"otlpHttpEndpoint":     "https://otel.example.com",
			},
			"telemetryExport": map[string]any{
				"spanMetrics": map[string]any{"enabled": true},
				"traces": map[string]any{
					"enabled":  true,
					"endpoint": "tempo.example.svc.cluster.local:4317",
				},
			},
		}))
	})

	It("workspace PVC storage without Ceph config -> no active MDS", func() {
		cfg := testConfig()
		cfg.Codesphere.Internal = []string{"workspace-pvc-storage"}
		values := build(cfg, testBOM(nil))

		Expect(at(values, "global.ceph.activeMds")).To(Equal("0"))
	})

	It("several data centers -> target, default and all of them rendered", func() {
		cfg := testConfig()
		cfg.DataCenters = []files.DatacenterConfig{
			{ID: 1, Name: "main", City: "Karlsruhe", CountryCode: "DE"},
			{ID: 2, Name: "second", City: "Frankfurt", CountryCode: "DE"},
		}
		cfg.DefaultDataCenterID = 2
		values := build(cfg, testBOM(nil))

		Expect(at(values, "global.dataCenterId")).To(Equal(1))
		Expect(at(values, "global.defaultDataCenterId")).To(Equal(2))
		Expect(at(values, "global.availableDataCenters")).To(Equal([]any{1, 2}))
		Expect(at(values, "global.dataCenters")).To(Equal([]any{
			map[string]any{"id": 1, "name": "main", "city": "Karlsruhe", "countryCode": "DE"},
			map[string]any{"id": 2, "name": "second", "city": "Frankfurt", "countryCode": "DE"},
		}))
	})

	It("ACME with a separate custom domains EAB key -> custom domains key used", func() {
		cfg := testConfig()
		cfg.Codesphere.CertIssuer = &files.CertIssuerConfig{Type: files.CertIssuerTypeACME, Acme: &files.ACMEConfig{
			Server:                "https://acme.example.com/directory",
			Email:                 "certs@example.com",
			EABKeyID:              "cluster-key",
			CustomDomainsEABKeyID: "custom-domains-key",
		}}
		values := build(cfg, testBOM(nil))

		acme := at(values, "global.certIssuer.acme")
		Expect(acme).To(HaveKeyWithValue("eabKeyId", "custom-domains-key"))
		Expect(acme).To(HaveKeyWithValue("eabMacKeyName", "acmeCustomDomainsEabMacKey"))
	})

	It("self-signed cert issuer -> ACME disabled", func() {
		cfg := testConfig()
		cfg.Codesphere.CertIssuer = &files.CertIssuerConfig{Type: files.CertIssuerTypeSelfSigned}
		values := build(cfg, testBOM(nil))

		Expect(at(values, "global.certIssuer")).To(Equal(map[string]any{
			"acme": map[string]any{"enabled": false},
		}))
		Expect(at(values, "global.frontendGateway.issuer")).To(Equal("self-signed"))
	})

	It("deploy config and pooled instances -> error", func() {
		cfg := testConfig()
		cfg.Codesphere.Plans.HostingPlans[8] = files.HostingPlan{PooledInstances: ptr.To(1)}
		cfg.Codesphere.DeployConfig = files.DeployConfig{Images: map[string]files.ImageConfig{}}

		_, err := csappsvalues.Build(cfg, testBOM(nil))

		Expect(err).To(MatchError(ContainSubstring("ambiguous")))
	})

	It("auth-service image without a tag -> release tag error", func() {
		untagged := map[string]string{"auth-service": imagePrefix + "auth-service@" + authServiceDigest}

		_, err := csappsvalues.Build(testConfig(), testBOM(untagged))

		Expect(err).To(MatchError(ContainSubstring("missing a tag")))
	})

	It("override -> wins over the installer's values, null kept for Helm", func() {
		cfg := testConfig()
		cfg.Codesphere.Override = files.ChartOverride{"global": map[string]any{
			"prePullImages": false,
			"metrics":       nil,
			"hostName":      "override.example.com",
			"imageTag":      "from-override",
		}}
		values := build(cfg, testBOM(nil))

		Expect(at(values, "global.prePullImages")).To(BeFalse())
		Expect(at(values, "global")).To(HaveKeyWithValue("metrics", BeNil()))
		Expect(at(values, "global.hostName")).To(Equal("override.example.com"))
		Expect(at(values, "global.imageTag")).To(Equal("v1.2.3"))
	})

	It("override of one workspace env var -> the others stay", func() {
		cfg := testConfig()
		cfg.Codesphere.ExtraWorkspaceEnvVars = map[string]string{"FOO": "foo", "BAR": "bar"}
		cfg.Codesphere.Override = files.ChartOverride{"global": map[string]any{
			"extraWorkspaceEnvVars": map[string]any{"FOO": "changed"},
		}}
		values := build(cfg, testBOM(nil))

		Expect(at(values, "global.extraWorkspaceEnvVars")).To(Equal(map[string]any{
			"FOO": "changed",
			"BAR": "bar",
		}))
	})

	It("override with numeric keys -> merged into the built map", func() {
		cfg := testConfig()
		cfg.Codesphere.Plans.HostingPlans = map[int]files.HostingPlan{
			20: {CPUTenth: 10, MemoryMb: 2048, StorageMb: 20480, TempStorageMb: 1500, PooledInstances: ptr.To(1)},
			21: {CPUTenth: 20, MemoryMb: 4096, StorageMb: 20480, TempStorageMb: 1500, PooledInstances: ptr.To(2)},
		}
		// YAML decodes the numeric pool keys of an override to a map with int keys.
		cfg.Codesphere.Override = files.ChartOverride{"global": map[string]any{
			"deployConfig": map[string]any{"images": map[string]any{"ubuntu-24.04": map[string]any{
				"flavors": map[string]any{"default": map[string]any{"pool": map[any]any{20: 5}}},
			}}},
		}}
		values := build(cfg, testBOM(nil))

		ubuntu := at(values, "global.deployConfig.images").(map[string]any)["ubuntu-24.04"]
		flavors := ubuntu.(map[string]any)["flavors"].(map[string]any)
		Expect(flavors["default"]).To(HaveKeyWithValue("pool", And(
			HaveKeyWithValue("20", 5),
			HaveKeyWithValue("21", 2),
		)))
	})

	It("override -> config is not changed through the values", func() {
		cfg := testConfig()
		cfg.Codesphere.Override = files.ChartOverride{"global": map[string]any{
			"ceph": map[string]any{"storageClass": "rbd"},
		}}
		values := build(cfg, testBOM(nil))
		at(values, "global.ceph").(map[string]any)["storageClass"] = "changed"

		globalOverride := cfg.Codesphere.Override["global"].(map[string]any)
		Expect(globalOverride["ceph"]).To(Equal(map[string]any{"storageClass": "rbd"}))
	})
})
