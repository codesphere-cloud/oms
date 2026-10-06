// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"debug/elf"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	"go.yaml.in/yaml/v3"
)

const singleVMPackagePath = "/root/installer-lite.tar.gz"
const singleVMPackageDir = "/root/installer-lite"
const singleVMConfigPath = "/root/codesphere/config.yaml"
const singleVMConfigTemplatePath = "/root/codesphere/config-template.yaml"
const singleVMVaultPath = "/root/codesphere/prod.vault.yaml"

// BootstrapSingleVM provisions one Spot VM and runs the local Codesphere installer there.
// It deliberately uses the local bootstrap's Rook and CloudNativePG setup instead of the
// multi-host GCP bootstrap's dedicated Ceph and PostgreSQL machines.
func (b *GCPBootstrapper) BootstrapSingleVM(registryUser, cephDeviceFilter, storageEngine string) error {
	b.Env.SingleVM = true
	b.Env.SpotVMs = true
	b.Env.SpotOnly = true

	b.Env.GoogleACMEIssuer = true
	if err := b.validateSingleVMPackageSource(); err != nil {
		return err
	}

	if err := b.validateInstallVersion(); err != nil {
		return err
	}

	if err := b.validateVMProvisioningOptions(); err != nil {
		return err
	}

	if b.Env.RemoteOmsBinaryPath == "" {
		return fmt.Errorf("--remote-oms-binary is required so the VM runs an OMS build with the single VM bootstrap options")
	}

	if !b.fw.Exists(b.Env.RemoteOmsBinaryPath) {
		return fmt.Errorf("remote OMS binary not found at path: %s", b.Env.RemoteOmsBinaryPath)
	}
	if b.Env.InstallConfigTemplatePath != "" && !b.fw.Exists(b.Env.InstallConfigTemplatePath) {
		return fmt.Errorf("single VM install config template not found at path: %s", b.Env.InstallConfigTemplatePath)
	}
	for _, path := range b.Env.InstallConfigs {
		if !b.fw.Exists(path) {
			return fmt.Errorf("single VM install config not found at path: %s", path)
		}
	}
	if b.Env.InstallVaultPath != "" && !b.fw.Exists(b.Env.InstallVaultPath) {
		return fmt.Errorf("single VM vault not found at path: %s", b.Env.InstallVaultPath)
	}

	omsBinary, err := elf.Open(b.Env.RemoteOmsBinaryPath)
	if err != nil {
		return fmt.Errorf("remote OMS binary must be a Linux amd64 ELF executable: %w", err)
	}

	machine, binaryType := omsBinary.Machine, omsBinary.Type
	if err := omsBinary.Close(); err != nil {
		return fmt.Errorf("failed to close remote OMS binary: %w", err)
	}

	if machine != elf.EM_X86_64 || binaryType != elf.ET_EXEC && binaryType != elf.ET_DYN {
		return fmt.Errorf("remote OMS binary must be a Linux amd64 ELF executable")
	}

	steps := []struct {
		name string
		run  func() error
	}{
		{"Ensure project", b.EnsureProject},
		{"Write infrastructure file", b.WriteInfraFile},
		{"Ensure billing", b.EnsureBilling},
		{"Ensure APIs enabled", b.EnsureAPIsEnabled},
		{"Ensure service accounts", b.EnsureServiceAccounts},
		{"Ensure IAM roles", b.EnsureIAMRoles},
		{"Ensure VPC", b.EnsureVPC},
		{"Ensure firewall rules", b.EnsureFirewallRules},
		{"Ensure single VM", b.EnsureSingleVM},
		{"Prepare data center", b.prepareSingleVMDataCenter},
		{"Write infrastructure file", b.WriteInfraFile},
		{"Ensure root login enabled", func() error { return b.ensureRootLoginEnabledInNode(b.Env.Jumpbox) }},
		{"Configure OMS on VM", b.EnsureJumpboxConfigured},
		{"Configure VM host", b.configureSingleVMHost},
		{"Ensure installer package", b.ensureSingleVMPackage},
		{"Provide install config", b.provideSingleVMConfig},
		{"Install k0s", b.installSingleVMK0s},
		{"Install Codesphere", func() error { return b.installSingleVMCodesphere(registryUser, cephDeviceFilter, storageEngine) }},
		{"Ensure DNS records", b.EnsureDNSRecords},
		{"Write infrastructure file", b.WriteInfraFile},
	}
	for _, step := range steps {
		if err := b.stlog.Step(step.name, step.run); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}

	return nil
}

func (b *GCPBootstrapper) validateSingleVMPackageSource() error {
	if b.Env.InstallLocal == "" && b.Env.InstallVersion == "" {
		return fmt.Errorf("either --install-local or --install-version is required")
	}

	if b.Env.InstallLocal != "" && !strings.HasSuffix(b.Env.InstallLocal, ".tar.gz") && !strings.HasSuffix(b.Env.InstallLocal, ".tgz") {
		return fmt.Errorf("--install-local must be an installer archive (.tar.gz or .tgz)")
	}

	return nil
}

func (b *GCPBootstrapper) prepareSingleVMDataCenter() error {
	if err := b.ensureDataCenters(); err != nil {
		return err
	}

	b.primaryDC().ControlPlaneNodes = []*node.Node{b.Env.Jumpbox}
	ip := b.Env.Jumpbox.GetExternalIP()
	b.primaryDC().GatewayIP = ip
	b.primaryDC().PublicGatewayIP = ip
	b.primaryDC().SSHProxyIP = ip
	b.mirrorPrimaryDataCenter()

	return nil
}

func (b *GCPBootstrapper) configureSingleVMHost() error {
	n := b.Env.Jumpbox
	if !n.HasInotifyWatchesConfigured() {
		if err := n.ConfigureInotifyWatches(); err != nil {
			return fmt.Errorf("configure inotify watches: %w", err)
		}
	}

	if !n.HasMemoryMapConfigured() {
		if err := n.ConfigureMemoryMap(); err != nil {
			return fmt.Errorf("configure memory map limit: %w", err)
		}
	}

	if !n.HasUnprivilegedGatewayPortsConfigured() {
		if err := n.ConfigureUnprivilegedGatewayPorts(); err != nil {
			return fmt.Errorf("configure unprivileged gateway ports: %w", err)
		}
	}

	if err := n.RunSSHCommand("root", "mkdir -p /root/.kube /root/codesphere"); err != nil {
		return fmt.Errorf("create VM installation directories: %w", err)
	}

	return nil
}

func (b *GCPBootstrapper) ensureSingleVMPackage() error {
	if b.Env.InstallLocal != "" {
		if err := b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, b.Env.InstallLocal, singleVMPackagePath); err != nil {
			return fmt.Errorf("copy single VM installer package: %w", err)
		}
	} else {
		filename := portal.BuildPackageFilenameFromParts(b.Env.InstallVersion, b.Env.InstallHash, InstallerArchiveName)

		command := "cd /root && oms download package -f " + InstallerArchiveName +
			" -H " + shellQuote(b.Env.InstallHash) + " " + shellQuote(b.Env.InstallVersion) +
			" && mv -- " + shellQuote(filename) + " " + singleVMPackagePath
		if err := b.Env.Jumpbox.RunSSHCommand("root", command); err != nil {
			return fmt.Errorf("failed to download single VM installer package: %w", err)
		}
	}
	// bootstrap-local skips extraction when this directory exists, regardless of
	// whether the archive was replaced by a newer bundle.
	if err := b.Env.Jumpbox.RunSSHCommand("root", "rm -rf -- "+singleVMPackageDir); err != nil {
		return fmt.Errorf("remove previously extracted single VM installer package: %w", err)
	}

	return nil
}

func (b *GCPBootstrapper) singleVMConfig(config files.RootConfig) files.RootConfig {
	config.Codesphere.Domain = "cs." + b.Env.BaseDomain
	config.Codesphere.WorkspaceHostingBaseDomain = "ws." + b.Env.BaseDomain
	config.Codesphere.PublicIP = b.Env.Jumpbox.GetExternalIP()
	config.Codesphere.CustomDomains.CNameBaseDomain = config.Codesphere.WorkspaceHostingBaseDomain
	config.Cluster.Gateway.ServiceType = "ClusterIP"
	config.Cluster.PublicGateway.ServiceType = "ClusterIP"
	config.PcApps = util.DeepMergeMaps(config.PcApps, files.ChartValues{
		"applications": map[string]any{
			"ssh-workspace-proxy": map[string]any{
				"enabled": true,
				"valuesObject": map[string]any{
					"service": map[string]any{
						"enabled": true,
						"type":    "ClusterIP",
					},
				},
			},
		},
	})

	return config
}

func (b *GCPBootstrapper) provideSingleVMConfig() error {
	if err := b.ensureDataCenters(); err != nil {
		return fmt.Errorf("prepare single VM data center: %w", err)
	}
	if b.Env.InstallVaultPath != "" {
		store, err := vault.New(vault.TypeSOPS, vault.Options{Path: b.Env.InstallVaultPath, AgeKey: b.Env.InstallVaultPrivKey})
		if err != nil {
			return fmt.Errorf("failed to open single VM vault: %w", err)
		}
		loaded, err := store.Load()
		if err != nil {
			return fmt.Errorf("failed to load single VM vault: %w", err)
		}
		for _, secret := range loaded.Secrets {
			b.icg.GetVault().SetSecret(secret)
		}
	}

	dc := b.primaryDC()
	issuerConfig := files.NewRootConfig()
	dc.InstallConfig = &issuerConfig
	if err := b.applyACMEConfig(dc); err != nil {
		return fmt.Errorf("failed to configure Google Public CA for single VM: %w", err)
	}

	config, err := b.loadSingleVMBaseConfig()
	if err != nil {
		return err
	}
	config = b.singleVMConfig(config)
	config.Codesphere.CertIssuer = issuerConfig.Codesphere.CertIssuer
	config.Cluster.Certificates.Override = util.DeepMergeMaps(config.Cluster.Certificates.Override, issuerConfig.Cluster.Certificates.Override)
	dc.InstallConfig = &config

	b.mirrorPrimaryDataCenter()

	data, err := config.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal single VM install config: %w", err)
	}

	if err := b.fw.MkdirAll(b.Env.OmsWorkdir, 0755); err != nil {
		return fmt.Errorf("failed to create OMS workdir: %w", err)
	}

	path := filepath.Join(b.Env.OmsWorkdir, "single-vm-config.yaml")
	if err := b.fw.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write single VM install config: %w", err)
	}

	if err := b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, path, singleVMConfigPath); err != nil {
		return fmt.Errorf("failed to copy single VM install config: %w", err)
	}

	vaultData, err := b.icg.GetVault().Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal single VM vault: %w", err)
	}

	vaultPath := filepath.Join(b.Env.OmsWorkdir, "single-vm-vault.yaml")
	if err := b.fw.WriteFile(vaultPath, vaultData, 0600); err != nil {
		return fmt.Errorf("failed to write single VM vault: %w", err)
	}

	if err := b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, vaultPath, singleVMVaultPath); err != nil {
		return fmt.Errorf("failed to copy single VM vault: %w", err)
	}

	return nil
}

func (b *GCPBootstrapper) loadSingleVMBaseConfig() (files.RootConfig, error) {
	paths := append([]string(nil), b.Env.InstallConfigs...)
	if b.Env.InstallConfigTemplatePath != "" {
		paths = append([]string{b.Env.InstallConfigTemplatePath}, paths...)
	}
	config := files.NewRootConfig()
	if len(paths) == 0 {
		return config, nil
	}
	store := vault.NewVaultTemplatingSecretStore(b.icg.GetVault())
	merged := map[string]any{}
	for i, path := range paths {
		data, err := b.fw.ReadFile(path)
		if err != nil {
			return config, fmt.Errorf("failed to read single VM config %s: %w", path, err)
		}
		data, err = configtemplating.RenderInstallConfigTemplate(data, store)
		if err != nil {
			return config, fmt.Errorf("failed to render single VM config %s: %w", path, err)
		}
		var partial map[string]any
		if err := yaml.Unmarshal(data, &partial); err != nil {
			return config, fmt.Errorf("failed to parse single VM config %s: %w", path, err)
		}
		merged = util.DeepMergeMaps(merged, partial)
		remotePath := fmt.Sprintf("/root/codesphere/config-input-%d.yaml", i)
		if path == b.Env.InstallConfigTemplatePath {
			remotePath = singleVMConfigTemplatePath
		}
		if err := b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, path, remotePath); err != nil {
			return config, fmt.Errorf("failed to copy single VM config %s: %w", path, err)
		}
	}
	data, err := yaml.Marshal(merged)
	if err != nil {
		return config, fmt.Errorf("failed to marshal merged single VM config: %w", err)
	}
	if err := config.Unmarshal(data); err != nil {
		return config, fmt.Errorf("failed to load merged single VM config: %w", err)
	}
	return config, nil
}

func (b *GCPBootstrapper) installSingleVMK0s() error {
	if err := b.Env.Jumpbox.RunSSHCommand("root", "oms install k0s --single"); err != nil {
		return fmt.Errorf("failed to install single-node k0s: %w", err)
	}

	return nil
}

func (b *GCPBootstrapper) installSingleVMCodesphere(registryUser, cephDeviceFilter, storageEngine string) error {
	n := b.Env.Jumpbox
	if err := n.RunSSHCommand("root", "test -n \"$OMS_REGISTRY_PASSWORD\""); err != nil {
		return fmt.Errorf("registry password was not forwarded to the VM: %w", err)
	}

	args := []string{
		"oms beta bootstrap-local --yes --k0s",
		"--install-local " + singleVMPackagePath,
		"--install-dir /root/codesphere",
		"--install-config " + singleVMConfigPath,
		"--registry-user " + shellQuote(registryUser),
		"--ceph-device-filter " + shellQuote(cephDeviceFilter),
		"--storage-engine " + shellQuote(storageEngine),
		"--expose-shared",
	}

	command := "KUBECONFIG=/root/.kube/config " + strings.Join(args, " ")
	if err := n.RunSSHCommand("root", command); err != nil {
		return fmt.Errorf("local Codesphere bootstrap failed: %w", err)
	}

	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
