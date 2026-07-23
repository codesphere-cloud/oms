// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("prepareInstallConfig", func() {
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
		Expect(vault.EncryptFileWithSOPS(plaintextVaultPath, vaultPath, strings.TrimSpace(string(recipient)))).To(Succeed())

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
