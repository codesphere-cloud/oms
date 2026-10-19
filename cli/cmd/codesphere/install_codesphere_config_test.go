// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/installer/vault/sops"
	"github.com/codesphere-cloud/oms/internal/portal"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"net/http"
	"net/http/httptest"
)

var _ = Describe("prepareInstallConfig", func() {
	DescribeTable("merges legacy and object selections", func(base, overlay, version, hash string) {
		dir := GinkgoT().TempDir()
		basePath := filepath.Join(dir, "base.yaml")
		overlayPath := filepath.Join(dir, "overlay.yaml")

		Expect(os.WriteFile(basePath, []byte("operations:\n  skip: [ceph]\n  codesphere: "+base), 0600)).To(Succeed())
		Expect(os.WriteFile(overlayPath, []byte("operations:\n  codesphere: "+overlay), 0600)).To(Succeed())
		_, cfg, cleanup, err := prepareInstallConfig(&InstallCodesphereOpts{Configs: []string{basePath, overlayPath}}, installer.NewConfig())
		Expect(err).NotTo(HaveOccurred())

		defer cleanup()

		Expect(cfg.Operations.Codesphere).To(Equal(files.CodespherePackageConfig{Version: version, Hash: hash}))
		Expect(cfg.Operations.Skip).To(Equal([]string{"ceph"}))
	},
		Entry("object overlay adds a hash to a legacy version", "1.111.0", "{hash: abc123}", "1.111.0", "abc123"),
		Entry("legacy override replaces the version and clears the prior hash", "{version: 1.111.0, hash: abc123}", "1.112.0", "1.112.0", ""),
	)
	It("merges multiple config files in order and returns a single parsed config path", func() {
		tmpDir, err := os.MkdirTemp("", "install-config-merge-*")
		Expect(err).ToNot(HaveOccurred())
		defer func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		}()

		basePath := filepath.Join(tmpDir, "base.yaml")
		Expect(os.WriteFile(basePath, []byte(`
dataCenter:
  id: 1
  name: dc-base
  city: Base City
  countryCode: DE
secrets:
  baseDir: /base/secrets
registry:
  server: registry.base.example.com
cluster:
  gateway:
    serviceType: LoadBalancer
  publicGateway:
    serviceType: LoadBalancer
codesphere:
  domain: base.example.com
  workspaceHostingBaseDomain: ws.base.example.com
  customDomains:
    cNameBaseDomain: cname.base.example.com
  deployConfig:
    images: {}
pcApps:
  spec:
    source:
      targetRevision: from-base
`), 0644)).To(Succeed())

		overlayPath := filepath.Join(tmpDir, "overlay.yaml")
		Expect(os.WriteFile(overlayPath, []byte(`
dataCenter:
  name: dc-overlay
registry:
  server: registry.overlay.example.com
codesphere:
  domain: overlay.example.com
pcApps:
  spec:
    source:
      helm:
        valuesObject:
          featureFlags:
            enabled: true
`), 0644)).To(Succeed())

		opts := &InstallCodesphereOpts{
			Configs: []string{basePath, overlayPath},
		}

		effectiveOpts, cfg, cleanup, err := prepareInstallConfig(opts, installer.NewConfig())
		Expect(err).ToNot(HaveOccurred())
		defer cleanup()

		Expect(effectiveOpts.ConfigPath).ToNot(BeEmpty())
		Expect(effectiveOpts.ConfigPath).ToNot(Equal(basePath))
		Expect(effectiveOpts.ConfigPath).ToNot(Equal(overlayPath))
		Expect(filepath.Base(effectiveOpts.ConfigPath)).To(Equal("config.yaml"))
		Expect(effectiveOpts.Vault).To(Equal(""))
		Expect(cfg.Datacenter.ID).To(Equal(1))
		Expect(cfg.Datacenter.Name).To(Equal("dc-overlay"))
		Expect(cfg.Datacenter.City).To(Equal("Base City"))
		Expect(cfg.Registry).ToNot(BeNil())
		Expect(cfg.Registry.Server).To(Equal("registry.overlay.example.com"))
		Expect(cfg.Codesphere.Domain).To(Equal("overlay.example.com"))
		Expect(cfg.Codesphere.WorkspaceHostingBaseDomain).To(Equal("ws.base.example.com"))
		Expect(cfg.PcApps).To(HaveKey("spec"))
		spec := cfg.PcApps["spec"].(map[string]interface{})
		source := spec["source"].(map[string]interface{})
		Expect(source["targetRevision"]).To(Equal("from-base"))
		helm := source["helm"].(map[string]interface{})
		valuesObject := helm["valuesObject"].(map[string]interface{})
		featureFlags := valuesObject["featureFlags"].(map[string]interface{})
		Expect(featureFlags["enabled"]).To(BeTrue())

		_, statErr := os.Stat(effectiveOpts.ConfigPath)
		Expect(statErr).ToNot(HaveOccurred())

		cleanup()
		_, statErr = os.Stat(effectiveOpts.ConfigPath)
		Expect(os.IsNotExist(statErr)).To(BeTrue())
	})

	It("renders each config file with vault templating before merging", func() {
		if !installCodesphereSopsAndAgeAvailable() {
			Skip("sops and age-keygen not available")
		}

		tmpDir := GinkgoT().TempDir()
		basePath := filepath.Join(tmpDir, "base.yaml")
		overlayPath := filepath.Join(tmpDir, "overlay.yaml")
		vaultPath := filepath.Join(tmpDir, "prod.vault.yaml")
		plaintextVaultPath := filepath.Join(tmpDir, "prod.vault.plain.yaml")
		ageKeyPath := filepath.Join(tmpDir, "age_key.txt")

		Expect(os.WriteFile(basePath, []byte(`
dataCenter:
  id: 1
  name: dc-base
  city: '{{ secret "dcCity" }}'
  countryCode: DE
secrets:
  baseDir: /base/secrets
cluster:
  gateway:
    serviceType: LoadBalancer
  publicGateway:
    serviceType: LoadBalancer
codesphere:
  releaseName: codesphere-test
  domain: '{{ secret "baseDomain" }}'
  workspaceHostingBaseDomain: ws.base.example.com
  customDomains:
    cNameBaseDomain: cname.base.example.com
  deployConfig:
    images: {}
`), 0644)).To(Succeed())
		Expect(os.WriteFile(overlayPath, []byte(`
dataCenter:
  name: '{{ secret "dcName" }}'
pcApps:
  spec:
    source:
      targetRevision: '{{ secret "pcAppsRevision" }}'
`), 0644)).To(Succeed())

		testVault := &files.InstallVault{
			Secrets: []files.SecretEntry{
				{Name: "dcCity", File: &files.SecretFile{Content: "Templated City"}},
				{Name: "baseDomain", File: &files.SecretFile{Content: "templated.example.com"}},
				{Name: "dcName", File: &files.SecretFile{Content: "dc-from-vault"}},
				{Name: "pcAppsRevision", File: &files.SecretFile{Content: "from-vault"}},
			},
		}
		vaultYAML, err := testVault.Marshal()
		Expect(err).ToNot(HaveOccurred())
		Expect(os.WriteFile(plaintextVaultPath, vaultYAML, 0600)).To(Succeed())
		Expect(exec.Command("age-keygen", "-o", ageKeyPath).Run()).To(Succeed())
		recipient, err := exec.Command("age-keygen", "-y", ageKeyPath).Output()
		Expect(err).ToNot(HaveOccurred())
		Expect(sops.EncryptFile(plaintextVaultPath, vaultPath, strings.TrimSpace(string(recipient)))).To(Succeed())

		opts := &InstallCodesphereOpts{
			Configs: []string{basePath, overlayPath},
			Vault:   vaultPath,
			PrivKey: ageKeyPath,
		}

		effectiveOpts, cfg, cleanup, err := prepareInstallConfig(opts, installer.NewConfig())
		Expect(err).ToNot(HaveOccurred())
		defer cleanup()

		Expect(effectiveOpts.Vault).To(Equal(vaultPath))
		Expect(cfg.Datacenter.City).To(Equal("Templated City"))
		Expect(cfg.Datacenter.Name).To(Equal("dc-from-vault"))
		Expect(cfg.Codesphere.Domain).To(Equal("templated.example.com"))
		Expect(cfg.PcApps).To(HaveKey("spec"))
		spec := cfg.PcApps["spec"].(map[string]interface{})
		source := spec["source"].(map[string]interface{})
		Expect(source["targetRevision"]).To(Equal("from-vault"))
	})

	Context("with secret templates in the codesphere block", func() {
		var (
			tmpDir    string
			basePath  string
			vaultPath string
		)

		writeConfig := func(name, content string) string {
			path := filepath.Join(tmpDir, name)
			Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())

			return path
		}

		prepare := func(configs ...string) (files.RootConfig, error) {
			opts := &InstallCodesphereOpts{Configs: configs, Vault: vaultPath, VaultType: "plain"}
			_, cfg, cleanup, err := prepareInstallConfig(opts, installer.NewConfig())
			cleanup()

			return cfg, err
		}

		BeforeEach(func() {
			tmpDir = GinkgoT().TempDir()
			testVault := &files.InstallVault{
				Secrets: []files.SecretEntry{{Name: "oidcClientSecret", File: &files.SecretFile{Content: "from-vault"}}},
			}
			vaultYAML, err := testVault.Marshal()
			Expect(err).ToNot(HaveOccurred())

			vaultPath = writeConfig("prod.vault.yaml", string(vaultYAML))
			basePath = writeConfig("base.yaml", `
codesphere:
  override:
    clientSecret: '{{ secret "oidcClientSecret" }}'
`)
		})

		It("rejects a config without a release name", func() {
			_, err := prepare(basePath)

			Expect(err).To(MatchError(ContainSubstring("found at codesphere.override.clientSecret")))
		})

		It("accepts the secret when an overlay sets the release name", func() {
			overlayPath := writeConfig("overlay.yaml", "codesphere:\n  releaseName: codesphere-preview\n")

			cfg, err := prepare(basePath, overlayPath)

			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Codesphere.Override).To(HaveKeyWithValue("clientSecret", "from-vault"))
		})

		It("accepts the config when an overlay replaces the secret with a literal", func() {
			overlayPath := writeConfig("overlay.yaml", "codesphere:\n  override:\n    clientSecret: literal\n")

			cfg, err := prepare(basePath, overlayPath)

			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Codesphere.Override).To(HaveKeyWithValue("clientSecret", "literal"))
		})
	})

	It("supports a single config path", func() {
		tmpDir := GinkgoT().TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")
		Expect(os.WriteFile(configPath, []byte(fmt.Sprintf(`
dataCenter:
  id: 7
  name: dc-legacy
  city: Legacy City
  countryCode: DE
secrets:
  baseDir: %s
cluster:
  gateway:
    serviceType: LoadBalancer
  publicGateway:
    serviceType: LoadBalancer
codesphere:
  domain: legacy.example.com
  workspaceHostingBaseDomain: ws.legacy.example.com
  customDomains:
    cNameBaseDomain: cname.legacy.example.com
  deployConfig:
    images: {}
`, filepath.ToSlash(tmpDir))), 0644)).To(Succeed())

		opts := &InstallCodesphereOpts{Configs: []string{configPath}}

		effectiveOpts, cfg, cleanup, err := prepareInstallConfig(opts, installer.NewConfig())
		Expect(err).ToNot(HaveOccurred())
		defer cleanup()

		Expect(effectiveOpts.Configs).To(Equal([]string{configPath}))
		Expect(cfg.Datacenter.ID).To(Equal(7))
		Expect(cfg.Datacenter.Name).To(Equal("dc-legacy"))
	})
})

func installCodesphereSopsAndAgeAvailable() bool {
	if _, err := exec.LookPath("sops"); err != nil {
		return false
	}
	if _, err := exec.LookPath("age-keygen"); err != nil {
		return false
	}
	return true
}

var _ = Describe("install codesphere vault type", func() {
	It("accepts sops", func() {
		opts := &InstallCodesphereOpts{VaultType: string(vault.TypeSOPS), PrivKey: "age-key.txt"}
		Expect(validateInstallCodesphereVault(opts)).To(Succeed())
	})

	It("rejects plain vaults at the command boundary", func() {
		opts := &InstallCodesphereOpts{VaultType: string(vault.TypePlain), PrivKey: "age-key.txt"}
		err := validateInstallCodesphereVault(opts)
		Expect(err).To(MatchError(`install codesphere requires vault type "sops"`))
	})
})

var _ = Describe("ensureCodespherePackageForVersion", func() {
	It("accepts an install command without the legacy package flag", func() {
		parent := &cobra.Command{Use: "install"}
		AddInstallCmd(parent, &util.GlobalOptions{})
		child, _, err := parent.Find([]string{"codesphere"})
		Expect(err).NotTo(HaveOccurred())

		child.RunE = func(_ *cobra.Command, _ []string) error { return nil }

		parent.SetArgs([]string{"codesphere", "-c", "config.yaml"})
		Expect(parent.Execute()).To(Succeed())
	})
	It("preserves an explicit package without requiring operations or portal access", func() {
		opts := &InstallCodesphereOpts{Package: "offline-package.tar.gz"}
		Expect(ensureCodespherePackageForVersion(opts, files.RootConfig{}, env.NewMockEnv(GinkgoT()))).To(Succeed())
		Expect(opts.Package).To(Equal("offline-package.tar.gz"))
	})
	It("rejects a missing version before accessing the workdir", func() {
		Expect(ensureCodespherePackageForVersion(&InstallCodesphereOpts{}, files.RootConfig{}, env.NewMockEnv(GinkgoT()))).To(MatchError(ContainSubstring("operations.codesphere is required")))
	})
	It("uses the version from the merged config and preserves configured skip steps", func() {
		dir := GinkgoT().TempDir()
		base := filepath.Join(dir, "base.yaml")
		overlay := filepath.Join(dir, "overlay.yaml")

		Expect(os.WriteFile(base, []byte("operations:\n  codesphere: 1.94.0\n  skip: [ceph]\n"), 0600)).To(Succeed())
		Expect(os.WriteFile(overlay, []byte("operations:\n  codesphere: {version: 1.95.0, hash: abc123}\n"), 0600)).To(Succeed())
		original := &InstallCodesphereOpts{Configs: []string{base, overlay}}
		effective, cfg, cleanup, err := prepareInstallConfig(original, installer.NewConfig())
		Expect(err).NotTo(HaveOccurred())

		defer cleanup()

		build := portal.Build{Version: "codesphere-v1.95.0", Hash: "abc123", Artifacts: []portal.Artifact{{Filename: "installer-lite.tar.gz"}}}
		sum := md5.Sum([]byte("cached package"))
		build.Artifacts[0].Md5Sum = hex.EncodeToString(sum[:])

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()

			switch r.URL.Path {
			case "/packages/codesphere":
				other := build
				other.Hash = "other-build"
				Expect(json.NewEncoder(w).Encode(portal.Builds{Builds: []portal.Build{build, other}})).To(Succeed())
			case "/packages/codesphere/download":
				Expect(r.Header.Get("Range")).To(Equal("bytes=14-"))
				w.WriteHeader(http.StatusPartialContent)
			default:
				Fail("unexpected package portal request: " + r.URL.Path)
			}
		}))
		defer server.Close()

		GinkgoT().Setenv("OMS_PORTAL_API", server.URL)
		GinkgoT().Setenv("OMS_PORTAL_API_KEY", "test-key")

		cached := filepath.Join(dir, build.BuildPackageFilename("installer-lite.tar.gz"))
		Expect(os.WriteFile(cached, []byte("cached package"), 0600)).To(Succeed())

		environment := env.NewMockEnv(GinkgoT())
		environment.EXPECT().GetOmsWorkdir().Return(dir)
		Expect(ensureCodespherePackageForVersion(effective, cfg, environment)).To(Succeed())
		Expect(effective.Package).To(Equal(cached))
		Expect(original.Package).To(BeEmpty())
		Expect(cfg.Operations.Skip).To(Equal([]string{"ceph"}))
	})
})
