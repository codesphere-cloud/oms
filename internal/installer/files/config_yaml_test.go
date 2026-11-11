// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files_test

import (
	"bytes"
	"log"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
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
pcApps:
  spec:
    source:
      targetRevision: main
      helm:
        valuesObject:
          featureFlags:
            enableSomething: true
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
			Expect(rootConfig.PcApps).To(HaveKey("spec"))
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

		It("parses additional registries independently of the registry config", func() {
			var cfg files.RootConfig
			Expect(cfg.Unmarshal([]byte(`registry:
  server: ghcr.io
additionalRegistries:
  - name: codesphere
    registry: registry.codesphere.com
`))).To(Succeed())

			Expect(cfg.Registry.Server).To(Equal("ghcr.io"))
			Expect(cfg.AdditionalRegistries).To(Equal([]files.AdditionalRegistryConfig{
				{Name: "codesphere", Registry: "registry.codesphere.com"},
			}))
		})

		It("omits additional registries when they are not configured", func() {
			cfg := files.RootConfig{}

			data, err := cfg.Marshal()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("additionalRegistries"))
		})
	})

	Describe("SetAdditionalRegistry", func() {
		It("appends a registry with a new name", func() {
			cfg := files.RootConfig{AdditionalRegistries: []files.AdditionalRegistryConfig{
				{Name: "other", Registry: "other.example.com"},
			}}

			cfg.SetAdditionalRegistry(files.AdditionalRegistryConfig{Name: "codesphere", Registry: "registry.codesphere.com"})

			Expect(cfg.AdditionalRegistries).To(Equal([]files.AdditionalRegistryConfig{
				{Name: "other", Registry: "other.example.com"},
				{Name: "codesphere", Registry: "registry.codesphere.com"},
			}))
		})

		It("replaces the registry with the same name", func() {
			cfg := files.RootConfig{AdditionalRegistries: []files.AdditionalRegistryConfig{
				{Name: "codesphere", Registry: "registry.dev.codesphere.com"},
			}}

			cfg.SetAdditionalRegistry(files.AdditionalRegistryConfig{Name: "codesphere", Registry: "registry.codesphere.com"})

			Expect(cfg.AdditionalRegistries).To(Equal([]files.AdditionalRegistryConfig{
				{Name: "codesphere", Registry: "registry.codesphere.com"},
			}))
		})
	})

	Describe("additional registry secret names", func() {
		It("prefixes the username and password entries with the registry name", func() {
			Expect(files.AdditionalRegistryUsernameSecret("codesphere")).To(Equal("codesphereRegistryUsername"))
			Expect(files.AdditionalRegistryPasswordSecret("codesphere")).To(Equal("codesphereRegistryPassword"))
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

	Describe("RegistryConfig.WorkspaceImage", func() {
		It("registry server -> image named after the image and flavor", func() {
			registry := files.RegistryConfig{Server: "registry.example.com"}

			Expect(registry.WorkspaceImage("ubuntu-24.04", "default")).To(Equal("registry.example.com/ubuntu-24.04-default"))
		})

		It("registry server with a trailing slash -> image without a double slash", func() {
			registry := files.RegistryConfig{Server: "registry.example.com/private-cloud/"}

			Expect(registry.WorkspaceImage("ubuntu-24.04", "gpu")).To(Equal("registry.example.com/private-cloud/ubuntu-24.04-gpu"))
		})
	})

	Describe("ACME config structure", func() {
		// Verifies the marshaled YAML matches the structure documented at:
		// https://docs.codesphere.com/private-cloud/cluster-ingress-ca-options
		It("should marshal config.yaml to the expected ACME structure", func() {
			rootConfig.Codesphere.CertIssuer = &files.CertIssuerConfig{
				Type: files.CertIssuerTypeACME,
				Acme: &files.ACMEConfig{
					Enabled:  true,
					Server:   "https://acme-v02.api.letsencrypt.org/directory",
					Email:    "admin@example.com",
					EABKeyID: "my-eab-key-id",
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

			Expect(parsed.Codesphere.CertIssuer).NotTo(BeNil())
			Expect(parsed.Codesphere.CertIssuer.Type).To(Equal(files.CertIssuerTypeACME))
			Expect(parsed.Codesphere.CertIssuer.Acme).NotTo(BeNil())
			Expect(parsed.Codesphere.CertIssuer.Acme.Server).To(Equal("https://acme-v02.api.letsencrypt.org/directory"))
			Expect(parsed.Codesphere.CertIssuer.Acme.Email).To(Equal("admin@example.com"))
			Expect(parsed.Codesphere.CertIssuer.Acme.EABKeyID).To(Equal("my-eab-key-id"))

			// Solver should be populated from override
			Expect(parsed.Codesphere.CertIssuer.Acme.Solver.DNS01).NotTo(BeNil())
			Expect(parsed.Codesphere.CertIssuer.Acme.Solver.DNS01.Provider).To(Equal("cloudflare"))
		})
	})
})

var _ = Describe("InstallVault", func() {
	var vault files.InstallVault

	BeforeEach(func() {
		vault = files.InstallVault{Secrets: []files.SecretEntry{
			{Name: "postgresPassword", Fields: &files.SecretFields{Password: "pg-secret"}},
			{Name: "kubeConfig", File: &files.SecretFile{Name: "kubeconfig", Content: "apiVersion: v1"}},
			{Name: "tokenPrivateKey", Fields: &files.SecretFields{Password: "token-key"}},
		}}
	})

	Describe("RemoveSecret", func() {
		It("removes the named secret and reports that it did", func() {
			Expect(vault.RemoveSecret("kubeConfig")).To(BeTrue())

			Expect(vault.GetSecret("kubeConfig")).To(BeNil())
			Expect(vault.Secrets).To(HaveLen(2))
			Expect(vault.Secrets[0].Name).To(Equal("postgresPassword"))
			Expect(vault.Secrets[1].Name).To(Equal("tokenPrivateKey"))
		})

		It("reports that nothing was removed for an unknown secret", func() {
			Expect(vault.RemoveSecret("doesNotExist")).To(BeFalse())
			Expect(vault.Secrets).To(HaveLen(3))
		})

		It("reports that nothing was removed from an empty vault", func() {
			empty := files.InstallVault{}
			Expect(empty.RemoveSecret("postgresPassword")).To(BeFalse())
			Expect(empty.Secrets).To(BeEmpty())
		})

		It("removes the last remaining secret", func() {
			single := files.InstallVault{Secrets: []files.SecretEntry{{Name: "only"}}}
			Expect(single.RemoveSecret("only")).To(BeTrue())
			Expect(single.Secrets).To(BeEmpty())
		})
	})

	Describe("Clone", func() {
		It("copies every secret entry", func() {
			clone := vault.Clone()

			Expect(clone.Secrets).To(HaveLen(3))
			Expect(clone.GetSecret("postgresPassword").Fields.Password).To(Equal("pg-secret"))
			Expect(clone.GetSecret("kubeConfig").File.Content).To(Equal("apiVersion: v1"))
			Expect(clone.GetSecret("tokenPrivateKey").Fields.Password).To(Equal("token-key"))
		})

		It("does not alias the original's entries", func() {
			clone := vault.Clone()

			clone.GetSecret("postgresPassword").Fields.Password = "changed"
			clone.GetSecret("kubeConfig").File.Content = "changed"
			clone.SetSecret(files.SecretEntry{Name: "cephSshPrivateKey"})
			Expect(clone.RemoveSecret("tokenPrivateKey")).To(BeTrue())

			Expect(vault.GetSecret("postgresPassword").Fields.Password).To(Equal("pg-secret"))
			Expect(vault.GetSecret("kubeConfig").File.Content).To(Equal("apiVersion: v1"))
			Expect(vault.GetSecret("cephSshPrivateKey")).To(BeNil())
			Expect(vault.GetSecret("tokenPrivateKey")).NotTo(BeNil())
		})

		It("keeps unset optional entry fields nil", func() {
			sparse := files.InstallVault{Secrets: []files.SecretEntry{{Name: "empty"}}}

			clone := sparse.Clone()

			Expect(clone.Secrets).To(HaveLen(1))
			Expect(clone.Secrets[0].File).To(BeNil())
			Expect(clone.Secrets[0].Fields).To(BeNil())
		})

		It("clones a nil vault to nil", func() {
			var unloaded *files.InstallVault
			Expect(unloaded.Clone()).To(BeNil())
		})
	})
})

var _ = Describe("RootConfig Clone", func() {
	var config files.RootConfig

	BeforeEach(func() {
		config = files.NewRootConfig()
		config.Datacenter = files.DatacenterConfig{ID: 1, Name: "dc1"}
		config.DataCenters = []files.DatacenterConfig{{ID: 1, Name: "dc1"}}
		config.Secrets.BaseDir = "/etc/codesphere/secrets"
		config.Registry.Server = "registry.example.com"
		config.Codesphere.Features = map[string]bool{"multiDc": true}
		config.Operations = &files.OperationsConfig{Skip: []string{"ceph"}}
	})

	It("copies the config", func() {
		clone, err := config.Clone()
		Expect(err).NotTo(HaveOccurred())

		Expect(clone.Datacenter).To(Equal(config.Datacenter))
		Expect(clone.DataCenters).To(Equal(config.DataCenters))
		Expect(clone.Secrets.BaseDir).To(Equal("/etc/codesphere/secrets"))
		Expect(clone.Registry.Server).To(Equal("registry.example.com"))
		Expect(clone.Codesphere.Features).To(Equal(map[string]bool{"multiDc": true}))
		Expect(clone.Operations.Skip).To(Equal([]string{"ceph"}))
	})

	It("does not share maps, slices or pointers with the original", func() {
		clone, err := config.Clone()
		Expect(err).NotTo(HaveOccurred())

		clone.Datacenter = files.DatacenterConfig{ID: 2, Name: "dc2"}
		clone.DataCenters[0].Name = "changed"
		clone.Secrets.BaseDir = "/etc/codesphere/secrets-dc2"
		clone.Registry.Server = "changed"
		clone.Codesphere.Features["multiDc"] = false
		clone.Operations.Skip = append(clone.Operations.Skip, "postgres")

		Expect(config.Datacenter.Name).To(Equal("dc1"))
		Expect(config.DataCenters[0].Name).To(Equal("dc1"))
		Expect(config.Secrets.BaseDir).To(Equal("/etc/codesphere/secrets"))
		Expect(config.Registry.Server).To(Equal("registry.example.com"))
		Expect(config.Codesphere.Features).To(Equal(map[string]bool{"multiDc": true}))
		Expect(config.Operations.Skip).To(Equal([]string{"ceph"}))
	})

	It("drops keys the struct does not model, like any other load of the config", func() {
		var loaded files.RootConfig
		Expect(loaded.Unmarshal([]byte("dataCenter:\n  id: 1\nunmodelledKey: value\n"))).NotTo(HaveOccurred())

		clone, err := loaded.Clone()
		Expect(err).NotTo(HaveOccurred())

		data, err := clone.Marshal()
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).NotTo(ContainSubstring("unmodelledKey"))
	})
})

var _ = Describe("Barman Cloud plugin migration", func() {
	const legacyOverride = "    override:\n      plugin-barman-cloud:\n        replicaCount: 2\n"

	var logOutput bytes.Buffer

	BeforeEach(func() {
		logOutput.Reset()

		previousLogWriter := log.Writer()

		log.SetOutput(&logOutput)
		DeferCleanup(func() { log.SetOutput(previousLogWriter) })
	})

	replicaOverride := files.ChartOverride{"plugin-barman-cloud": map[string]any{"replicaCount": 2}}

	expectBarmanCloudPlugin := func(actual, expected *files.BarmanCloudPluginConfig) {
		if expected == nil {
			Expect(actual).To(BeNil())
			return
		}

		Expect(actual).To(Equal(expected))
	}

	expectLog := func(expected string) {
		if expected == "" {
			Expect(logOutput.String()).To(BeEmpty())
			return
		}

		Expect(logOutput.String()).To(ContainSubstring(expected))
	}

	cases := []TableEntry{
		Entry("with the legacy key enabled, it moves with its override and logs the move",
			"cluster:\n  BarmanCloudPluginConfig:\n    enabled: true\n"+legacyOverride,
			&files.BarmanCloudPluginConfig{Enabled: true, Override: replicaOverride},
			"Moved cluster.BarmanCloudPluginConfig to managedServiceBackends.barmanCloudPlugin"),
		Entry("with the legacy key disabled, it moves enabled and warns",
			"cluster:\n  BarmanCloudPluginConfig:\n    enabled: false\n"+legacyOverride,
			&files.BarmanCloudPluginConfig{Enabled: true, Override: replicaOverride},
			"Warning: cluster.BarmanCloudPluginConfig had enabled: false"),
		Entry("with both keys set, it keeps the new key and warns that the legacy key is ignored",
			"cluster:\n  BarmanCloudPluginConfig:\n    enabled: true\n"+legacyOverride+
				"managedServiceBackends:\n  barmanCloudPlugin:\n    enabled: false\n",
			&files.BarmanCloudPluginConfig{Enabled: false},
			"Warning: ignoring cluster.BarmanCloudPluginConfig"),
		Entry("with override: null, it moves the key without an override",
			"cluster:\n  BarmanCloudPluginConfig:\n    enabled: true\n    override: null\n",
			&files.BarmanCloudPluginConfig{Enabled: true},
			"Moved cluster.BarmanCloudPluginConfig"),
		Entry("with managedServiceBackends: null, it creates the block",
			"cluster:\n  BarmanCloudPluginConfig:\n    enabled: true\nmanagedServiceBackends: null\n",
			&files.BarmanCloudPluginConfig{Enabled: true},
			"Moved cluster.BarmanCloudPluginConfig"),
		Entry("with a null legacy key, it drops it without creating the new key",
			"cluster:\n  BarmanCloudPluginConfig: null\n",
			nil,
			""),
		Entry("without the legacy key, it leaves the new key unchanged and logs nothing",
			"managedServiceBackends:\n  barmanCloudPlugin:\n    enabled: false\n",
			&files.BarmanCloudPluginConfig{Enabled: false},
			""),
	}

	DescribeTable("through RootConfig.Unmarshal", func(configYAML string, expected *files.BarmanCloudPluginConfig, expectedLog string) {
		config := files.NewRootConfig()
		Expect(config.Unmarshal([]byte(configYAML))).To(Succeed())

		expectBarmanCloudPlugin(config.ManagedServiceBackends.BarmanCloudPlugin, expected)
		expectLog(expectedLog)
	}, cases)

	DescribeTable("through MigrateLegacyBarmanCloudPluginKey on the raw map", func(configYAML string, expected *files.BarmanCloudPluginConfig, expectedLog string) {
		var raw map[string]any
		Expect(yaml.Unmarshal([]byte(configYAML), &raw)).To(Succeed())

		Expect(files.MigrateLegacyBarmanCloudPluginKey(raw)).To(Succeed())

		if cluster, ok := raw["cluster"].(map[string]any); ok {
			Expect(cluster).NotTo(HaveKey("BarmanCloudPluginConfig"))
		}

		data, err := yaml.Marshal(raw)
		Expect(err).NotTo(HaveOccurred())

		config := files.NewRootConfig()
		Expect(config.Unmarshal(data)).To(Succeed())
		expectBarmanCloudPlugin(config.ManagedServiceBackends.BarmanCloudPlugin, expected)
		expectLog(expectedLog)
	}, cases)

	It("with the legacy key loaded, marshal writes only managedServiceBackends.barmanCloudPlugin", func() {
		config := files.NewRootConfig()
		Expect(config.Unmarshal([]byte("cluster:\n  BarmanCloudPluginConfig:\n    enabled: true\n" + legacyOverride))).To(Succeed())

		data, err := config.Marshal()
		Expect(err).NotTo(HaveOccurred())

		var written map[string]any
		Expect(yaml.Unmarshal(data, &written)).To(Succeed())
		Expect(written["cluster"]).NotTo(HaveKey("BarmanCloudPluginConfig"))
		Expect(written["managedServiceBackends"]).To(HaveKeyWithValue("barmanCloudPlugin", map[string]any{
			"enabled":  true,
			"override": map[string]any{"plugin-barman-cloud": map[string]any{"replicaCount": 2}},
		}))
	})

	It("with override: null in the raw map, migrating writes no override key", func() {
		raw := map[string]any{"cluster": map[string]any{
			"BarmanCloudPluginConfig": map[string]any{"enabled": true, "override": nil},
		}}

		Expect(files.MigrateLegacyBarmanCloudPluginKey(raw)).To(Succeed())

		Expect(raw["managedServiceBackends"]).To(Equal(map[string]any{"barmanCloudPlugin": map[string]any{"enabled": true}}))
	})

	It("with both keys set in the raw map, migrating keeps the new key as written, unmodelled fields included", func() {
		current := map[string]any{"enabled": false, "unmodelled": "kept"}
		raw := map[string]any{
			"cluster":                map[string]any{"BarmanCloudPluginConfig": map[string]any{"enabled": true}},
			"managedServiceBackends": map[string]any{"barmanCloudPlugin": current},
		}

		Expect(files.MigrateLegacyBarmanCloudPluginKey(raw)).To(Succeed())

		Expect(raw["managedServiceBackends"]).To(Equal(map[string]any{"barmanCloudPlugin": map[string]any{"enabled": false, "unmodelled": "kept"}}))
	})

	It("with a legacy key that is not a mapping, migrating the raw map fails", func() {
		raw := map[string]any{"cluster": map[string]any{"BarmanCloudPluginConfig": "enabled"}}

		err := files.MigrateLegacyBarmanCloudPluginKey(raw)

		Expect(err).To(MatchError(ContainSubstring("read cluster.BarmanCloudPluginConfig")))
	})
})

var _ = Describe("ManagedService plan parameters", func() {
	It("parses the new format with scalar plan parameters and a provider-level resourceParameters block", func() {
		yamlData := `
codesphere:
  managedServices:
    - name: ferretdb
      schemaVersion: v0
      resourceParameters:
        storage:
          pricedAs: storage-mib
          schema:
            type: integer
            minimum: 512
      plans:
        - id: 0
          name: Small
          parameters:
            storage: 512
            cpu: 0
`

		var config files.RootConfig
		Expect(config.Unmarshal([]byte(yamlData))).NotTo(HaveOccurred())

		svc := config.Codesphere.ManagedServices[0]
		Expect(svc.SchemaVersion).To(Equal("v0"))
		resource := svc.ResourceParameters["storage"]
		Expect(resource.PricedAs).To(Equal("storage-mib"))
		Expect(resource.Schema).To(HaveKeyWithValue("type", "integer"))

		params := svc.Plans[0].Parameters
		Expect(params["storage"]).To(Equal(512))
		Expect(params["cpu"]).To(Equal(0))
	})

	It("preserves a new-format managed service through a Clone round-trip", func() {
		yamlData := `
codesphere:
  managedServices:
    - name: virtual-k8s
      schemaVersion: v1
      resourceParameters:
        cpu:
          pricedAs: cpu-tenths
          schema:
            type: integer
            minimum: 32
        loadBalancerLimit:
          schema:
            type: integer
            minimum: 0
            maximum: 2
      plans:
        - id: 0
          name: Custom
          description: Modify all parameters
          parameters:
            cpu: 50
            loadBalancerLimit: 0
`

		var config files.RootConfig
		Expect(config.Unmarshal([]byte(yamlData))).NotTo(HaveOccurred())
		Expect(config.Codesphere.ManagedServices[0].SchemaVersion).To(Equal("v1"))

		clone, err := config.Clone()
		Expect(err).NotTo(HaveOccurred())

		Expect(clone.Codesphere.ManagedServices[0].SchemaVersion).To(Equal("v1"))
		Expect(clone.Codesphere.ManagedServices).To(Equal(config.Codesphere.ManagedServices))
	})

	It("keeps a null plan parameter as null through a round-trip", func() {
		yamlData := `
codesphere:
  managedServices:
    - name: ferretdb
      schemaVersion: v0
      plans:
        - id: 0
          name: Small
          parameters:
            storage: null
`

		var config files.RootConfig
		Expect(config.Unmarshal([]byte(yamlData))).NotTo(HaveOccurred())
		Expect(config.Codesphere.ManagedServices[0].SchemaVersion).To(Equal("v0"))

		clone, err := config.Clone()
		Expect(err).NotTo(HaveOccurred())

		params := clone.Codesphere.ManagedServices[0].Plans[0].Parameters
		Expect(params).To(HaveKeyWithValue("storage", BeNil()))
	})
})

var _ = Describe("CodespherePackageVersion", func() {
	DescribeTable("resolves the configured portal version", func(input, expected string) {
		cfg := files.NewRootConfig()
		Expect(cfg.Unmarshal([]byte("operations:\n  codesphere: " + input + "\n  skip: [ceph]\n"))).To(Succeed())
		version, err := cfg.CodespherePackageVersion()
		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(Equal(expected))
		Expect(cfg.Operations.Skip).To(Equal([]string{"ceph"}))
	},
		Entry("plain version", "1.95.0", "codesphere-v1.95.0"),
		Entry("quoted two-component version", `"1.10"`, "codesphere-v1.10"),
		Entry("v version", "v1.95.0", "codesphere-v1.95.0"),
		Entry("portal tag", "codesphere-v1.95.0", "codesphere-v1.95.0"),
		Entry("prerelease", "1.95.0-rc.1", "codesphere-v1.95.0-rc.1"),
		Entry("branch", "release/test", "release/test"),
	)
	DescribeTable("reads object selections with optional hashes", func(input, version, hash string) {
		cfg := files.NewRootConfig()
		Expect(cfg.Unmarshal([]byte(input))).To(Succeed())
		selected, err := cfg.CodespherePackageVersion()
		Expect(err).NotTo(HaveOccurred())
		Expect(selected).To(Equal(version))
		Expect(cfg.Operations.Codesphere.Hash).To(Equal(hash))
		encoded, err := yaml.Marshal(cfg)
		Expect(err).NotTo(HaveOccurred())

		clone := files.NewRootConfig()
		Expect(clone.Unmarshal(encoded)).To(Succeed())
		Expect(clone.Operations.Codesphere).To(Equal(cfg.Operations.Codesphere))
	},
		Entry("version only", "operations:\n  codesphere: {version: 1.111.0}\n", "codesphere-v1.111.0", ""),
		Entry("quoted two-component version", "operations:\n  codesphere: {version: \"1.10\"}\n", "codesphere-v1.10", ""),
		Entry("version and hash", "operations:\n  codesphere: {version: 1.111.0, hash: abc123}\n", "codesphere-v1.111.0", "abc123"),
		Entry("branch and hash", "operations:\n  codesphere: {version: master, hash: abc123}\n", "master", "abc123"),
	)
	It("preserves skip steps when setting a package", func() {
		cfg := files.RootConfig{Operations: &files.OperationsConfig{Skip: []string{"ceph"}}}
		cfg.SetCodespherePackage("1.111.0", "abc123")
		Expect(cfg.Operations.Codesphere).To(Equal(files.CodespherePackageConfig{Version: "1.111.0", Hash: "abc123"}))
		Expect(cfg.Operations.Skip).To(Equal([]string{"ceph"}))
	})
	DescribeTable("rejects unsupported selection types", func(selection string) {
		cfg := files.NewRootConfig()
		Expect(cfg.Unmarshal([]byte("operations:\n  codesphere: " + selection))).NotTo(Succeed())
	}, Entry("list", "[1.111.0]"), Entry("boolean", "true"), Entry("unquoted numeric version", "1.10"))

	DescribeTable("rejects a missing version", func(operations *files.OperationsConfig) {
		cfg := files.RootConfig{Operations: operations}
		_, err := cfg.CodespherePackageVersion()
		Expect(err).To(MatchError(ContainSubstring("operations.codesphere is required")))
	},
		Entry("no operations", nil),
		Entry("only skip steps", &files.OperationsConfig{Skip: []string{"ceph"}}),
		Entry("empty version", &files.OperationsConfig{}),
		Entry("hash without version", &files.OperationsConfig{Codesphere: files.CodespherePackageConfig{Hash: "abc123"}}),
	)
})

var _ = Describe("RootConfig round-trip", func() {
	roundTrip := func(path string) (map[string]interface{}, map[string]interface{}) {
		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())

		var original map[string]interface{}
		Expect(yaml.Unmarshal(data, &original)).To(Succeed())

		config := files.NewRootConfig()
		Expect(config.Unmarshal(data)).To(Succeed())
		marshaled, err := config.Marshal()
		Expect(err).NotTo(HaveOccurred())

		var saved map[string]interface{}
		Expect(yaml.Unmarshal(marshaled, &saved)).To(Succeed())

		return original, saved
	}

	It("keeps every field the Codesphere values are built from", func() {
		original, saved := roundTrip(filepath.Join("testdata", "values_config.yaml"))

		// Merging the original over the saved config only changes it if a field was lost or altered.
		// Fields OMS always writes, like secrets.baseDir, are allowed to be added.
		_, savedCopy := roundTrip(filepath.Join("testdata", "values_config.yaml"))
		Expect(util.DeepMergeMaps(savedCopy, original)).To(Equal(saved))
	})
})

var _ = Describe("RootConfig data centers", func() {
	var (
		primary   = files.DatacenterConfig{ID: 1, Name: "primary", City: "Karlsruhe", CountryCode: "DE"}
		secondary = files.DatacenterConfig{ID: 2, Name: "secondary", City: "Frankfurt", CountryCode: "DE"}
	)

	Describe("AllDataCenters", func() {
		It("no .dataCenters -> only .dataCenter", func() {
			config := files.RootConfig{Datacenter: primary}

			Expect(config.AllDataCenters()).To(Equal([]files.DatacenterConfig{primary}))
		})

		It(".dataCenters set -> .dataCenters", func() {
			config := files.RootConfig{Datacenter: primary, DataCenters: []files.DatacenterConfig{secondary, primary}}

			Expect(config.AllDataCenters()).To(Equal([]files.DatacenterConfig{secondary, primary}))
		})
	})

	Describe("ValidateDataCenters", func() {
		It("valid data centers with a default -> no error", func() {
			config := files.RootConfig{
				Datacenter:          primary,
				DataCenters:         []files.DatacenterConfig{primary, secondary},
				DefaultDataCenterID: 2,
			}

			Expect(config.ValidateDataCenters()).To(Succeed())
		})

		It("only .dataCenter -> no error", func() {
			config := files.RootConfig{Datacenter: primary}

			Expect(config.ValidateDataCenters()).To(Succeed())
		})

		It("data center with id 0 in .dataCenters -> error", func() {
			config := files.RootConfig{
				Datacenter:  primary,
				DataCenters: []files.DatacenterConfig{primary, {ID: 0, Name: "zero"}},
			}

			Expect(config.ValidateDataCenters()).To(MatchError(ContainSubstring(`data center "zero" has id 0`)))
		})

		It(".dataCenter with id 0 and no .dataCenters -> error", func() {
			config := files.RootConfig{Datacenter: files.DatacenterConfig{ID: 0, Name: "zero"}}

			Expect(config.ValidateDataCenters()).To(MatchError(ContainSubstring(`data center "zero" has id 0`)))
		})

		It("data center with a negative id -> error", func() {
			config := files.RootConfig{
				Datacenter:  primary,
				DataCenters: []files.DatacenterConfig{primary, {ID: -1, Name: "negative"}},
			}

			Expect(config.ValidateDataCenters()).To(MatchError(ContainSubstring(`data center "negative" has id -1`)))
		})

		It("duplicate id -> error", func() {
			config := files.RootConfig{
				Datacenter:  primary,
				DataCenters: []files.DatacenterConfig{primary, secondary, primary},
			}

			Expect(config.ValidateDataCenters()).To(MatchError(ContainSubstring("duplicate data center id 1")))
		})

		It(".dataCenters without .dataCenter -> error", func() {
			config := files.RootConfig{
				Datacenter:  primary,
				DataCenters: []files.DatacenterConfig{secondary},
			}

			Expect(config.ValidateDataCenters()).To(MatchError(ContainSubstring("is missing .dataCenter (id 1)")))
		})

		It(".dataCenters entry that differs from .dataCenter -> error naming both", func() {
			renamed := primary
			renamed.Name = "renamed"
			config := files.RootConfig{
				Datacenter:  primary,
				DataCenters: []files.DatacenterConfig{renamed, secondary},
			}

			err := config.ValidateDataCenters()

			Expect(err).To(MatchError(ContainSubstring("entry with id 1 differs from .dataCenter")))
			Expect(err).To(MatchError(ContainSubstring("Name:renamed")))
			Expect(err).To(MatchError(ContainSubstring("Name:primary")))
		})

		It("unknown default -> error", func() {
			config := files.RootConfig{
				Datacenter:          primary,
				DataCenters:         []files.DatacenterConfig{primary, secondary},
				DefaultDataCenterID: 3,
			}

			Expect(config.ValidateDataCenters()).To(MatchError(ContainSubstring(".defaultDataCenterId 3 is not one")))
		})

		It("no .dataCenters and a default of another data center -> error", func() {
			config := files.RootConfig{Datacenter: primary, DefaultDataCenterID: 2}

			Expect(config.ValidateDataCenters()).To(MatchError(ContainSubstring(".defaultDataCenterId 2 is not one")))
		})
	})
})
