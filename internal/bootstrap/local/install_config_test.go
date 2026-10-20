// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("local bootstrap install config version", func() {
	DescribeTable("uses config defaults while preserving explicit package choices",
		func(version, hash, local, configuredHash, expectedVersion, expectedHash string) {
			cfg := files.NewRootConfig()
			cfg.SetCodespherePackage("1.111.0", configuredHash)

			manager := installer.NewMockInstallConfigManager(GinkgoT())
			manager.EXPECT().GetInstallConfig().Return(&cfg)
			manager.EXPECT().ApplyProfile("minimal").Return(nil)

			fw := util.NewMockFileIO(GinkgoT())
			fw.EXPECT().Exists("config.yaml").Return(false)
			bootstrapper := &LocalBootstrapper{icg: manager, fw: fw, Env: &CodesphereEnvironment{
				Profile: "minimal", InstallConfigPath: "config.yaml", InstallVersion: version, InstallHash: hash, InstallLocal: local,
			}}
			Expect(bootstrapper.EnsureInstallConfig()).To(Succeed())
			Expect(bootstrapper.Env.InstallVersion).To(Equal(expectedVersion))
			Expect(bootstrapper.Env.InstallHash).To(Equal(expectedHash))
			Expect(bootstrapper.Env.InstallLocal).To(Equal(local))
		},
		Entry("config with hash", "", "", "", "abc123", "1.111.0", "abc123"),
		Entry("config without hash", "", "", "", "", "1.111.0", ""),
		Entry("explicit hash overrides config hash", "", "override", "", "abc123", "1.111.0", "override"),
		Entry("explicit version overrides config", "master", "", "", "abc123", "master", ""),
		Entry("explicit archive overrides config", "", "", "installer.tar.gz", "abc123", "", ""),
	)

	DescribeTable("stores the requested version without losing operation skips", func(hash string) {
		cfg := files.NewRootConfig()
		cfg.Operations = &files.OperationsConfig{Skip: []string{"ceph"}}
		manager := installer.NewMockInstallConfigManager(GinkgoT())
		manager.EXPECT().GetInstallConfig().Return(&cfg)
		manager.EXPECT().ApplyProfile("minimal").Return(nil)

		fw := util.NewMockFileIO(GinkgoT())
		fw.EXPECT().Exists("config.yaml").Return(false)
		bootstrapper := &LocalBootstrapper{icg: manager, fw: fw, Env: &CodesphereEnvironment{
			Profile: "minimal", InstallConfigPath: "config.yaml", InstallVersion: "1.111.0", InstallHash: hash,
		}}
		Expect(bootstrapper.EnsureInstallConfig()).To(Succeed())
		Expect(bootstrapper.Env.InstallConfig.Operations.Codesphere).To(Equal(files.CodespherePackageConfig{Version: "1.111.0", Hash: hash}))
		Expect(bootstrapper.Env.InstallConfig.Operations.Skip).To(Equal([]string{"ceph"}))
	}, Entry("version only", ""), Entry("pinned build", "abc123"))
})

var _ = Describe("local bootstrap install config", func() {
	It("keeps a supplied config without an existing vault", func() {
		dir := GinkgoT().TempDir()

		configPath := filepath.Join(dir, "config.yaml")
		Expect(os.WriteFile(configPath, []byte("codesphere:\n  domain: cs.example.com\n  workspaceHostingBaseDomain: ws.example.com\n  publicIp: 192.0.2.13\n"), 0600)).To(Succeed())

		manager, err := installer.NewInstallConfigManager("plain", "")
		Expect(err).NotTo(HaveOccurred())

		b := &LocalBootstrapper{
			fw:  util.NewFilesystemWriter(),
			icg: manager,
			Env: &CodesphereEnvironment{
				BaseDomain:        "cs.local",
				Profile:           installer.PROFILE_DEV,
				InstallConfigPath: configPath,
				SecretsFilePath:   filepath.Join(dir, "prod.vault.yaml"),
			},
		}
		Expect(b.EnsureInstallConfig()).To(Succeed())
		Expect(b.EnsureSecrets()).To(Succeed())
		Expect(b.Env.InstallConfig.Codesphere.Domain).To(Equal("cs.example.com"))
		Expect(b.Env.InstallConfig.Codesphere.WorkspaceHostingBaseDomain).To(Equal("ws.example.com"))
		Expect(b.Env.InstallConfig.Codesphere.PublicIP).To(Equal("192.0.2.13"))
		Expect(b.Env.Vault).NotTo(BeNil())
		Expect(b.Env.InstallConfig.Codesphere.CertIssuer).NotTo(BeNil())
		Expect(b.Env.InstallConfig.Codesphere.CertIssuer.Type).To(Equal(files.CertIssuerTypeSelfSigned))
	})

	It("keeps a supplied Google issuer and vault through the local profile", func() {
		dir := GinkgoT().TempDir()
		configPath := filepath.Join(dir, "config.yaml")
		vaultPath := filepath.Join(dir, "prod.vault.yaml")

		config := `codesphere:
  domain: cs.example.com
  certIssuer:
    type: acme
    acme:
      enabled: true
      server: https://dv.acme-v02.api.pki.goog/directory
      eabKeyId: default-id
      customDomainsEabKeyId: custom-id
cluster:
  certificates:
    override:
      issuers:
        letsEncryptHttp:
          enabled: false
        acme:
          dnsSolver:
            config:
              cloudDNS:
                project: dns-project
`
		Expect(os.WriteFile(configPath, []byte(config), 0600)).To(Succeed())

		vault := `secrets:
  - name: acmeEabMacKey
    fields:
      password: default-mac
  - name: acmeCustomDomainsEabMacKey
    fields:
      password: custom-mac
`
		Expect(os.WriteFile(vaultPath, []byte(vault), 0600)).To(Succeed())

		manager, err := installer.NewInstallConfigManager("plain", "")
		Expect(err).NotTo(HaveOccurred())

		b := &LocalBootstrapper{
			fw: util.NewFilesystemWriter(), icg: manager,
			Env: &CodesphereEnvironment{BaseDomain: "cs.local", Profile: installer.PROFILE_DEV, InstallConfigPath: configPath, SecretsFilePath: vaultPath},
		}
		Expect(b.EnsureInstallConfig()).To(Succeed())
		Expect(b.EnsureSecrets()).To(Succeed())

		issuer := b.Env.InstallConfig.Codesphere.CertIssuer
		Expect(issuer).NotTo(BeNil())
		Expect(issuer.Type).To(Equal(files.CertIssuerTypeACME))
		Expect(issuer.Acme.EABKeyID).To(Equal("default-id"))

		issuers := b.Env.InstallConfig.Cluster.Certificates.Override["issuers"].(map[string]interface{})
		Expect(issuers["acme"].(map[string]interface{})["dnsSolver"].(map[string]interface{})["config"].(map[string]interface{})["cloudDNS"].(map[string]interface{})["project"]).To(Equal("dns-project"))

		Expect(b.Env.Vault.GetSecret(files.SecretAcmeEabMacKey).Fields.Password).To(Equal("default-mac"))
		Expect(b.Env.Vault.GetSecret(files.SecretAcmeCustomDomainsEabMacKey).Fields.Password).To(Equal("custom-mac"))
	})

	DescribeTable("configures virtual machine apps", func(internal []string, preview, features map[string]bool, enabled bool) {
		vmApps := []string{"kubevirt-operator", "kubevirt-cr", "cdi-operator", "cdi-cr"}
		config := files.NewRootConfig()
		config.Codesphere.Internal = internal
		config.Codesphere.Preview = preview
		config.Codesphere.Features = features
		config.PcApps = files.ChartValues{
			"applications": map[string]any{
				"other-app": map[string]any{"enabled": true},
				"kubevirt-operator": map[string]any{
					"enabled":      false,
					"valuesObject": map[string]any{"custom": "kept"},
				},
			},
		}

		configureVirtualMachineApps(&config)

		apps := config.PcApps["applications"].(map[string]any)
		Expect(apps["other-app"].(map[string]any)["enabled"]).To(BeTrue())
		Expect(apps["kubevirt-operator"].(map[string]any)["valuesObject"].(map[string]any)["custom"]).To(Equal("kept"))

		for _, name := range vmApps {
			app, exists := apps[name]
			if enabled {
				Expect(exists).To(BeTrue(), "%s should be enabled", name)
				Expect(app.(map[string]any)["enabled"]).To(BeTrue(), "%s should be enabled", name)
			} else if name == "kubevirt-operator" {
				Expect(app.(map[string]any)["enabled"]).To(BeFalse())
			} else {
				Expect(exists).To(BeFalse(), "%s should not be added", name)
			}
		}
	},
		Entry("internal flag", []string{"virtual-machines"}, nil, nil, true),
		Entry("preview flag", nil, map[string]bool{"virtual-machines": true}, nil, true),
		Entry("feature flag", nil, nil, map[string]bool{"virtual-machines": true}, true),
		Entry("flag disabled", nil, map[string]bool{"virtual-machines": false}, map[string]bool{"virtual-machines": false}, false),
		Entry("unrelated flags", []string{"other"}, map[string]bool{"other": true}, map[string]bool{"other": true}, false),
		Entry("virtual machines absent", nil, nil, nil, false),
	)
})

var _ = Describe("local bootstrap cluster components", func() {
	It("with the minimal profile and OpenFGA deployed, setting the local cluster components passes the pgOperator validation", func() {
		manager, err := installer.NewInstallConfigManager("plain", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(manager.ApplyProfile(installer.PROFILE_MINIMAL)).To(Succeed())

		setLocalClusterComponents(manager.GetInstallConfig())

		Expect(manager.GetInstallConfig().Cluster.PgOperator.Enabled).To(BeFalse())
		Expect(manager.ValidateInstallConfig()).NotTo(ContainElement(ContainSubstring("pgOperator")))
	})

	It("with the minimal profile, setting the local cluster components keeps the profile's barman plugin", func() {
		manager, err := installer.NewInstallConfigManager("plain", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(manager.ApplyProfile(installer.PROFILE_MINIMAL)).To(Succeed())
		profileBarman := *manager.GetInstallConfig().ManagedServiceBackends.BarmanCloudPlugin

		setLocalClusterComponents(manager.GetInstallConfig())

		Expect(manager.GetInstallConfig().ManagedServiceBackends.BarmanCloudPlugin).To(Equal(&profileBarman))
		Expect(profileBarman.Enabled).To(BeTrue())
	})
})
