// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("ConfigYaml", func() {
	var (
		rootConfig files.RootConfig
		tempDir    string
		configFile string
		sampleYaml string
	)

	BeforeEach(func() {
		rootConfig = files.NewRootConfig()

		var err error
		tempDir, err = os.MkdirTemp("", "config_yaml_test")
		Expect(err).NotTo(HaveOccurred())

		configFile = filepath.Join(tempDir, "config.yaml")

		sampleYaml = `registry:
  server: registry.example.com

codesphere:
  migration:
    postgres:
      host: 10.0.0.25
      port: 30432
      database: masterdata
      altName: masterdata-rw.codesphere.svc.cluster.local
  deployConfig:
    images:
      workspace-agent-24.04:
        name: ubuntu-24.04
        supportedUntil: "2029-04-01"
        flavors:
          default:
            image:
              bomRef: workspace-agent-24.04
              dockerfile: dockerfile-24.04
            pool:
              8: 2
              16: 1
          minimal:
            image:
              bomRef: workspace-agent-24.04-minimal
              dockerfile: dockerfile-24.04-minimal
            pool:
              4: 1
          directref:
            image: custom-fake-image:latest
            pool:
              4: 2
      workspace-agent-20.04:
        name: ubuntu-20.04
        supportedUntil: "2025-04-01"
        flavors:
          default:
            image:
              bomRef: workspace-agent-20.04
              dockerfile: dockerfile-20.04
            pool:
              8: 1
      ide-service:
        name: ide-service
        supportedUntil: "2026-01-01"
        flavors:
          default:
            image:
              bomRef: ide-service
            pool:
              4: 2
`
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Describe("ParseConfig", func() {
		It("should parse a valid YAML config file successfully", func() {
			err := os.WriteFile(configFile, []byte(sampleYaml), 0644)
			Expect(err).NotTo(HaveOccurred())

			data, err := os.ReadFile(configFile)
			Expect(err).NotTo(HaveOccurred())

			err = rootConfig.Unmarshal(data)
			Expect(err).NotTo(HaveOccurred())

			Expect(rootConfig.Registry.Server).To(Equal("registry.example.com"))
			Expect(rootConfig.Codesphere.Migration).NotTo(BeNil())
			Expect(rootConfig.Codesphere.Migration.Postgres).NotTo(BeNil())
			Expect(rootConfig.Codesphere.Migration.Postgres.Host).To(Equal("10.0.0.25"))
			Expect(rootConfig.Codesphere.Migration.Postgres.Port).To(Equal(30432))
			Expect(rootConfig.Codesphere.Migration.Postgres.Database).To(Equal("masterdata"))
			Expect(rootConfig.Codesphere.Migration.Postgres.AltName).To(Equal("masterdata-rw.codesphere.svc.cluster.local"))
			Expect(rootConfig.Codesphere.DeployConfig.Images).To(HaveKey("workspace-agent-24.04"))
			Expect(rootConfig.Codesphere.DeployConfig.Images).To(HaveKey("workspace-agent-20.04"))
			Expect(rootConfig.Codesphere.DeployConfig.Images).To(HaveKey("ide-service"))

			// Check specific image config
			workspaceAgent24 := rootConfig.Codesphere.DeployConfig.Images["workspace-agent-24.04"]
			Expect(workspaceAgent24.Name).To(Equal("ubuntu-24.04"))
			Expect(workspaceAgent24.SupportedUntil).To(Equal("2029-04-01"))
			Expect(workspaceAgent24.Flavors).To(HaveKey("default"))
			Expect(workspaceAgent24.Flavors).To(HaveKey("minimal"))

			// Check flavor details
			defaultFlavor := workspaceAgent24.Flavors["default"]
			Expect(defaultFlavor.Image.BomRef).To(Equal("workspace-agent-24.04"))
			Expect(defaultFlavor.Image.Dockerfile).To(Equal("dockerfile-24.04"))
			Expect(defaultFlavor.Pool).To(HaveKeyWithValue(8, 2))
			Expect(defaultFlavor.Pool).To(HaveKeyWithValue(16, 1))

			directReferencedFlavor := workspaceAgent24.Flavors["directref"]
			Expect(directReferencedFlavor.Image.ImageName).To(Equal("custom-fake-image:latest"))
			Expect(directReferencedFlavor.Image.BomRef).To(Equal(""))
			Expect(directReferencedFlavor.Image.Dockerfile).To(Equal(""))
			Expect(directReferencedFlavor.Pool).To(HaveKeyWithValue(4, 2))
		})

		It("should return error for non-existent file", func() {
			_, err := os.ReadFile("/non/existent/config.yaml")
			Expect(err).To(HaveOccurred())
		})

		It("should return error for invalid YAML", func() {
			invalidYaml := `registry:
  server: registry.example.com
codesphere:
  deployConfig:
    images:
      - invalid: yaml structure without proper mapping
`
			err := os.WriteFile(configFile, []byte(invalidYaml), 0644)
			Expect(err).NotTo(HaveOccurred())

			data, err := os.ReadFile(configFile)
			Expect(err).NotTo(HaveOccurred())

			err = rootConfig.Unmarshal(data)
			Expect(err).To(HaveOccurred())
		})

		It("should handle empty config file", func() {
			err := os.WriteFile(configFile, []byte(""), 0644)
			Expect(err).NotTo(HaveOccurred())

			data, err := os.ReadFile(configFile)
			Expect(err).NotTo(HaveOccurred())

			err = rootConfig.Unmarshal(data)
			Expect(err).NotTo(HaveOccurred())

			Expect(rootConfig.Registry.Server).To(BeEmpty())
			Expect(rootConfig.Codesphere.DeployConfig.Images).To(BeEmpty())
		})

		It("should handle minimal valid config", func() {
			minimalYaml := `registry:
  server: minimal.registry.com
codesphere:
  deployConfig:
    images: {}
`
			err := os.WriteFile(configFile, []byte(minimalYaml), 0644)
			Expect(err).NotTo(HaveOccurred())

			data, err := os.ReadFile(configFile)
			Expect(err).NotTo(HaveOccurred())

			err = rootConfig.Unmarshal(data)
			Expect(err).NotTo(HaveOccurred())

			Expect(rootConfig.Registry.Server).To(Equal("minimal.registry.com"))
			Expect(rootConfig.Codesphere.DeployConfig.Images).To(BeEmpty())
		})

		It("should handle LTS 1.77.2 format where codesphere is a path string", func() {
			lts177Yaml := `registry:
  server: registry.example.com
codesphere: /etc/codesphere/codesphere.yaml
`
			err := os.WriteFile(configFile, []byte(lts177Yaml), 0644)
			Expect(err).NotTo(HaveOccurred())

			data, err := os.ReadFile(configFile)
			Expect(err).NotTo(HaveOccurred())

			err = rootConfig.Unmarshal(data)
			Expect(err).NotTo(HaveOccurred())

			Expect(rootConfig.Registry.Server).To(Equal("registry.example.com"))
			Expect(rootConfig.CodesphereConfigPath).To(Equal("/etc/codesphere/codesphere.yaml"))
			Expect(rootConfig.Codesphere.DeployConfig.Images).To(BeEmpty())
		})
	})

	Describe("ExtractBomRefs", func() {
		BeforeEach(func() {
			err := os.WriteFile(configFile, []byte(sampleYaml), 0644)
			Expect(err).NotTo(HaveOccurred())

			data, err := os.ReadFile(configFile)
			Expect(err).NotTo(HaveOccurred())

			err = rootConfig.Unmarshal(data)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should extract all BOM references from config", func() {
			bomRefs := rootConfig.ExtractBomRefs()

			Expect(bomRefs).NotTo(BeEmpty())
			Expect(bomRefs).To(ContainElement("workspace-agent-24.04"))
			Expect(bomRefs).To(ContainElement("workspace-agent-24.04-minimal"))
			Expect(bomRefs).To(ContainElement("workspace-agent-20.04"))
			Expect(bomRefs).To(ContainElement("ide-service"))
			Expect(len(bomRefs)).To(Equal(4))
		})

		It("should return empty slice when no images are configured", func() {
			emptyConfig := &files.RootConfig{}
			bomRefs := emptyConfig.ExtractBomRefs()

			Expect(bomRefs).To(BeEmpty())
		})

		It("should handle flavors without BOM references", func() {
			noImagesConfig := &files.RootConfig{}
			yamlWithoutBomRefs := `registry:
  server: registry.example.com
codesphere:
  deployConfig:
    images:
      test-image:
        name: test
        flavors:
          default:
            image:
              dockerfile: dockerfile-only
            pool:
              4: 1
`
			err := os.WriteFile(configFile, []byte(yamlWithoutBomRefs), 0644)
			Expect(err).NotTo(HaveOccurred())

			data, err := os.ReadFile(configFile)
			Expect(err).NotTo(HaveOccurred())

			err = noImagesConfig.Unmarshal(data)
			Expect(err).NotTo(HaveOccurred())

			bomRefs := noImagesConfig.ExtractBomRefs()
			Expect(bomRefs).To(BeEmpty())
		})
	})

	Describe("ExtractWorkspaceDockerfiles", func() {
		BeforeEach(func() {
			err := os.WriteFile(configFile, []byte(sampleYaml), 0644)
			Expect(err).NotTo(HaveOccurred())

			data, err := os.ReadFile(configFile)
			Expect(err).NotTo(HaveOccurred())

			err = rootConfig.Unmarshal(data)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("ExtractVault", func() {
		It("extracts external Loki password into the configured vault secret", func() {
			rootConfig.Cluster.Monitoring = &files.MonitoringConfig{
				GrafanaAlloy: &files.GrafanaAlloyConfig{
					Loki: &files.LokiConnectionConfig{
						Endpoint: "https://loki.example.com/loki/api/v1/push",
						Password: "fake-loki-password",
						User:     "fake-loki-user",
					},
				},
			}

			vault := rootConfig.ExtractVault()

			Expect(vault.Secrets).To(ContainElement(files.SecretEntry{
				Name: files.LokiGatewayPasswordSecretName,
				Fields: &files.SecretFields{
					Password: "fake-loki-password",
				},
			}))
		})

		It("extracts Prometheus remote write credentials into vault secrets", func() {
			rootConfig.Cluster.Monitoring = &files.MonitoringConfig{
				Prometheus: &files.PrometheusConfig{
					RemoteWrite: &files.RemoteWriteConfig{
						Enabled:     true,
						Url:         "https://prometheus.example.com/api/v1/write",
						ClusterName: "test-cluster",
						Username:    "prom-user",
						Password:    "prom-password",
					},
				},
			}

			vault := rootConfig.ExtractVault()

			Expect(vault.Secrets).To(ContainElement(files.SecretEntry{
				Name:   "promRemoteWriteUser",
				Fields: &files.SecretFields{Password: "prom-user"},
			}))
			Expect(vault.Secrets).To(ContainElement(files.SecretEntry{
				Name:   "promRemoteWritePassword",
				Fields: &files.SecretFields{Password: "prom-password"},
			}))
		})

		It("skips Prometheus remote write secrets when credentials are missing", func() {
			rootConfig.Cluster.Monitoring = &files.MonitoringConfig{
				Prometheus: &files.PrometheusConfig{
					RemoteWrite: &files.RemoteWriteConfig{
						Enabled: true,
						Url:     "https://prometheus.example.com/api/v1/write",
					},
				},
			}

			vault := rootConfig.ExtractVault()

			for _, s := range vault.Secrets {
				Expect(s.Name).NotTo(Equal("promRemoteWriteUser"))
				Expect(s.Name).NotTo(Equal("promRemoteWritePassword"))
			}
		})
	})

	Describe("ACME config structure", func() {
		// Verifies the marshaled YAML matches the structure documented at:
		// https://docs.codesphere.com/private-cloud/cluster-ingress-ca-options
		It("should marshal config.yaml to the expected ACME structure", func() {
			rootConfig.Codesphere.CertIssuer = files.CertIssuerConfig{
				Type: files.CertIssuerTypeACME,
				Acme: &files.ACMEConfig{
					Enabled:   true,
					Server:    "https://acme-v02.api.letsencrypt.org/directory",
					Email:     "admin@example.com",
					EABKeyID:  "my-eab-key-id",
					EABMacKey: "my-eab-mac-key",
					Solver: files.ACMESolver{
						DNS01: &files.ACMEDNS01Solver{
							Provider: "cloudflare",
							Config: map[string]interface{}{
								"apiTokenSecretRef": map[string]interface{}{
									"name": "acme-solver",
									"key":  "api-token",
								},
							},
							Secrets: map[string]string{
								"api-token": "fake-api-token",
							},
						},
					},
				},
			}
			// Only set user-provided override fields; buildACMEOverride (called by Marshal)
			// generates the dnsSolver section from the Solver config.

			data, err := rootConfig.Marshal()
			Expect(err).NotTo(HaveOccurred())

			var raw map[string]interface{}
			Expect(yaml.Unmarshal(data, &raw)).NotTo(HaveOccurred())

			// Expected codesphere.certIssuer per upstream docs (no solver field)
			expectedCertIssuer := map[string]interface{}{
				"type": "acme",
				"acme": map[string]interface{}{
					"enabled":  true,
					"server":   "https://acme-v02.api.letsencrypt.org/directory",
					"email":    "admin@example.com",
					"eabKeyId": "my-eab-key-id",
				},
			}

			// Expected cluster.certificates.override per upstream docs
			expectedOverride := map[string]interface{}{
				"issuers": map[string]interface{}{
					"acme": map[string]interface{}{
						"dnsSolver": map[string]interface{}{
							"cloudflare": map[string]interface{}{
								"apiTokenSecretRef": map[string]interface{}{
									"name": "acme-solver",
									"key":  "api-token",
								},
							},
						},
					},
				},
			}

			// Deep compare relevant sections
			codesphere := raw["codesphere"].(map[string]interface{})
			Expect(codesphere["certIssuer"]).To(Equal(expectedCertIssuer))

			cluster := raw["cluster"].(map[string]interface{})
			certs := cluster["certificates"].(map[string]interface{})
			Expect(certs["override"]).To(Equal(expectedOverride))
		})

		It("should unmarshal ACME config from upstream docs format and populate Solver", func() {
			acmeYaml := `codesphere:
  certIssuer:
    type: acme
    acme:
      enabled: true
      server: https://acme-v02.api.letsencrypt.org/directory
      email: admin@example.com
      eabKeyId: my-eab-key-id
cluster:
  certificates:
    override:
      issuers:
        acme:
          dnsSolver:
            cloudflare:
              apiTokenSecretRef:
                key: api-token
                name: acme-solver
          solverSecret:
            data:
              api-token: fake-api-token
            name: acme-solver
`
			var parsed files.RootConfig
			err := parsed.Unmarshal([]byte(acmeYaml))
			Expect(err).NotTo(HaveOccurred())

			Expect(parsed.Codesphere.CertIssuer.Type).To(Equal(files.CertIssuerTypeACME))
			Expect(parsed.Codesphere.CertIssuer.Acme).NotTo(BeNil())
			Expect(parsed.Codesphere.CertIssuer.Acme.Server).To(Equal("https://acme-v02.api.letsencrypt.org/directory"))
			Expect(parsed.Codesphere.CertIssuer.Acme.Email).To(Equal("admin@example.com"))
			Expect(parsed.Codesphere.CertIssuer.Acme.EABKeyID).To(Equal("my-eab-key-id"))

			// Solver should be populated from override
			Expect(parsed.Codesphere.CertIssuer.Acme.Solver.DNS01).NotTo(BeNil())
			Expect(parsed.Codesphere.CertIssuer.Acme.Solver.DNS01.Provider).To(Equal("cloudflare"))
		})

		It("should marshal vault secrets to the expected ACME structure", func() {
			rootConfig.Codesphere.CertIssuer = files.CertIssuerConfig{
				Type: files.CertIssuerTypeACME,
				Acme: &files.ACMEConfig{
					Enabled:   true,
					Server:    "https://acme-v02.api.letsencrypt.org/directory",
					Email:     "admin@example.com",
					EABKeyID:  "my-eab-key-id",
					EABMacKey: "my-eab-mac-key",
					Solver: files.ACMESolver{
						DNS01: &files.ACMEDNS01Solver{
							Provider: "cloudflare",
							Secrets: map[string]string{
								"api-token": "fake-cloudflare-token",
							},
						},
					},
				},
			}

			vault := rootConfig.ExtractVault()
			vaultData, err := vault.Marshal()
			Expect(err).NotTo(HaveOccurred())

			var raw map[string]interface{}
			Expect(yaml.Unmarshal(vaultData, &raw)).NotTo(HaveOccurred())

			// Expected vault structure per upstream docs
			expectedSecrets := []interface{}{
				map[string]interface{}{
					"name": "acmeEabMacKey",
					"fields": map[string]interface{}{
						"password": "my-eab-mac-key",
					},
				},
				map[string]interface{}{
					"name": "acmeDNS01Api-token",
					"fields": map[string]interface{}{
						"password": "fake-cloudflare-token",
					},
				},
			}

			secrets := raw["secrets"].([]interface{})
			for _, expected := range expectedSecrets {
				Expect(secrets).To(ContainElement(expected))
			}
		})
	})
})
