// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codespherevalues_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/codespherevalues"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const imagePrefix = "ghcr.io/codesphere-cloud/codesphere-monorepo/"

func testBOM(extraImages map[string]string) *bom.Config {
	images := map[string]string{
		"auth-service":          imagePrefix + "auth-service:v1.2.3",
		"deployment-service":    imagePrefix + "deployment-service:v1.2.3",
		"error-page-server":     imagePrefix + "error-page-server:v1.2.3",
		"ide-frontend":          imagePrefix + "ide-frontend:v1.2.3",
		"ide-service":           imagePrefix + "ide-service:v1.2.3",
		"marketplace":           imagePrefix + "marketplace:v1.2.3",
		"payment-service":       imagePrefix + "payment-service:v1.2.3",
		"public-api-service":    imagePrefix + "public-api-service:v1.2.3",
		"team-service":          imagePrefix + "team-service:v1.2.3",
		"workspace-proxy":       imagePrefix + "workspace-proxy:v1.2.3",
		"workspace-service":     imagePrefix + "workspace-service:v1.2.3",
		"workspace-agent-24.04": imagePrefix + "workspace-agent-24.04:v1.2.3",
		"workspace-agent":       imagePrefix + "workspace-agent:v1.2.3",
		"workspace-server":      imagePrefix + "workspace-server:v1.2.3",
		"vpn":                   imagePrefix + "vpn:v1.2.3",
		"nginx":                 "ghcr.io/codesphere-cloud/docker/nginx:1.31.5",
		"frontendGatewayNginx":  "ghcr.io/codesphere-cloud/docker/nginx-gateway:1.31.5",
		"opensearch":            "ghcr.io/codesphere-cloud/docker/opensearch:3.8.0",
		"opensearchInit":        "ghcr.io/codesphere-cloud/docker/busybox:1.37.0",
		"kubectl":               "ghcr.io/codesphere-cloud/docker/kubectl:1.34.0",
		"observabilityOtel":     "ghcr.io/codesphere-cloud/docker/otel:0.160.0",
		"observabilityOtelInit": "ghcr.io/codesphere-cloud/docker/curl:8.20.0",
	}
	for name, image := range extraImages {
		images[name] = image
	}

	return &bom.Config{Components: map[string]bom.ComponentConfig{"codesphere": {ContainerImages: images}}}
}

func testConfig(yamlConfig string) *files.RootConfig {
	base := `
dataCenter: {id: 1, name: main, city: Karlsruhe, countryCode: DE}
postgres: {serverAddress: postgres.example.com}
kubernetes: {managedByCodesphere: false, podCidr: 10.1.0.0/16, serviceCidr: 10.2.0.0/16}
codesphere:
  domain: codesphere.example.com
  workspaceHostingBaseDomain: ws.example.com
  publicIp: 203.0.113.10
  customDomains: {cNameBaseDomain: custom.example.com}
  plans:
    hostingPlans:
      8: {cpuTenth: 10, gpuParts: 0, memoryMb: 2048, storageMb: 20480, tempStorageMb: 1500}
    workspacePlans: {}
`
	config := files.NewRootConfig()
	Expect(config.Unmarshal([]byte(base))).To(Succeed())
	Expect(yaml.Unmarshal([]byte(yamlConfig), &config)).To(Succeed())

	return &config
}

func build(cfg *files.RootConfig, b *bom.Config) map[string]any {
	values, err := codespherevalues.Build(cfg, b)
	Expect(err).NotTo(HaveOccurred())

	return values
}

// at returns the value at a dot-separated path.
func at(values map[string]any, path string) any {
	var current any = values
	for _, key := range strings.Split(path, ".") {
		m, ok := current.(map[string]any)
		Expect(ok).To(BeTrue(), "%s is not a map at %s", path, key)
		current, ok = m[key]
		Expect(ok).To(BeTrue(), "%s is missing %s", path, key)
	}

	return current
}

// deployImage returns a deploy config image, whose keys contain dots and so do not work with at.
func deployImage(values map[string]any, key string) map[string]any {
	images := at(values, "global.deployConfig.images").(map[string]any)
	Expect(images).To(HaveKey(key))

	return images[key].(map[string]any)
}

var _ = Describe("Build", func() {
	It("minimal config -> installer defaults", func() {
		values := build(testConfig(""), testBOM(nil))

		Expect(at(values, "global.dnsServers")).To(Equal([]string{"8.8.8.8"}))
		Expect(at(values, "global.prePullImages")).To(BeTrue())
		Expect(at(values, "global.managedServiceWorkspacePlanId")).To(Equal(20))
		Expect(at(values, "global.ceph.mdsNamespace")).To(Equal("codesphere"))
		Expect(at(values, "global.ceph.activeMds")).To(Equal("7"))
		Expect(at(values, "global.otel.centralOtelExport.otlpHttpEndpoint")).To(Equal("https://otel.tools.codesphere.com"))
		Expect(at(values, "global.frontendGateway.issuer")).To(Equal("self-signed"))
		Expect(at(values, "global.frontendGateway.gatewayApi.enabled")).To(BeTrue())
		Expect(at(values, "global.customDomains.gatewayApi.useGateway")).To(BeTrue())
		Expect(at(values, "global.experiments")).To(HaveKey("gateway-domains"))
		Expect(at(values, "global.certIssuer")).To(Equal(map[string]any{"acme": map[string]any{"enabled": false}}))
		Expect(at(values, "global.imageTag")).To(Equal("v1.2.3"))
		Expect(at(values, "global.hostName")).To(Equal("codesphere.example.com"))
	})

	It("minimal config -> default Ubuntu image from the BOM, pooled per plan", func() {
		values := build(testConfig(`
codesphere:
  plans:
    hostingPlans:
      8: {cpuTenth: 10, gpuParts: 0, memoryMb: 2048, storageMb: 20480, tempStorageMb: 1500, pooledInstances: 2}
`), testBOM(nil))

		Expect(at(values, "global.deployConfig")).To(Equal(map[string]any{
			"images": map[string]any{
				"ubuntu-24.04": map[string]any{
					"name":           "Ubuntu 24.04",
					"supportedUntil": "2028-05-31T00:00:00.000Z",
					"flavors": map[string]any{
						"default": map[string]any{
							"image": imagePrefix + "workspace-agent-24.04",
							"pool":  map[string]any{"8": 2},
						},
					},
				},
			},
		}))
		Expect(at(values, "global.poolDefinition")).To(Equal(map[string]any{"8": 2}))
	})

	It("BOM with a GPU agent -> gpu flavor pools only the GPU plans", func() {
		values := build(testConfig(`
codesphere:
  plans:
    hostingPlans:
      8: {cpuTenth: 10, gpuParts: 0, memoryMb: 2048, storageMb: 20480, tempStorageMb: 1500}
      9: {cpuTenth: 10, gpuParts: 1, memoryMb: 2048, storageMb: 20480, tempStorageMb: 1500, pooledInstances: 1}
`), testBOM(map[string]string{"workspace-agent-24.04.gpu": imagePrefix + "workspace-agent-24.04-gpu:v1.2.3"}))

		flavors := deployImage(values, "ubuntu-24.04")["flavors"].(map[string]any)
		Expect(flavors["default"]).To(HaveKeyWithValue("pool", map[string]any{"8": 0}))
		Expect(flavors["gpu"]).To(Equal(map[string]any{
			"image": imagePrefix + "workspace-agent-24.04-gpu",
			"pool":  map[string]any{"9": 1},
		}))
	})

	It("configured deploy config -> only the default and gpu flavors", func() {
		values := build(testConfig(`
codesphere:
  deployConfig:
    images:
      ubuntu-22.04:
        name: Ubuntu 22.04
        supportedUntil: "2027-04-01"
        default: true
        flavors:
          default: {image: {bomRef: workspace-agent-24.04}, pool: {8: 1}}
          minimal: {image: {bomRef: workspace-agent-24.04}, pool: {8: 1}}
`), testBOM(nil))

		image := deployImage(values, "ubuntu-22.04")
		Expect(image["default"]).To(BeTrue())
		Expect(image["supportedUntil"]).To(Equal("2027-04-01T00:00:00.000Z"))
		Expect(image["flavors"]).To(HaveLen(1))
		Expect(image["flavors"]).To(HaveKey("default"))
	})

	It("registry that replaces BOM images -> every image moves onto the registry", func() {
		values := build(testConfig(`registry: {server: registry.example.com:5000}`), testBOM(nil))

		Expect(at(values, "global.services.auth_service.image")).To(Equal("registry.example.com:5000/ghcr_io/codesphere-cloud/codesphere-monorepo/auth-service"))
		Expect(at(values, "global.workspaceRouter.image")).To(Equal(map[string]any{
			"name": "registry.example.com:5000/ghcr_io/codesphere-cloud/docker/nginx",
			"tag":  "1.31.5",
		}))
	})

	It("registry that keeps BOM images -> images unchanged", func() {
		values := build(testConfig(`registry: {server: registry.example.com, replaceImagesInBom: false}`), testBOM(nil))

		Expect(at(values, "global.services.auth_service.image")).To(Equal(imagePrefix + "auth-service"))
	})

	It("BOM image with a registry port -> port kept, tag split off", func() {
		values := build(testConfig(""), testBOM(map[string]string{"nginx": "localhost:5000/nginx:1.31.5"}))

		Expect(at(values, "global.workspaceRouter.image")).To(Equal(map[string]any{"name": "localhost:5000/nginx", "tag": "1.31.5"}))
	})

	It("flavor with a Dockerfile and a registry -> image built from it", func() {
		values := build(testConfig(`
registry: {server: registry.example.com, replaceImagesInBom: false}
codesphere:
  workspaceImages:
    agent: {bomRef: workspace-agent-24.04, dockerfile: Dockerfile.agent}
`), testBOM(nil))

		defaultFlavor := deployImage(values, "ubuntu-24.04")["flavors"].(map[string]any)["default"]
		Expect(defaultFlavor).To(HaveKeyWithValue("image", "registry.example.com/ubuntu-24.04-default"))
	})

	It("override -> wins over the installer's values, null removes a key", func() {
		values := build(testConfig(`
codesphere:
  override:
    global:
      prePullImages: false
      metrics: null
      hostName: override.example.com
      imageTag: from-override
`), testBOM(nil))

		Expect(at(values, "global.prePullImages")).To(BeFalse())
		Expect(at(values, "global")).NotTo(HaveKey("metrics"))
		Expect(at(values, "global.hostName")).To(Equal("override.example.com"))
		Expect(at(values, "global.imageTag")).To(Equal("v1.2.3"))
	})

	It("override of one workspace env var -> the others stay", func() {
		values := build(testConfig(`
codesphere:
  extraWorkspaceEnvVars: {FOO: foo, BAR: bar}
  override:
    global:
      extraWorkspaceEnvVars: {FOO: changed}
`), testBOM(nil))

		Expect(at(values, "global.extraWorkspaceEnvVars")).To(Equal(map[string]any{"FOO": "changed", "BAR": "bar"}))
	})

	It("override -> config is not changed through the values", func() {
		cfg := testConfig(`
codesphere:
  override:
    global:
      ceph: {storageClass: rbd}
`)
		values := build(cfg, testBOM(nil))
		at(values, "global.ceph").(map[string]any)["storageClass"] = "changed"

		Expect(cfg.Codesphere.Override["global"].(map[string]any)["ceph"]).To(Equal(map[string]any{"storageClass": "rbd"}))
	})

	It("workspace-pvc-storage flag -> no CephFS", func() {
		values := build(testConfig(`codesphere: {internal: [workspace-pvc-storage]}`), testBOM(nil))

		Expect(at(values, "global.ceph.mdsNamespace")).To(Equal(""))
		Expect(at(values, "global.ceph.activeMds")).To(Equal("0"))
	})

	It("configured ceph volume -> its name and active MDS count", func() {
		values := build(testConfig(`ceph: {volumes: {codesphere-demo: {mds: {active: 3}}}}`), testBOM(nil))

		Expect(at(values, "global.ceph.mdsNamespace")).To(Equal("codesphere-demo"))
		Expect(at(values, "global.ceph.activeMds")).To(Equal("3"))
	})

	It("disabled gateway APIs -> no gateway domains experiment", func() {
		values := build(testConfig(`
cluster:
  gateway: {gatewayApi: {enabled: false}}
  publicGateway: {gatewayApi: {enabled: false, use: false}}
`), testBOM(nil))

		Expect(at(values, "global.frontendGateway.gatewayApi.enabled")).To(BeFalse())
		Expect(at(values, "global.customDomains.gatewayApi.useGateway")).To(BeFalse())
		Expect(at(values, "global.experiments")).NotTo(HaveKey("gateway-domains"))
	})

	It("flags -> experiments and features", func() {
		values := build(testConfig(`
codesphere:
  experiments: [legacy]
  internal: [internal-flag]
  preview: {preview-on: true, preview-off: false}
  features: {feature-on: true, feature-off: false}
`), testBOM(nil))

		experiments := at(values, "global.experiments").(map[string]any)
		Expect(experiments).To(HaveKeyWithValue("legacy", []string{"prod"}))
		Expect(experiments).To(HaveKeyWithValue("internal-flag", []string{"prod"}))
		Expect(experiments).To(HaveKeyWithValue("preview-on", []string{"prod"}))
		Expect(experiments).NotTo(HaveKey("preview-off"))

		features := at(values, "global.features").(map[string]any)
		Expect(features).To(HaveKeyWithValue("feature-on", []string{"prod"}))
		Expect(features).To(HaveKeyWithValue("feature-off", []string{}))
	})

	It("ACME with a custom domains EAB key -> that key and its vault entry", func() {
		values := build(testConfig(`
codesphere:
  certIssuer:
    type: acme
    acme: {server: https://acme.example.com, email: ops@example.com, eabKeyId: cluster-key, customDomainsEabKeyId: custom-key}
`), testBOM(nil))

		Expect(at(values, "global.certIssuer.acme")).To(Equal(map[string]any{
			"enabled":       true,
			"server":        "https://acme.example.com",
			"email":         "ops@example.com",
			"eabKeyId":      "custom-key",
			"eabMacKeyName": "acmeCustomDomainsEabMacKey",
		}))
		Expect(at(values, "global.frontendGateway.issuer")).To(Equal("acme"))
	})

	It("ACME with only the cluster EAB key -> falls back to it", func() {
		values := build(testConfig(`
codesphere:
  certIssuer:
    type: acme
    acme: {server: https://acme.example.com, email: ops@example.com, eabKeyId: cluster-key}
`), testBOM(nil))

		Expect(at(values, "global.certIssuer.acme.eabKeyId")).To(Equal("cluster-key"))
		Expect(at(values, "global.certIssuer.acme.eabMacKeyName")).To(Equal("acmeEabMacKey"))
	})

	It("internal postgres -> primary host on the default port", func() {
		values := build(testConfig(`
postgres:
  serverAddress: ""
  caCertPem: ca-pem
  primary: {ip: 10.0.0.5, hostname: pg-primary, sslConfig: {serverCertPem: cert}}
`), testBOM(nil))

		Expect(at(values, "global.postgres")).To(Equal(map[string]any{
			"host":               "10.0.0.5",
			"port":               5432,
			"database":           "codesphere",
			"rejectUnauthorized": true,
			"ssl":                map[string]any{"ca": "ca-pem", "serverName": "pg-primary"},
		}))
	})

	It("external postgres -> alt name as server name", func() {
		values := build(testConfig(`postgres: {serverAddress: pg.example.com, altName: pg-rw, port: 5433, database: masterdata}`), testBOM(nil))

		Expect(at(values, "global.postgres.host")).To(Equal("pg.example.com"))
		Expect(at(values, "global.postgres.port")).To(Equal(5433))
		Expect(at(values, "global.postgres.database")).To(Equal("masterdata"))
		Expect(at(values, "global.postgres.ssl.serverName")).To(Equal("pg-rw"))
	})

	It("several data centers -> sorted by id, configured default", func() {
		values := build(testConfig(`
dataCenters:
  - {id: 2, name: second, city: Frankfurt, countryCode: DE}
  - {id: 1, name: main, city: Karlsruhe, countryCode: DE}
defaultDataCenterId: 2
`), testBOM(nil))

		Expect(at(values, "global.availableDataCenters")).To(Equal([]int{1, 2}))
		Expect(at(values, "global.dataCenterId")).To(Equal(1))
		Expect(at(values, "global.defaultDataCenterId")).To(Equal(2))
	})

	It("data centers without the installed one -> error", func() {
		_, err := codespherevalues.Build(testConfig(`dataCenters: [{id: 2, name: second, city: Frankfurt, countryCode: DE}]`), testBOM(nil))

		Expect(err).To(MatchError(ContainSubstring("missing in .dataCenters")))
	})

	It("enabled MetalLB without an IP service -> all its pools", func() {
		values := build(testConfig(`cluster: {metallb: {enabled: true, pools: [{name: public, ipAddresses: [203.0.113.0/28]}]}}`), testBOM(nil))

		Expect(at(values, "global.ipService")).To(Equal(map[string]any{"loadBalancerKind": "metallb", "addressPools": []string{"public"}}))
	})

	It("IP service with a pool MetalLB does not define -> error", func() {
		_, err := codespherevalues.Build(testConfig(`
cluster: {metallb: {enabled: true, pools: [{name: public, ipAddresses: [203.0.113.0/28]}]}}
codesphere: {ipService: {loadBalancerKind: metallb, addressPools: [private]}}
`), testBOM(nil))

		Expect(err).To(MatchError(ContainSubstring("pool private is not defined")))
	})

	It("managed service with legacy plans -> parameters moved to the provider", func() {
		values := build(testConfig(`
codesphere:
  managedServices:
    - name: postgres
      schemaVersion: v1
      plans:
        - id: 1
          name: small
          description: Small
          parameters:
            storage: {pricedAs: storage, schema: {type: integer, default: 10}}
`), testBOM(nil))

		Expect(at(values, "global.managedServices")).To(Equal([]any{map[string]any{
			"name":               "postgres",
			"schemaVersion":      "v1",
			"scope":              "global",
			"resourceParameters": map[string]any{"storage": map[string]any{"pricedAs": "storage", "schema": map[string]any{"type": "integer"}}},
			"plans":              []any{map[string]any{"id": 1, "name": "small", "description": "Small", "parameters": map[string]any{"storage": 10}}},
		}}))
	})

	It("managed service resource parameter set to null -> kept as null", func() {
		values := build(testConfig(`
codesphere:
  managedServices:
    - {name: postgres, schemaVersion: v1, scope: team, resourceParameters: {storage: null}}
`), testBOM(nil))

		service := at(values, "global.managedServices").([]any)[0].(map[string]any)
		Expect(service["scope"]).To(Equal("team"))
		Expect(service["resourceParameters"]).To(Equal(map[string]any{"storage": nil}))
	})

	It("deploy config and pooled instances -> error", func() {
		_, err := codespherevalues.Build(testConfig(`
codesphere:
  plans:
    hostingPlans:
      8: {cpuTenth: 10, gpuParts: 0, memoryMb: 2048, storageMb: 20480, tempStorageMb: 1500, pooledInstances: 2}
  deployConfig:
    images:
      ubuntu-24.04:
        name: Ubuntu 24.04
        supportedUntil: "2028-05-31"
        flavors: {default: {image: {bomRef: workspace-agent-24.04}, pool: {8: 1}}}
`), testBOM(nil))

		Expect(err).To(MatchError(ContainSubstring("ambiguous")))
	})

	It("traces without an endpoint -> error", func() {
		_, err := codespherevalues.Build(testConfig(`codesphere: {telemetryExport: {traces: true}}`), testBOM(nil))

		Expect(err).To(MatchError(ContainSubstring("traceEndpoint must be set")))
	})

	It("BOM without the workspace server -> error", func() {
		b := testBOM(nil)
		delete(b.Components["codesphere"].ContainerImages, "workspace-server")

		_, err := codespherevalues.Build(testConfig(""), b)

		Expect(err).To(MatchError(ContainSubstring("workspace-server not found")))
	})
})
