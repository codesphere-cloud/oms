// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

func TestSuppliedConfigWorksWithoutExistingVault(t *testing.T) {
	dir := t.TempDir()

	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("codesphere:\n  domain: cs.example.com\n  workspaceHostingBaseDomain: ws.example.com\n  publicIp: 192.0.2.13\n"), 0600); err != nil {
		t.Fatal(err)
	}

	manager, err := installer.NewInstallConfigManager("plain", "")
	if err != nil {
		t.Fatal(err)
	}

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
	if err := b.EnsureInstallConfig(); err != nil {
		t.Fatal(err)
	}

	if err := b.EnsureSecrets(); err != nil {
		t.Fatal(err)
	}

	if b.Env.InstallConfig.Codesphere.Domain != "cs.example.com" || b.Env.InstallConfig.Codesphere.WorkspaceHostingBaseDomain != "ws.example.com" || b.Env.InstallConfig.Codesphere.PublicIP != "192.0.2.13" {
		t.Fatalf("supplied config was not retained: %+v", b.Env.InstallConfig.Codesphere)
	}

	if b.Env.Vault == nil {
		t.Fatal("expected an empty vault for the first installation")
	}

	if b.Env.InstallConfig.Codesphere.CertIssuer == nil || b.Env.InstallConfig.Codesphere.CertIssuer.Type != files.CertIssuerTypeSelfSigned {
		t.Fatalf("expected the local default to be self signed, got %+v", b.Env.InstallConfig.Codesphere.CertIssuer)
	}
}

func TestSuppliedGoogleIssuerAndVaultSurviveLocalProfile(t *testing.T) {
	dir := t.TempDir()
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
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}

	vault := `secrets:
  - name: acmeEabMacKey
    fields:
      password: default-mac
  - name: acmeCustomDomainsEabMacKey
    fields:
      password: custom-mac
`
	if err := os.WriteFile(vaultPath, []byte(vault), 0600); err != nil {
		t.Fatal(err)
	}

	manager, err := installer.NewInstallConfigManager("plain", "")
	if err != nil {
		t.Fatal(err)
	}

	b := &LocalBootstrapper{
		fw: util.NewFilesystemWriter(), icg: manager,
		Env: &CodesphereEnvironment{BaseDomain: "cs.local", Profile: installer.PROFILE_DEV, InstallConfigPath: configPath, SecretsFilePath: vaultPath},
	}
	if err := b.EnsureInstallConfig(); err != nil {
		t.Fatal(err)
	}

	if err := b.EnsureSecrets(); err != nil {
		t.Fatal(err)
	}

	issuer := b.Env.InstallConfig.Codesphere.CertIssuer
	if issuer == nil || issuer.Type != files.CertIssuerTypeACME || issuer.Acme.EABKeyID != "default-id" {
		t.Fatalf("Google issuer was not preserved: %+v", issuer)
	}

	issuers := b.Env.InstallConfig.Cluster.Certificates.Override["issuers"].(map[string]interface{})
	if issuers["acme"].(map[string]interface{})["dnsSolver"].(map[string]interface{})["config"].(map[string]interface{})["cloudDNS"].(map[string]interface{})["project"] != "dns-project" {
		t.Fatalf("Cloud DNS solver was not preserved: %+v", issuers)
	}

	if b.Env.Vault.GetSecret(files.SecretAcmeEabMacKey).Fields.Password != "default-mac" || b.Env.Vault.GetSecret(files.SecretAcmeCustomDomainsEabMacKey).Fields.Password != "custom-mac" {
		t.Fatal("Google EAB MAC keys were not loaded")
	}
}

func TestConfigureVirtualMachineApps(t *testing.T) {
	vmApps := []string{"kubevirt-operator", "kubevirt-cr", "cdi-operator", "cdi-cr"}

	for _, tc := range []struct {
		name     string
		internal []string
		preview  map[string]bool
		features map[string]bool
		enabled  bool
	}{
		{name: "internal flag", internal: []string{"virtual-machines"}, enabled: true},
		{name: "preview flag", preview: map[string]bool{"virtual-machines": true}, enabled: true},
		{name: "feature flag", features: map[string]bool{"virtual-machines": true}, enabled: true},
		{name: "flag disabled", preview: map[string]bool{"virtual-machines": false}, features: map[string]bool{"virtual-machines": false}},
		{name: "unrelated flags", internal: []string{"other"}, preview: map[string]bool{"other": true}, features: map[string]bool{"other": true}},
		{name: "virtual machines absent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := files.NewRootConfig()
			config.Codesphere.Internal = tc.internal
			config.Codesphere.Preview = tc.preview
			config.Codesphere.Features = tc.features
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
			if apps["other-app"].(map[string]any)["enabled"] != true {
				t.Fatal("unrelated pc-apps entry was changed")
			}
			if apps["kubevirt-operator"].(map[string]any)["valuesObject"].(map[string]any)["custom"] != "kept" {
				t.Fatal("existing KubeVirt values were changed")
			}
			for _, name := range vmApps {
				app, exists := apps[name]
				if tc.enabled {
					if !exists || app.(map[string]any)["enabled"] != true {
						t.Errorf("%s should be enabled", name)
					}
				} else if name == "kubevirt-operator" {
					if app.(map[string]any)["enabled"] != false {
						t.Errorf("%s should retain its existing setting", name)
					}
				} else if exists {
					t.Errorf("%s should not be added", name)
				}
			}
		})
	}
}
