// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/util"
)

func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

//mockery:generate: true
type InstallConfigManager interface {
	// Profile management
	ApplyProfile(profile string) error

	// Imports
	FetchFromAnsibleInventory(inventoryPath string) error

	// Configuration management
	LoadInstallConfigFromFile(configPath string) error
	LoadVaultFromFile(vaultPath string) error
	LoadVaultFromUnecryptedFile(vaultPath string) error
	ValidateInstallConfig() []string
	ValidateVault() []string
	GetInstallConfig() *files.RootConfig
	GetVault() *files.InstallVault
	GetSecretFilePath() string
	CollectInteractively() error

	// Output
	GenerateSecrets() error
	WriteInstallConfig(configPath string, withComments bool) error
	WriteVault(vaultPath string, withComments bool) error
}

type InstallConfig struct {
	fileIO util.FileIO
	Config *files.RootConfig
	Vault  *files.InstallVault
}

// SetFileIO overrides the file I/O implementation (useful for testing).
func (g *InstallConfig) SetFileIO(fio util.FileIO) {
	g.fileIO = fio
}

func NewInstallConfigManager() InstallConfigManager {
	config := files.NewRootConfig()
	return &InstallConfig{
		fileIO: &util.FilesystemWriter{},
		Config: &config,
		Vault:  &files.InstallVault{},
	}
}

func (g *InstallConfig) LoadInstallConfigFromFile(configPath string) error {
	data, err := g.fileIO.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", configPath, err)
	}

	store := vault.NewVaultTemplatingSecretStore(g.Vault)
	data, err = configtemplating.RenderInstallConfigTemplate(data, store)
	if err != nil {
		return err
	}

	config := files.NewRootConfig()
	if err := config.Unmarshal(data); err != nil {
		return fmt.Errorf("failed to unmarshal %s: %w", configPath, err)
	}

	g.Config = &config
	return nil
}

func (g *InstallConfig) LoadVaultFromFile(vaultPath string) error {
	vault, err := vault.LoadVaultData(vaultPath, "")
	if err != nil {
		return err
	}

	g.Vault = vault
	return nil
}

func (g *InstallConfig) LoadVaultFromUnecryptedFile(vaultPath string) error {
	vault, err := vault.LoadUnencryptedVaultData(vaultPath)
	if err != nil {
		return err
	}

	g.Vault = vault
	return nil
}

func (g *InstallConfig) ValidateInstallConfig() []string {
	if g.Config == nil {
		return []string{"config not set, cannot validate"}
	}

	errors := []string{}

	if g.Config.Datacenter.ID == 0 {
		errors = append(errors, "datacenter ID is required")
	}
	if g.Config.Datacenter.Name == "" {
		errors = append(errors, "datacenter name is required")
	}

	if g.Config.Postgres.Mode == "" {
		errors = append(errors, "postgres mode is required (install or external)")
	} else if g.Config.Postgres.Mode != "install" && g.Config.Postgres.Mode != "external" {
		errors = append(errors, fmt.Sprintf("invalid postgres mode: %s (must be 'install' or 'external')", g.Config.Postgres.Mode))
	}

	switch g.Config.Postgres.Mode {
	case "install":
		if g.Config.Postgres.Primary == nil {
			errors = append(errors, "postgres primary configuration is required when mode is 'install'")
		} else {
			if g.Config.Postgres.Primary.IP == "" {
				errors = append(errors, "postgres primary IP is required")
			}
			if g.Config.Postgres.Primary.Hostname == "" {
				errors = append(errors, "postgres primary hostname is required")
			}
		}
	case "external":
		if g.Config.Postgres.ServerAddress == "" {
			errors = append(errors, "postgres server address is required when mode is 'external'")
		}
	}

	if len(g.Config.Ceph.Hosts) == 0 {
		errors = append(errors, "at least one Ceph host is required")
	}
	for _, host := range g.Config.Ceph.Hosts {
		if !IsValidIP(host.IPAddress) {
			errors = append(errors, fmt.Sprintf("invalid Ceph host IP: %s", host.IPAddress))
		}
	}

	if g.Config.Kubernetes.ManagedByCodesphere {
		if len(g.Config.Kubernetes.ControlPlanes) == 0 {
			errors = append(errors, "at least one K8s control plane node is required")
		}
	} else {
		if g.Config.Kubernetes.PodCIDR == "" {
			errors = append(errors, "pod CIDR is required for external Kubernetes")
		}
		if g.Config.Kubernetes.ServiceCIDR == "" {
			errors = append(errors, "service CIDR is required for external Kubernetes")
		}
	}

	if g.Config.Codesphere.Domain == "" {
		errors = append(errors, "Codesphere domain is required")
	}

	if g.Config.Codesphere.OpenBao != nil {
		if g.Config.Codesphere.OpenBao.URI == "" {
			errors = append(errors, "OpenBao URI is required when OpenBao integration is enabled")
		}
		if _, err := url.ParseRequestURI(g.Config.Codesphere.OpenBao.URI); err != nil {
			errors = append(errors, "OpenBao URI must be a valid URL")
		}
		if g.Config.Codesphere.OpenBao.Engine == "" {
			errors = append(errors, "OpenBao engine name is required when OpenBao integration is enabled")
		}
		if g.Config.Codesphere.OpenBao.User == "" {
			errors = append(errors, "OpenBao username is required when OpenBao integration is enabled")
		}
	}

	return errors
}

func (g *InstallConfig) ValidateVault() []string {
	if g.Vault == nil {
		return []string{"vault not set, cannot validate"}
	}

	errors := []string{}
	requiredSecrets := []string{files.SecretTokenPrivateKey, files.SecretTokenPublicKey, files.SecretCephSshPrivateKey, files.SecretSelfSignedCaKeyPem, files.SecretDomainAuthPrivateKey, files.SecretDomainAuthPublicKey}
	foundSecrets := make(map[string]bool)

	for _, secret := range g.Vault.Secrets {
		foundSecrets[secret.Name] = true
	}

	for _, required := range requiredSecrets {
		if !foundSecrets[required] {
			errors = append(errors, fmt.Sprintf("required secret missing: %s", required))
		}
	}

	if g.Config.Codesphere.OpenBao != nil {
		if !foundSecrets[files.SecretOpenBaoPassword] {
			errors = append(errors, "required OpenBao secret missing: openBaoPassword")
		}
	}

	return errors
}

func (g *InstallConfig) GetInstallConfig() *files.RootConfig {
	return g.Config
}

func (g *InstallConfig) GetVault() *files.InstallVault {
	return g.Vault
}

func (g *InstallConfig) GetSecretFilePath() string {
	return filepath.Join(g.Config.Secrets.BaseDir, "prod.vault.yaml")
}

func (g *InstallConfig) WriteInstallConfig(configPath string, withComments bool) error {
	if g.Config == nil {
		return fmt.Errorf("no configuration provided - config is nil")
	}

	configYAML, err := g.Config.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal config.yaml: %w", err)
	}

	if withComments {
		configYAML = AddConfigComments(configYAML)
	}

	if err := g.fileIO.CreateAndWrite(configPath, configYAML, "Configuration"); err != nil {
		return err
	}

	return nil
}

func (g *InstallConfig) WriteVault(vaultPath string, withComments bool) error {
	if g.Config == nil {
		return fmt.Errorf("no configuration provided - config is nil")
	}
	if g.Vault == nil {
		g.Vault = &files.InstallVault{}
	}

	vaultYAML, err := g.Vault.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal vault.yaml: %w", err)
	}

	if withComments {
		vaultYAML = AddVaultComments(vaultYAML)
	}

	if err := g.fileIO.CreateAndWrite(vaultPath, vaultYAML, "Secrets"); err != nil {
		return err
	}

	return nil
}

func AddConfigComments(yamlData []byte) []byte {
	header := `# Codesphere Installer Configuration
# Generated by OMS CLI
#
# This file contains the main configuration for installing Codesphere Private Cloud.
# Review and modify as needed before running the installer.
#
# For more information, see the installation documentation.

`
	return append([]byte(header), yamlData...)
}

func AddVaultComments(yamlData []byte) []byte {
	header := `# Codesphere Installer Secrets
# Generated by OMS CLI
#
# IMPORTANT: This file contains sensitive information!
#
# Before storing or transmitting this file:
# 1. Install SOPS and Age: brew install sops age
# 2. Generate an Age keypair: age-keygen -o age_key.txt
# 3. Encrypt this file:
#    age-keygen -y age_key.txt  # Get public key
#    sops --encrypt --age <PUBLIC_KEY> --in-place prod.vault.yaml
#
# Keep the Age private key (age_key.txt) extremely secure!
#
# To edit the encrypted file later:
#    export SOPS_AGE_KEY_FILE=/path/to/age_key.txt
#    sops prod.vault.yaml

`
	return append([]byte(header), yamlData...)
}

func (g *InstallConfig) ApplyProfile(profile string) error {
	g.applyCommonProperties()

	switch profile {
	case PROFILE_DEV, PROFILE_DEVELOPMENT:
		return g.applyProfileDev()
	case PROFILE_PROD, PROFILE_PRODUCTION:
		return g.applyProfileProd()

	case PROFILE_MINIMAL:
		return g.applyProfileMinimal()
	}
	return fmt.Errorf("unknown profile: %s, available profiles: dev, prod, minimal", profile)
}
