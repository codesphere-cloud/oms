// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package values_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	csappsvalues "github.com/codesphere-cloud/oms/internal/installer/csapps/values"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// The tests here are ported from the Codesphere installer's codesphere.test.ts with the same inputs
// (withInstallerOptions) and expectations, so a difference to the installer fails here.

// installerImageNames are the installer's codesphereImages.
var installerImageNames = []string{
	"auth-service", "deployment-service", "error-page-server", "ide-service",
	"integrations/github-action", "integrations/bitbucket-ci", "integrations/gitlab-ci",
	"marketplace", "mounter", "payment-service", "public-api-service", "team-service",
	"vm-vnc-proxy", "vpn", "workspace-agent-24.04", "workspace-agent-24.04.gpu",
	"workspace-agent-20.04", "workspace-agent-20.04.gpu", "workspace-agent", "workspace-agent-gpu",
	"workspace-agent-24.04-slim", "workspace-proxy", "workspace-service", "workspace-server",
	"ide-frontend", "custom-workspace-image-init", "workspace-router",
}

func installerBOM(overrides map[string]string) *bom.Config {
	images := map[string]string{
		"kubectl":               "alpine/kubectl:latest",
		"frontendGatewayNginx":  "nginx-cs-patched:latest",
		"nginx":                 "nginx:latest",
		"observabilityOtel":     "otel/opentelemetry-collector-contrib:latest",
		"observabilityOtelInit": "curl:latest",
		"opensearch":            "opensearch:latest",
		"opensearchInit":        "busybox:latest",
	}
	for _, name := range installerImageNames {
		images[strings.Replace(name, "/", "_", 1)] = name + ":latest"
	}

	for name, image := range overrides {
		if image == "" {
			delete(images, name)
			continue
		}

		images[name] = image
	}

	return &bom.Config{Components: map[string]bom.ComponentConfig{
		"codesphere": {ContainerImages: images},
	}}
}

func installerConfig() *files.RootConfig {
	config := files.NewRootConfig()
	// Data center 1, not the installer's 0: OMS rejects id 0.
	config.Datacenter = files.DatacenterConfig{ID: 1, Name: "test", City: "KA", CountryCode: "None"}
	config.Postgres = files.PostgresConfig{
		Mode:      "install",
		CACertPem: "ca",
		Primary:   &files.PostgresPrimaryConfig{IP: "1.2.3.4", Hostname: "foo"},
		Replica:   &files.PostgresReplicaConfig{IP: "1.2.3.5", Name: "bar"},
	}
	config.Ceph = files.CephConfig{
		NodesSubnet: "0.0.0.0/0",
		Hosts:       []files.CephHost{},
		OSDs:        []files.CephOSD{},
		Volumes:     map[string]files.CephVolume{"codesphere": {MDS: files.CephMDS{Active: 2}}},
	}
	config.Kubernetes = files.KubernetesConfig{PodCIDR: "1.1.1.1/1", ServiceCIDR: "2.2.2.2/2"}
	config.Cluster.Gateway = gateway(true, false)
	config.Cluster.PublicGateway = gateway(true, false)
	config.Codesphere = files.CodesphereConfig{
		Domain:                     "cs.internal",
		WorkspaceHostingBaseDomain: "ws.cs.internal",
		PublicIP:                   "0.0.0.0",
		DNSServers:                 []string{"8.8.8.8"},
		CustomDomains:              files.CustomDomainsConfig{CNameBaseDomain: "ws.example.com"},
		Plans: files.PlansConfig{
			HostingPlans:   map[int]files.HostingPlan{},
			WorkspacePlans: map[int]files.WorkspacePlan{},
		},
	}

	return &config
}

func gateway(enabled, use bool) files.GatewayConfig {
	return files.GatewayConfig{GatewayAPI: &files.GatewayAPIConfig{Enabled: enabled, Use: ptr.To(use)}}
}

func global(values map[string]any) map[string]any {
	return at(values, "global").(map[string]any)
}

var _ = Describe("Build, as the Codesphere installer's tests", func() {
	It("create values -> success", func() {
		values := global(build(installerConfig(), installerBOM(nil)))

		Expect(values["ceph"]).To(HaveKeyWithValue("activeMds", "2"))
		Expect(values["frontendGateway"]).To(Equal(map[string]any{
			"image":         map[string]any{"name": "nginx-cs-patched", "tag": "latest"},
			"issuer":        "self-signed",
			"redirectToIde": true,
			"gatewayApi":    map[string]any{"enabled": true},
		}))
		Expect(values["hosts"]).To(Equal([]any{"cs.internal"}))
		Expect(values["customDomains"]).To(Equal(map[string]any{
			"cNameBaseDomain": "ws.example.com",
			"gatewayApi":      map[string]any{"useGateway": false},
		}))
		Expect(values["networkPolicies"]).To(Equal(map[string]any{
			"workspace": map[string]any{
				"namespace":     "workspaces",
				"podSubnet":     "1.1.1.1/1",
				"serviceSubnet": "2.2.2.2/2",
			},
		}))
		Expect(values["gitProviders"]).To(Equal(map[string]any{
			"azureDevOps": map[string]any{"enabled": false},
			"bitbucket":   map[string]any{"enabled": false},
			"github":      map[string]any{"enabled": false},
			"gitlab":      map[string]any{"enabled": false},
			"scmManager":  map[string]any{"enabled": false},
		}))
		Expect(values["postgres"]).To(Equal(map[string]any{
			"database":           "codesphere",
			"host":               "1.2.3.4",
			"port":               5432,
			"rejectUnauthorized": true,
			"ssl":                map[string]any{"ca": "ca", "serverName": "foo"},
		}))
		Expect(values["vpn"]).To(Equal(map[string]any{}))
		Expect(values["workspaceImages"]).To(Equal(map[string]any{
			"server": "workspace-server",
			"vpn":    "vpn",
		}))
		Expect(values["workspaceRouter"]).To(Equal(map[string]any{
			"image":      map[string]any{"name": "nginx", "tag": "latest"},
			"haReplicas": 2,
		}))
	})

	It("workspace PVC storage enabled without Ceph config -> create values succeeds", func() {
		cfg := installerConfig()
		cfg.Codesphere.Internal = []string{"workspace-pvc-storage"}
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["ceph"]).To(HaveKeyWithValue("storageClass", "codesphere"))
		Expect(values["ceph"]).To(HaveKeyWithValue("activeMds", "0"))
		Expect(values["experiments"]).To(
			HaveKeyWithValue("workspace-pvc-storage", []any{"prod"}),
		)
	})

	It("SMTP is configured > create values > SMTP values are forwarded to the chart", func() {
		cfg := installerConfig()
		cfg.Codesphere.Email = &files.EmailConfig{
			Provider: "smtp",
			SMTP:     files.SMTPConfig{Host: "smtp.example.com", Port: 465, Secure: true, Username: "username"},
		}
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["smtp"]).To(Equal(map[string]any{
			"host":     "smtp.example.com",
			"port":     465,
			"secure":   true,
			"username": "username",
		}))
	})

	It("one data center -> only the target one is available", func() {
		values := global(build(installerConfig(), installerBOM(nil)))

		Expect(values["dataCenterId"]).To(Equal(1))
		Expect(values["defaultDataCenterId"]).To(Equal(1))
		Expect(values["availableDataCenters"]).To(Equal([]any{1}))
		Expect(values["dataCenters"]).To(Equal([]any{
			map[string]any{"id": 1, "name": "test", "city": "KA", "countryCode": "None"},
		}))
	})

	It("multiple data centers -> all of them are available", func() {
		cfg := installerConfig()
		cfg.DataCenters = []files.DatacenterConfig{
			{ID: 1, Name: "test", City: "KA", CountryCode: "None"},
			{ID: 2, Name: "test-dc2", City: "FRA", CountryCode: "DE"},
		}
		cfg.DefaultDataCenterID = 2
		values := global(build(cfg, installerBOM(nil)))

		// The target data center, the other one is remote.
		Expect(values["dataCenterId"]).To(Equal(1))
		Expect(values["defaultDataCenterId"]).To(Equal(2))
		Expect(values["availableDataCenters"]).To(Equal([]any{1, 2}))
		Expect(values["dataCenters"]).To(Equal([]any{
			map[string]any{"id": 1, "name": "test", "city": "KA", "countryCode": "None"},
			map[string]any{"id": 2, "name": "test-dc2", "city": "FRA", "countryCode": "DE"},
		}))
	})

	// The installer checks the rendered VPN_LEFT_PEER_ID, which the chart takes from this value.
	It("configure VPN left peer ID -> deployment receives VPN_LEFT_PEER_ID", func() {
		cfg := installerConfig()
		cfg.Codesphere.VPNLeftPeerID = "peer-id.example"
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["vpn"]).To(Equal(map[string]any{"leftPeerId": "peer-id.example"}))
	})

	It("overwrite workspace images -> success", func() {
		cfg := installerConfig()
		cfg.Codesphere.WorkspaceImages = &files.WorkspaceImagesConfig{
			Agent:    &files.ImageRef{ImageName: "agent"},
			AgentGpu: &files.ImageRef{ImageName: "agentGpu"},
			Server:   &files.ImageRef{ImageName: "server"},
			VPN:      &files.ImageRef{ImageName: "vpn"},
		}
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["workspaceImages"]).To(Equal(map[string]any{
			"server": "server",
			"vpn":    "vpn",
		}))
	})

	It("overwrite workspace images by bom -> success", func() {
		cfg := installerConfig()
		cfg.Codesphere.DeployConfig = files.DeployConfig{Images: map[string]files.ImageConfig{
			"uwu-24.04": {
				Name:           "UwU 24.04",
				SupportedUntil: "2025-05-31",
				Flavors: map[string]files.FlavorConfig{
					"default": {Image: files.ImageRef{BomRef: "workspace-agent-24.04-slim"}, Pool: map[int]int{}},
				},
			},
		}}
		cfg.Codesphere.WorkspaceImages = &files.WorkspaceImagesConfig{
			Agent: &files.ImageRef{BomRef: "workspace-agent-24.04"},
		}
		values := global(build(cfg, installerBOM(map[string]string{
			"workspace-agent-24.04":      "foo.bar:latest",
			"workspace-agent-24.04-slim": "bobby.babby:latest",
		})))

		images := values["deployConfig"].(map[string]any)["images"].(map[string]any)
		Expect(images).To(HaveLen(1))
		flavors := images["uwu-24.04"].(map[string]any)["flavors"].(map[string]any)
		Expect(flavors).To(HaveLen(1))
		Expect(flavors["default"]).To(HaveKeyWithValue("image", "bobby.babby"))
	})

	It("no deploy config set > construct based on legacy config", func() {
		cfg := installerConfig()
		cfg.Codesphere.Plans.HostingPlans = map[int]files.HostingPlan{
			101: {CPUTenth: 1, GPUParts: 0, MemoryMb: 1, StorageMb: 2, TempStorageMb: 3, PooledInstances: ptr.To(10)},
			777: {CPUTenth: 1, GPUParts: 1, MemoryMb: 1, StorageMb: 2, TempStorageMb: 3, PooledInstances: ptr.To(223)},
		}
		cfg.Codesphere.WorkspaceImages = &files.WorkspaceImagesConfig{
			Agent:    &files.ImageRef{BomRef: "workspace-agent-24.04"},
			AgentGpu: &files.ImageRef{ImageName: "bar"},
		}
		b := installerBOM(map[string]string{"workspace-agent-24.04": "foo-experimental:latest"})
		values := global(build(cfg, b))

		images := values["deployConfig"].(map[string]any)["images"].(map[string]any)
		Expect(images).To(HaveLen(1))
		Expect(images["ubuntu-24.04"]).To(HaveKeyWithValue("flavors", map[string]any{
			"default": map[string]any{
				"image": "foo-experimental",
				"pool":  map[string]any{"101": 10},
			},
			"gpu": map[string]any{"image": "bar", "pool": map[string]any{"777": 223}},
		}))
	})

	It("provide deployConfig AND pooledInstances > throws", func() {
		cfg := installerConfig()
		cfg.Codesphere.Plans.HostingPlans = map[int]files.HostingPlan{
			101: {CPUTenth: 1, GPUParts: 0, MemoryMb: 1, StorageMb: 2, TempStorageMb: 3, PooledInstances: ptr.To(10)},
		}
		cfg.Codesphere.DeployConfig = files.DeployConfig{Images: map[string]files.ImageConfig{}}
		_, err := csappsvalues.Build(cfg, installerBOM(nil))

		Expect(err).To(HaveOccurred())
	})

	It("gpu image not set -> success", func() {
		b := installerBOM(map[string]string{"workspace-agent-gpu": ""})
		values := global(build(installerConfig(), b))

		Expect(values["workspaceImages"]).To(Equal(map[string]any{
			"server": "workspace-server",
			"vpn":    "vpn",
		}))
	})

	It("external postgres -> success", func() {
		cfg := installerConfig()
		cfg.Postgres = files.PostgresConfig{
			CACertPem:     "foo",
			ServerAddress: "example.com",
			Port:          1234,
		}
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["postgres"]).To(Equal(map[string]any{
			"database":           "codesphere",
			"host":               "example.com",
			"port":               1234,
			"rejectUnauthorized": true,
			"ssl":                map[string]any{"ca": "foo", "serverName": "example.com"},
		}))
	})

	// The installer also checks that the rendered Secret has the client credentials. These values
	// hold no secrets, so only the values part is ported.
	It("configure oidc -> success", func() {
		cfg := installerConfig()
		cfg.Codesphere.OAuth = &files.OAuthProvidersConfig{Oidc: &files.OidcOAuthProvider{
			Enabled:   true,
			Type:      "oidc",
			Name:      "fooIdp",
			IssuerURL: "http://idp.example.com",
			Scopes:    []string{"abc"},
		}}
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["oauth"]).To(Equal(map[string]any{
			"providers": map[string]any{
				"bitbucket": map[string]any{"enabled": false},
				"github":    map[string]any{"enabled": false},
				"gitlab":    map[string]any{"enabled": false},
				"google":    map[string]any{"enabled": false},
			},
			"additionalProviders": map[string]any{
				"fooIdp": map[string]any{
					"enabled":   true,
					"type":      "oidc",
					"name":      "fooIdp",
					"issuerUrl": "http://idp.example.com",
					"scopes":    []any{"abc"},
				},
			},
		}))
	})

	It("configure load balancer -> success", func() {
		cfg := installerConfig()
		cfg.Codesphere.IPService = &files.IPServiceConfig{LoadBalancerKind: "metallb", AddressPools: []string{"foo", "bar"}}
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["ipService"]).To(Equal(map[string]any{
			"loadBalancerKind": "metallb",
			"addressPools":     []any{"foo", "bar"},
		}))
	})

	It("configure gitProviders -> success", func() {
		providerConfig := &files.GitProviderConfig{
			Enabled: true,
			URL:     "http://example.org",
			API:     files.APIConfig{BaseURL: "https://api.example.org/2.0"},
			OAuth: files.OAuthConfig{
				Issuer:                "https://example.org",
				AuthorizationEndpoint: "https://example.org/site/oauth2/authorize",
				TokenEndpoint:         "https://example.org/access_token",
				ClientAuthMethod:      "client_secret_post",
			},
		}
		cfg := installerConfig()
		cfg.Codesphere.GitProviders = &files.GitProvidersConfig{
			AzureDevOps: providerConfig,
			Bitbucket:   providerConfig,
			SCMManager: &files.SCMManagerConfig{
				Enabled: true,
				URL:     "http://scm.example.org",
				API:     &files.APIConfig{BaseURL: "https://api.scm.example.org/2.0"},
			},
		}
		values := global(build(cfg, installerBOM(nil)))

		provider := map[string]any{
			"enabled": true,
			"url":     "http://example.org",
			"api":     map[string]any{"baseUrl": "https://api.example.org/2.0"},
			"oauth": map[string]any{
				"issuer":                "https://example.org",
				"authorizationEndpoint": "https://example.org/site/oauth2/authorize",
				"tokenEndpoint":         "https://example.org/access_token",
				"clientAuthMethod":      "client_secret_post",
			},
		}
		Expect(values["gitProviders"]).To(Equal(map[string]any{
			"azureDevOps": provider,
			"bitbucket":   provider,
			"github":      map[string]any{"enabled": false},
			"gitlab":      map[string]any{"enabled": false},
			"scmManager": map[string]any{
				"enabled": true,
				"url":     "http://scm.example.org",
				"api":     map[string]any{"baseUrl": "https://api.scm.example.org/2.0"},
			},
		}))
	})

	It("extra experiments -> success", func() {
		cfg := installerConfig()
		cfg.Codesphere.Experiments = []string{"foo", "bar"} //nolint:staticcheck
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["experiments"]).To(Equal(map[string]any{
			"foo":             []any{"prod"},
			"bar":             []any{"prod"},
			"hermetic":        []any{"prod"},
			"persistent-nix":  []any{"prod"},
			"gateway-domains": []any{"prod"},
		}))
	})

	It("extra features -> success", func() {
		cfg := installerConfig()
		cfg.Codesphere.Features = map[string]bool{"foo": true, "bar": true}
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["features"]).To(Equal(map[string]any{
			"email-signup":     []any{"prod"},
			"email-signin":     []any{"prod"},
			"standalone-teams": []any{"prod"},
			"billing":          []any{},
			"foo":              []any{"prod"},
			"bar":              []any{"prod"},
		}))
	})

	// The installer checks the rendered Issuer and EAB Secret. With external secrets, the chart
	// reads the key id and the vault entry of the MAC key from these values.
	It("use acme with eab -> expect enabled", func() {
		cfg := installerConfig()
		cfg.Codesphere.CertIssuer = &files.CertIssuerConfig{Type: files.CertIssuerTypeACME, Acme: &files.ACMEConfig{
			Server:   "https://acme.corp42.test/v2/directory",
			Email:    "test@corp42.test",
			EABKeyID: "dummykeyid",
		}}
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["certIssuer"]).To(Equal(map[string]any{"acme": map[string]any{
			"enabled":       true,
			"server":        "https://acme.corp42.test/v2/directory",
			"email":         "test@corp42.test",
			"eabKeyId":      "dummykeyid",
			"eabMacKeyName": "acmeEabMacKey",
		}}))
	})

	It("use acme with separate custom domains eab -> expect custom domains eab used", func() {
		cfg := installerConfig()
		cfg.Codesphere.CertIssuer = &files.CertIssuerConfig{Type: files.CertIssuerTypeACME, Acme: &files.ACMEConfig{
			Server:                "https://acme.corp42.test/v2/directory",
			Email:                 "test@corp42.test",
			EABKeyID:              "clusterissuerkeyid",
			CustomDomainsEABKeyID: "customdomainskeyid",
		}}
		values := global(build(cfg, installerBOM(nil)))

		// The custom-domain Issuer must use its own key id, not the ClusterIssuer's.
		acme := values["certIssuer"].(map[string]any)["acme"]
		Expect(acme).To(HaveKeyWithValue("eabKeyId", "customdomainskeyid"))
		Expect(acme).To(HaveKeyWithValue("eabMacKeyName", "acmeCustomDomainsEabMacKey"))
	})

	It("no certIssuer -> expect self-signed issuer", func() {
		values := global(build(installerConfig(), installerBOM(nil)))

		Expect(values["certIssuer"]).To(Equal(map[string]any{
			"acme": map[string]any{"enabled": false},
		}))
		Expect(values["frontendGateway"]).To(HaveKeyWithValue("issuer", "self-signed"))
	})

	It("certIssuer type self-signed -> expect self-signed issuer", func() {
		cfg := installerConfig()
		cfg.Codesphere.CertIssuer = &files.CertIssuerConfig{Type: files.CertIssuerTypeSelfSigned}
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["certIssuer"]).To(Equal(map[string]any{
			"acme": map[string]any{"enabled": false},
		}))
		Expect(values["frontendGateway"]).To(HaveKeyWithValue("issuer", "self-signed"))
	})

	It("use acme without eab -> expect acme issuer without eab", func() {
		cfg := installerConfig()
		cfg.Codesphere.CertIssuer = &files.CertIssuerConfig{Type: files.CertIssuerTypeACME, Acme: &files.ACMEConfig{
			Server: "https://acme.corp42.test/v2/directory",
			Email:  "test@corp42.test",
		}}
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["certIssuer"]).To(Equal(map[string]any{"acme": map[string]any{
			"enabled": true,
			"server":  "https://acme.corp42.test/v2/directory",
			"email":   "test@corp42.test",
		}}))
	})

	It("hermetic is enabled by default", func() {
		values := global(build(installerConfig(), installerBOM(nil)))

		Expect(values["experiments"]).To(HaveKeyWithValue("hermetic", []any{"prod"}))
	})

	It("override experiments -> success", func() {
		cfg := installerConfig()
		cfg.Codesphere.Override = files.ChartOverride{"global": map[string]any{
			"experiments": map[string]any{"hermetic": []any{}},
		}}
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["experiments"]).To(HaveKeyWithValue("hermetic", []any{}))
	})

	It("no telemetryExport configured -> all otel values default to disabled", func() {
		values := global(build(installerConfig(), installerBOM(nil)))

		Expect(values["otel"]).To(Equal(map[string]any{
			"centralOtelExport": map[string]any{
				"enabled": false,
				"telemetryCredentials": map[string]any{
					"secretName": "otel-collector-central-creds",
				},
				"otlpHttpEndpoint": "https://otel.tools.codesphere.com",
			},
			"telemetryExport": map[string]any{
				"spanMetrics": map[string]any{"enabled": false},
				"traces":      map[string]any{"enabled": false},
			},
		}))
	})

	It("all telemetryExport options set -> all otel values mapped correctly", func() {
		cfg := installerConfig()
		cfg.Codesphere.TelemetryExport = &files.TelemetryExport{
			RemoteExport:   true,
			RemoteEndpoint: "https://custom.otel.example.com",
			SpanMetrics:    true,
			Traces:         true,
			TraceEndpoint:  "tempo.custom.svc.cluster.local:4317",
		}
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["otel"]).To(Equal(map[string]any{
			"centralOtelExport": map[string]any{
				"enabled": true,
				"telemetryCredentials": map[string]any{
					"secretName": "otel-collector-central-creds",
				},
				"otlpHttpEndpoint": "https://custom.otel.example.com",
			},
			"telemetryExport": map[string]any{
				"spanMetrics": map[string]any{"enabled": true},
				"traces": map[string]any{
					"enabled":  true,
					"endpoint": "tempo.custom.svc.cluster.local:4317",
				},
			},
		}))
	})

	// The installer checks the rendered Gateway, which the chart creates from this value.
	It("enable codesphere-gateway > gateway created in frontend-gateway-values", func() {
		cfg := installerConfig()
		cfg.Cluster.Gateway = gateway(true, false)
		cfg.Cluster.PublicGateway = gateway(false, false)
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["frontendGateway"]).To(
			HaveKeyWithValue("gatewayApi", map[string]any{"enabled": true}),
		)
	})

	It("enable workspaces-gateway > enables experiment", func() {
		cfg := installerConfig()
		cfg.Cluster.Gateway = gateway(false, false)
		cfg.Cluster.PublicGateway = gateway(true, false)
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["experiments"]).To(HaveKeyWithValue("gateway-domains", []any{"prod"}))
	})

	// The installer checks the rendered Issuer's solver, which the chart picks from this value.
	It("enable and use workspaces-gateway > sets issuer solver to gateway", func() {
		cfg := installerConfig()
		cfg.Cluster.Gateway = gateway(false, false)
		cfg.Cluster.PublicGateway = gateway(true, true)
		cfg.Codesphere.CertIssuer = &files.CertIssuerConfig{
			Type: files.CertIssuerTypeACME,
			Acme: &files.ACMEConfig{Server: "acme.test", Email: "cert@acme.test"},
		}
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["customDomains"]).To(
			HaveKeyWithValue("gatewayApi", map[string]any{"useGateway": true}),
		)
	})
})
