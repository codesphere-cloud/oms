// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/stretchr/testify/mock"
	"go.yaml.in/yaml/v3"
)

func TestSingleVMProvidesGoogleIssuerConfigAndVault(t *testing.T) {
	dir := t.TempDir()

	manager, err := installer.NewInstallConfigManager("plain", "")
	if err != nil {
		t.Fatal(err)
	}

	client := NewMockGCPClientManager(t)
	client.EXPECT().CreatePublicCAExternalAccountKey("project").Return("default-id", "default-mac", nil).Once()
	client.EXPECT().CreatePublicCAExternalAccountKey("project").Return("custom-id", "custom-mac", nil).Once()

	nodeClient := node.NewMockNodeClient(t)
	n := &node.Node{NodeClient: nodeClient, ExternalIP: "192.0.2.13"}
	nodeClient.EXPECT().CopyFile(n, filepath.Join(dir, "single-vm-config.yaml"), singleVMConfigPath).Return(nil)
	nodeClient.EXPECT().CopyFile(n, filepath.Join(dir, "single-vm-vault.yaml"), singleVMVaultPath).Return(nil)

	b := &GCPBootstrapper{
		Env:       &CodesphereEnvironment{BaseDomain: "example.com", ProjectID: "project", DNSProjectID: "dns-project", OmsWorkdir: dir, Jumpbox: n, GoogleACMEIssuer: true},
		GCPClient: client,
		icg:       manager,
		fw:        util.NewFilesystemWriter(),
	}
	if err := b.provideSingleVMConfig(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "single-vm-config.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	config := files.NewRootConfig()
	if err := config.Unmarshal(data); err != nil {
		t.Fatal(err)
	}

	manager.SetInstallConfig(&config)

	if err := manager.ApplyProfile(installer.PROFILE_DEV); err != nil {
		t.Fatal(err)
	}

	issuer := config.Codesphere.CertIssuer
	if issuer == nil || issuer.Type != files.CertIssuerTypeACME || issuer.Acme.Server != "https://dv.acme-v02.api.pki.goog/directory" || issuer.Acme.EABKeyID != "default-id" || issuer.Acme.CustomDomainsEABKeyID != "custom-id" {
		t.Fatalf("Google issuer was not preserved by the local profile: %+v", issuer)
	}

	issuers, ok := config.Cluster.Certificates.Override["issuers"].(map[string]interface{})
	if !ok {
		t.Fatalf("certificate override missing after local profile: %s", data)
	}

	if issuers["letsEncryptHttp"].(map[string]interface{})["enabled"] != false || issuers["acme"].(map[string]interface{})["dnsSolver"].(map[string]interface{})["config"].(map[string]interface{})["cloudDNS"].(map[string]interface{})["project"] != "dns-project" {
		t.Fatalf("unexpected certificate solver config: %+v", issuers)
	}

	vaultData, err := os.ReadFile(filepath.Join(dir, "single-vm-vault.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	var vault files.InstallVault
	if err := yaml.Unmarshal(vaultData, &vault); err != nil {
		t.Fatal(err)
	}

	if vault.GetSecret(files.SecretAcmeEabMacKey).Fields.Password != "default-mac" || vault.GetSecret(files.SecretAcmeCustomDomainsEabMacKey).Fields.Password != "custom-mac" {
		t.Fatalf("Google EAB keys missing from vault")
	}
}

func TestSingleVMUsesUploadedConfigTemplateAsBase(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "base.yaml")
	template := []byte("dataCenter:\n  name: custom\npcApps:\n  applications:\n    ssh-workspace-proxy:\n      valuesObject:\n        service:\n          port: 2222\n")
	if err := os.WriteFile(templatePath, template, 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := installer.NewInstallConfigManager("plain", "")
	if err != nil {
		t.Fatal(err)
	}
	client := NewMockGCPClientManager(t)
	client.EXPECT().CreatePublicCAExternalAccountKey("project").Return("default-id", "default-mac", nil).Once()
	client.EXPECT().CreatePublicCAExternalAccountKey("project").Return("custom-id", "custom-mac", nil).Once()
	nodeClient := node.NewMockNodeClient(t)
	n := &node.Node{NodeClient: nodeClient, ExternalIP: "192.0.2.13"}
	nodeClient.EXPECT().CopyFile(n, templatePath, singleVMConfigTemplatePath).Return(nil).Once()
	nodeClient.EXPECT().CopyFile(n, filepath.Join(dir, "single-vm-config.yaml"), singleVMConfigPath).Return(nil).Once()
	nodeClient.EXPECT().CopyFile(n, filepath.Join(dir, "single-vm-vault.yaml"), singleVMVaultPath).Return(nil).Once()
	b := &GCPBootstrapper{
		Env: &CodesphereEnvironment{BaseDomain: "example.com", ProjectID: "project", OmsWorkdir: dir,
			Jumpbox: n, GoogleACMEIssuer: true, InstallConfigTemplatePath: templatePath},
		GCPClient: client, icg: manager, fw: util.NewFilesystemWriter(),
	}
	if err := b.provideSingleVMConfig(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "single-vm-config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	config := files.NewRootConfig()
	if err := config.Unmarshal(data); err != nil {
		t.Fatal(err)
	}
	if config.Datacenter.Name != "custom" || config.Codesphere.Domain != "cs.example.com" || config.Codesphere.PublicIP != "192.0.2.13" {
		t.Fatalf("template or VM settings missing: %+v", config)
	}
	service := config.PcApps["applications"].(map[string]any)["ssh-workspace-proxy"].(map[string]any)["valuesObject"].(map[string]any)["service"].(map[string]any)
	if service["port"] != 2222 || service["type"] != "ClusterIP" {
		t.Fatalf("template service values or VM overrides missing: %+v", service)
	}
}

func TestSingleVMMergesRepeatedConfigsInOrder(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.yaml")
	second := filepath.Join(dir, "second.yaml")
	if err := os.WriteFile(first, []byte("dataCenter:\n  name: first\n  city: '{{ secret \"dcCity\" }}'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("dataCenter:\n  name: second\n"), 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := installer.NewInstallConfigManager("plain", "")
	if err != nil {
		t.Fatal(err)
	}
	manager.GetVault().SetSecret(files.SecretEntry{Name: "dcCity", File: &files.SecretFile{Content: "Berlin"}})
	nodeClient := node.NewMockNodeClient(t)
	n := &node.Node{NodeClient: nodeClient}
	nodeClient.EXPECT().CopyFile(n, first, "/root/codesphere/config-input-0.yaml").Return(nil).Once()
	nodeClient.EXPECT().CopyFile(n, second, "/root/codesphere/config-input-1.yaml").Return(nil).Once()
	b := &GCPBootstrapper{Env: &CodesphereEnvironment{InstallConfigs: []string{first, second}, Jumpbox: n}, icg: manager, fw: util.NewFilesystemWriter()}
	config, err := b.loadSingleVMBaseConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Datacenter.Name != "second" || config.Datacenter.City != "Berlin" {
		t.Fatalf("repeated configs were not merged in order: %+v", config.Datacenter)
	}
}

func TestSingleVMConfigSurvivesLocalProfile(t *testing.T) {
	b := &GCPBootstrapper{Env: &CodesphereEnvironment{
		BaseDomain:      "example.com",
		GatewayIP:       "192.0.2.10",
		PublicGatewayIP: "192.0.2.11",
		SshProxyIP:      "192.0.2.12",
		Jumpbox:         &node.Node{ExternalIP: "192.0.2.13"},
	}}
	seed := b.singleVMConfig(files.NewRootConfig())

	data, err := seed.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	config := files.NewRootConfig()
	if err := config.Unmarshal(data); err != nil {
		t.Fatal(err)
	}

	manager, err := installer.NewInstallConfigManager("plain", "")
	if err != nil {
		t.Fatal(err)
	}

	manager.SetInstallConfig(&config)

	if err := manager.ApplyProfile(installer.PROFILE_DEV); err != nil {
		t.Fatal(err)
	}

	if config.Codesphere.Domain != "cs.example.com" || config.Codesphere.WorkspaceHostingBaseDomain != "ws.example.com" || config.Codesphere.PublicIP != "192.0.2.13" || config.Codesphere.CustomDomains.CNameBaseDomain != "ws.example.com" {
		t.Fatalf("unexpected Codesphere settings: %+v", config.Codesphere)
	}

	if config.Cluster.Gateway.ServiceType != "ClusterIP" || config.Cluster.PublicGateway.ServiceType != "ClusterIP" {
		t.Fatalf("downstream gateways must be internal: %+v", config.Cluster)
	}

	apps := config.PcApps["applications"].(map[string]any)
	proxy := apps["ssh-workspace-proxy"].(map[string]any)

	service := proxy["valuesObject"].(map[string]any)["service"].(map[string]any)
	if service["type"] != "ClusterIP" {
		t.Fatalf("SSH proxy must be internal: %+v", service)
	}
}

func TestSingleVMUsesVMAddressForAllPublicNames(t *testing.T) {
	b := &GCPBootstrapper{Env: &CodesphereEnvironment{
		BaseDomain: "example.com",
		Jumpbox:    &node.Node{ExternalIP: "192.0.2.13"},
	}}
	if err := b.prepareSingleVMDataCenter(); err != nil {
		t.Fatal(err)
	}

	if b.Env.GatewayIP != "192.0.2.13" || b.Env.PublicGatewayIP != "192.0.2.13" || b.Env.SshProxyIP != "192.0.2.13" {
		t.Fatalf("expected one public IP for all DNS targets: %#v", b.Env)
	}
}

func TestSingleVMPackageSources(t *testing.T) {
	for _, tc := range []struct {
		name    string
		local   string
		version string
		wantErr bool
	}{
		{name: "missing", wantErr: true},
		{name: "local archive", local: "installer-lite.tar.gz"},
		{name: "version", version: "v1.2.3"},
		{name: "invalid local archive", local: "installer.zip", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &GCPBootstrapper{Env: &CodesphereEnvironment{InstallLocal: tc.local, InstallVersion: tc.version}}
			if err := b.validateSingleVMPackageSource(); (err != nil) != tc.wantErr {
				t.Fatalf("validation error = %v, want error %t", err, tc.wantErr)
			}
		})
	}
}

func TestSingleVMDownloadsVersionedInstaller(t *testing.T) {
	client := node.NewMockNodeClient(t)
	n := &node.Node{NodeClient: client}
	b := &GCPBootstrapper{Env: &CodesphereEnvironment{
		InstallVersion: "v1.2.3",
		InstallHash:    "abc123",
		Jumpbox:        n,
	}}

	client.EXPECT().RunCommand(mock.Anything, "root", "cd /root && oms download package -f installer-lite.tar.gz -H 'abc123' 'v1.2.3' && mv -- 'v1.2.3-abc123-installer-lite.tar.gz' /root/installer-lite.tar.gz").Return(nil)
	client.EXPECT().RunCommand(mock.Anything, "root", "rm -rf -- /root/installer-lite").Return(nil)

	if err := b.ensureSingleVMPackage(); err != nil {
		t.Fatal(err)
	}
}

func TestSingleVMCopiesLocalInstaller(t *testing.T) {
	client := node.NewMockNodeClient(t)
	n := &node.Node{NodeClient: client}
	b := &GCPBootstrapper{Env: &CodesphereEnvironment{InstallLocal: "installer-lite.tar.gz", Jumpbox: n}}
	client.EXPECT().CopyFile(n, "installer-lite.tar.gz", singleVMPackagePath).Return(nil)
	client.EXPECT().RunCommand(mock.Anything, "root", "rm -rf -- /root/installer-lite").Return(nil)

	if err := b.ensureSingleVMPackage(); err != nil {
		t.Fatal(err)
	}
}

func TestSingleVMInstallsBundledK0s(t *testing.T) {
	client := node.NewMockNodeClient(t)
	n := &node.Node{NodeClient: client}
	b := &GCPBootstrapper{Env: &CodesphereEnvironment{Jumpbox: n}}

	client.EXPECT().RunCommand(mock.Anything, "root", "oms install k0s --single").Return(nil)

	if err := b.installSingleVMK0s(); err != nil {
		t.Fatal(err)
	}
}

func TestSingleVMForwardsStorageEngine(t *testing.T) {
	client := node.NewMockNodeClient(t)
	n := &node.Node{NodeClient: client}
	b := &GCPBootstrapper{Env: &CodesphereEnvironment{Jumpbox: n}}
	client.EXPECT().RunCommand(n, "root", "test -n \"$OMS_REGISTRY_PASSWORD\"").Return(nil)
	client.EXPECT().RunCommand(n, "root", mock.MatchedBy(func(command string) bool {
		return strings.Contains(command, "--storage-engine 'local'") &&
			strings.Contains(command, "--ceph-device-filter '^sdb$'")
	})).Return(nil)

	if err := b.installSingleVMCodesphere("user", "^sdb$", "local"); err != nil {
		t.Fatal(err)
	}
}

func TestSingleVMConfiguresHostPortsBeforeInstall(t *testing.T) {
	client := node.NewMockNodeClient(t)
	n := &node.Node{NodeClient: client}

	b := &GCPBootstrapper{Env: &CodesphereEnvironment{Jumpbox: n}}
	for _, command := range []string{
		"sudo grep -E '^fs.inotify.max_user_watches=1048576' /etc/sysctl.conf >/dev/null 2>&1",
		"sudo sysctl -n fs.inotify.max_user_watches | grep -q '^1048576$'",
		"sudo grep -E '^fs.inotify.max_user_instances=8192' /etc/sysctl.conf >/dev/null 2>&1",
		"sudo sysctl -n fs.inotify.max_user_instances | grep -q '^8192$'",
		"sudo grep -E '^vm.max_map_count=262144' /etc/sysctl.conf >/dev/null 2>&1",
		"sudo sysctl -n vm.max_map_count | grep -q '^262144$'",
	} {
		client.EXPECT().RunCommand(n, "root", command).Return(nil).Once()
	}

	client.EXPECT().RunCommand(n, "root", "sudo grep -E '^net.ipv4.ip_unprivileged_port_start=80' /etc/sysctl.conf >/dev/null 2>&1").Return(errors.New("not configured")).Twice()
	client.EXPECT().RunCommand(n, "root", "echo 'net.ipv4.ip_unprivileged_port_start=80' | sudo tee -a /etc/sysctl.conf").Return(nil).Once()
	client.EXPECT().RunCommand(n, "root", "sudo sysctl -p").Return(nil).Once()
	client.EXPECT().RunCommand(n, "root", "mkdir -p /root/.kube /root/codesphere").Return(nil).Once()

	if err := b.configureSingleVMHost(); err != nil {
		t.Fatal(err)
	}
}
