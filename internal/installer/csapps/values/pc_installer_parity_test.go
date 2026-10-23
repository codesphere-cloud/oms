// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package values_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

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

func installerConfig(yamlConfig string) *files.RootConfig {
	// Data center 1, not the installer's 0: OMS rejects id 0.
	base := `
		dataCenter: {id: 1, name: test, city: KA, countryCode: None}
		postgres:
		  mode: install
		  caCertPem: ca
		  primary: {ip: 1.2.3.4, hostname: foo, sslConfig: {serverCertPem: ""}}
		  replica: {ip: 1.2.3.5, name: bar, sslConfig: {serverCertPem: ""}}
		ceph:
		  cephAdmSshKey: {publicKey: ""}
		  nodesSubnet: 0.0.0.0/0
		  hosts: []
		  osds: []
		  volumes: {codesphere: {mds: {active: 2}}}
		kubernetes: {managedByCodesphere: false, podCidr: 1.1.1.1/1, serviceCidr: 2.2.2.2/2}
		cluster:
		  gateway: {gatewayApi: {enabled: true, use: false}}
		  publicGateway: {gatewayApi: {enabled: true, use: false}}
		codesphere:
		  domain: cs.internal
		  workspaceHostingBaseDomain: ws.cs.internal
		  publicIp: 0.0.0.0
		  dnsServers: [8.8.8.8]
		  customDomains: {cNameBaseDomain: ws.example.com}
		  plans: {hostingPlans: {}, workspacePlans: {}}
	`
	config := files.NewRootConfig()
	Expect(config.Unmarshal([]byte(dedent(base)))).To(Succeed())
	Expect(yaml.Unmarshal([]byte(dedent(yamlConfig)), &config)).To(Succeed())

	return &config
}

func global(values map[string]any) map[string]any {
	return at(values, "global").(map[string]any)
}

var _ = Describe("Build, as the Codesphere installer's tests", func() {
	It("create values -> success", func() {
		values := global(build(installerConfig(""), installerBOM(nil)))

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
		cfg := installerConfig(`codesphere: {internal: [workspace-pvc-storage]}`)
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["ceph"]).To(HaveKeyWithValue("storageClass", "codesphere"))
		Expect(values["ceph"]).To(HaveKeyWithValue("activeMds", "0"))
		Expect(values["experiments"]).To(
			HaveKeyWithValue("workspace-pvc-storage", []any{"prod"}),
		)
	})

	It("SMTP is configured > create values > SMTP values are forwarded to the chart", func() {
		values := global(build(installerConfig(`
			codesphere:
			  email:
			    provider: smtp
			    smtp: {host: smtp.example.com, port: 465, secure: true, username: username}
		`), installerBOM(nil)))

		Expect(values["smtp"]).To(Equal(map[string]any{
			"host":     "smtp.example.com",
			"port":     465,
			"secure":   true,
			"username": "username",
		}))
	})

	It("one data center -> only the target one is available", func() {
		values := global(build(installerConfig(""), installerBOM(nil)))

		Expect(values["dataCenterId"]).To(Equal(1))
		Expect(values["defaultDataCenterId"]).To(Equal(1))
		Expect(values["availableDataCenters"]).To(Equal([]any{1}))
		Expect(values["dataCenters"]).To(Equal([]any{
			map[string]any{"id": 1, "name": "test", "city": "KA", "countryCode": "None"},
		}))
	})

	It("multiple data centers -> all of them are available", func() {
		values := global(build(installerConfig(`
			dataCenters:
			  - {id: 1, name: test, city: KA, countryCode: None}
			  - {id: 2, name: test-dc2, city: FRA, countryCode: DE}
			defaultDataCenterId: 2
		`), installerBOM(nil)))

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
		cfg := installerConfig(`codesphere: {vpnLeftPeerId: peer-id.example}`)
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["vpn"]).To(Equal(map[string]any{"leftPeerId": "peer-id.example"}))
	})

	It("overwrite workspace images -> success", func() {
		values := global(build(installerConfig(`
			codesphere:
			  workspaceImages: {agent: agent, agentGpu: agentGpu, server: server, vpn: vpn}
		`), installerBOM(nil)))

		Expect(values["workspaceImages"]).To(Equal(map[string]any{
			"server": "server",
			"vpn":    "vpn",
		}))
	})

	It("overwrite workspace images by bom -> success", func() {
		values := global(build(installerConfig(`
			codesphere:
			  deployConfig:
			    images:
			      uwu-24.04:
			        name: UwU 24.04
			        supportedUntil: "2025-05-31"
			        flavors:
			          default: {image: {bomRef: workspace-agent-24.04-slim}, pool: {}}
			  workspaceImages:
			    agent: {bomRef: workspace-agent-24.04}
		`), installerBOM(map[string]string{
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
		values := global(build(installerConfig(`
			codesphere:
			  plans:
			    workspacePlans: {}
			    hostingPlans:
			      101:
			        cpuTenth: 1
			        gpuParts: 0
			        memoryMb: 1
			        storageMb: 2
			        tempStorageMb: 3
			        pooledInstances: 10
			      777:
			        cpuTenth: 1
			        gpuParts: 1
			        memoryMb: 1
			        storageMb: 2
			        tempStorageMb: 3
			        pooledInstances: 223
			  workspaceImages:
			    agent: {bomRef: workspace-agent-24.04}
			    agentGpu: bar
		`), installerBOM(map[string]string{"workspace-agent-24.04": "foo-experimental:latest"})))

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
		_, err := csappsvalues.Build(installerConfig(`
			codesphere:
			  plans:
			    workspacePlans: {}
			    hostingPlans:
			      101:
			        cpuTenth: 1
			        gpuParts: 0
			        memoryMb: 1
			        storageMb: 2
			        tempStorageMb: 3
			        pooledInstances: 10
			  deployConfig: {images: {}}
		`), installerBOM(nil))

		Expect(err).To(HaveOccurred())
	})

	It("gpu image not set -> success", func() {
		b := installerBOM(map[string]string{"workspace-agent-gpu": ""})
		values := global(build(installerConfig(""), b))

		Expect(values["workspaceImages"]).To(Equal(map[string]any{
			"server": "workspace-server",
			"vpn":    "vpn",
		}))
	})

	It("external postgres -> success", func() {
		cfg := installerConfig("")
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
		values := global(build(installerConfig(`
			codesphere:
			  oauth:
			    oidc:
			      enabled: true
			      type: oidc
			      name: fooIdp
			      issuerUrl: "http://idp.example.com"
			      scopes: [abc]
		`), installerBOM(nil)))

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
		values := global(build(installerConfig(`
			codesphere:
			  ipService: {loadBalancerKind: metallb, addressPools: [foo, bar]}
		`), installerBOM(nil)))

		Expect(values["ipService"]).To(Equal(map[string]any{
			"loadBalancerKind": "metallb",
			"addressPools":     []any{"foo", "bar"},
		}))
	})

	It("configure gitProviders -> success", func() {
		values := global(build(installerConfig(`
			codesphere:
			  gitProviders:
			    azureDevOps: &provider
			      enabled: true
			      url: "http://example.org"
			      api: {baseUrl: "https://api.example.org/2.0"}
			      oauth:
			        issuer: "https://example.org"
			        authorizationEndpoint: "https://example.org/site/oauth2/authorize"
			        tokenEndpoint: "https://example.org/access_token"
			        clientAuthMethod: client_secret_post
			        scope: ""
			    bitbucket: *provider
			    scmManager:
			      enabled: true
			      url: "http://scm.example.org"
			      api: {baseUrl: "https://api.scm.example.org/2.0"}
		`), installerBOM(nil)))

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
		cfg := installerConfig(`codesphere: {experiments: [foo, bar]}`)
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
		cfg := installerConfig(`codesphere: {features: {foo: true, bar: true}}`)
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
		values := global(build(installerConfig(`
			codesphere:
			  certIssuer:
			    type: acme
			    acme:
			      server: "https://acme.corp42.test/v2/directory"
			      email: test@corp42.test
			      eabKeyId: dummykeyid
		`), installerBOM(nil)))

		Expect(values["certIssuer"]).To(Equal(map[string]any{"acme": map[string]any{
			"enabled":       true,
			"server":        "https://acme.corp42.test/v2/directory",
			"email":         "test@corp42.test",
			"eabKeyId":      "dummykeyid",
			"eabMacKeyName": "acmeEabMacKey",
		}}))
	})

	It("use acme with separate custom domains eab -> expect custom domains eab used", func() {
		values := global(build(installerConfig(`
			codesphere:
			  certIssuer:
			    type: acme
			    acme:
			      server: "https://acme.corp42.test/v2/directory"
			      email: test@corp42.test
			      eabKeyId: clusterissuerkeyid
			      customDomainsEabKeyId: customdomainskeyid
		`), installerBOM(nil)))

		// The custom-domain Issuer must use its own key id, not the ClusterIssuer's.
		acme := values["certIssuer"].(map[string]any)["acme"]
		Expect(acme).To(HaveKeyWithValue("eabKeyId", "customdomainskeyid"))
		Expect(acme).To(HaveKeyWithValue("eabMacKeyName", "acmeCustomDomainsEabMacKey"))
	})

	It("no certIssuer -> expect self-signed issuer", func() {
		values := global(build(installerConfig(""), installerBOM(nil)))

		Expect(values["certIssuer"]).To(Equal(map[string]any{
			"acme": map[string]any{"enabled": false},
		}))
		Expect(values["frontendGateway"]).To(HaveKeyWithValue("issuer", "self-signed"))
	})

	It("certIssuer type self-signed -> expect self-signed issuer", func() {
		cfg := installerConfig(`codesphere: {certIssuer: {type: self-signed}}`)
		values := global(build(cfg, installerBOM(nil)))

		Expect(values["certIssuer"]).To(Equal(map[string]any{
			"acme": map[string]any{"enabled": false},
		}))
		Expect(values["frontendGateway"]).To(HaveKeyWithValue("issuer", "self-signed"))
	})

	It("use acme without eab -> expect acme issuer without eab", func() {
		values := global(build(installerConfig(`
			codesphere:
			  certIssuer:
			    type: acme
			    acme: {server: "https://acme.corp42.test/v2/directory", email: test@corp42.test}
		`), installerBOM(nil)))

		Expect(values["certIssuer"]).To(Equal(map[string]any{"acme": map[string]any{
			"enabled": true,
			"server":  "https://acme.corp42.test/v2/directory",
			"email":   "test@corp42.test",
		}}))
	})

	It("hermetic is enabled by default", func() {
		values := global(build(installerConfig(""), installerBOM(nil)))

		Expect(values["experiments"]).To(HaveKeyWithValue("hermetic", []any{"prod"}))
	})

	It("override experiments -> success", func() {
		values := global(build(installerConfig(`
			codesphere:
			  override:
			    global:
			      experiments: {hermetic: []}
		`), installerBOM(nil)))

		Expect(values["experiments"]).To(HaveKeyWithValue("hermetic", []any{}))
	})

	It("no telemetryExport configured -> all otel values default to disabled", func() {
		values := global(build(installerConfig(""), installerBOM(nil)))

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
		values := global(build(installerConfig(`
			codesphere:
			  telemetryExport:
			    remoteExport: true
			    remoteEndpoint: "https://custom.otel.example.com"
			    spanMetrics: true
			    traces: true
			    traceEndpoint: "tempo.custom.svc.cluster.local:4317"
		`), installerBOM(nil)))

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
		values := global(build(installerConfig(`
			cluster:
			  gateway: {gatewayApi: {enabled: true, use: false}}
			  publicGateway: {gatewayApi: {enabled: false, use: false}}
		`), installerBOM(nil)))

		Expect(values["frontendGateway"]).To(
			HaveKeyWithValue("gatewayApi", map[string]any{"enabled": true}),
		)
	})

	It("enable workspaces-gateway > enables experiment", func() {
		values := global(build(installerConfig(`
			cluster:
			  gateway: {gatewayApi: {enabled: false, use: false}}
			  publicGateway: {gatewayApi: {enabled: true, use: false}}
		`), installerBOM(nil)))

		Expect(values["experiments"]).To(HaveKeyWithValue("gateway-domains", []any{"prod"}))
	})

	// The installer checks the rendered Issuer's solver, which the chart picks from this value.
	It("enable and use workspaces-gateway > sets issuer solver to gateway", func() {
		values := global(build(installerConfig(`
			cluster:
			  gateway: {gatewayApi: {enabled: false, use: false}}
			  publicGateway: {gatewayApi: {enabled: true, use: true}}
			codesphere:
			  certIssuer: {type: acme, acme: {server: acme.test, email: cert@acme.test}}
		`), installerBOM(nil)))

		Expect(values["customDomains"]).To(
			HaveKeyWithValue("gatewayApi", map[string]any{"useGateway": true}),
		)
	})
})
