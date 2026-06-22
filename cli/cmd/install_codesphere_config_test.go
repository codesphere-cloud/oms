// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer"
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
			ConfigFiles: []string{basePath, overlayPath},
		}

		effectiveOpts, cfg, cleanup, err := prepareInstallConfig(opts, installer.NewConfig())
		Expect(err).ToNot(HaveOccurred())
		defer cleanup()

		Expect(effectiveOpts.Config).ToNot(BeEmpty())
		Expect(effectiveOpts.Config).ToNot(Equal(basePath))
		Expect(effectiveOpts.Config).ToNot(Equal(overlayPath))
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

		_, statErr := os.Stat(effectiveOpts.Config)
		Expect(statErr).ToNot(HaveOccurred())

		cleanup()
		_, statErr = os.Stat(effectiveOpts.Config)
		Expect(os.IsNotExist(statErr)).To(BeTrue())
	})
})
