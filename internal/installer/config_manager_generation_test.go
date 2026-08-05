// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/codesphere-cloud/oms/internal/installer"
)

var _ = Describe("Generated install config round-trip", func() {
	DescribeTable("profiles generate a valid, reloadable install config",
		func(profile string) {
			manager := installer.NewInstallConfigManager()

			err := manager.ApplyProfile(profile)
			Expect(err).NotTo(HaveOccurred())

			// A freshly generated config must already pass validation without warnings.
			Expect(manager.ValidateInstallConfig()).To(BeEmpty(),
				"profile %s produced an invalid config: %v", profile, manager.ValidateInstallConfig())

			// Secrets generation must succeed (pure Go crypto, no external tools).
			Expect(manager.GenerateSecrets()).To(Succeed())

			// Structural invariant: MetalLB lives under cluster, not at the root.
			config := manager.GetInstallConfig()
			Expect(config.Cluster.MetalLB).NotTo(BeNil(),
				"profile %s must configure cluster.metallb", profile)

			// Write both files to a temp dir.
			dir := GinkgoT().TempDir()
			configPath := filepath.Join(dir, "config.yaml")
			vaultPath := filepath.Join(dir, "prod.vault.yaml")
			Expect(manager.WriteInstallConfig(configPath, false)).To(Succeed())
			Expect(manager.WriteVault(vaultPath, false)).To(Succeed())

			// The written config must be valid YAML with cluster.metallb (not root metallb).
			raw, err := os.ReadFile(configPath)
			Expect(err).NotTo(HaveOccurred())
			var doc map[string]interface{}
			Expect(yaml.Unmarshal(raw, &doc)).To(Succeed())
			Expect(doc).NotTo(HaveKey("metallb"),
				"profile %s must not emit metallb at the root of config.yaml", profile)
			cluster, ok := doc["cluster"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "profile %s must emit a cluster mapping", profile)
			Expect(cluster).To(HaveKey("metallb"))

			// Reload the config from disk through the full render+unmarshal path
			// and re-validate: round-trip must be lossless and valid.
			reloaded := installer.NewInstallConfigManager()
			Expect(reloaded.LoadInstallConfigFromFile(configPath)).To(Succeed())
			Expect(reloaded.ValidateInstallConfig()).To(BeEmpty(),
				"reloaded config for profile %s is invalid: %v", profile, reloaded.ValidateInstallConfig())

			// Reload the (unencrypted) vault and validate all required secrets exist.
			Expect(reloaded.LoadVaultFromUnecryptedFile(vaultPath)).To(Succeed())
			Expect(reloaded.ValidateVault()).To(BeEmpty(),
				"vault for profile %s is invalid: %v", profile, reloaded.ValidateVault())
		},
		Entry("dev profile", installer.PROFILE_DEV),
		Entry("minimal profile", installer.PROFILE_MINIMAL),
		Entry("production profile", installer.PROFILE_PROD),
	)
})
