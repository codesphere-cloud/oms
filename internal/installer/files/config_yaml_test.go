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

	Describe("ACME config structure", func() {
		// Verifies the marshaled YAML matches the structure documented at:
		// https://docs.codesphere.com/private-cloud/cluster-ingress-ca-options
		It("should marshal config.yaml to the expected ACME structure", func() {
			rootConfig.Codesphere.CertIssuer = files.CertIssuerConfig{
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
