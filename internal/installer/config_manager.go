// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"io"
	"net"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

type InstallConfigManager interface {
	// Profile management
	ApplyProfile(profile string) error
	// Configuration management
	LoadInstallConfigFromFile(configPath string) error
	LoadVaultFromFile(vaultPath string) error
	ValidateInstallConfig() []string
	ValidateVault() []string
	GetInstallConfig() *files.RootConfig
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

func NewInstallConfigManager() InstallConfigManager {
	return &InstallConfig{
		fileIO: &util.FilesystemWriter{},
		Config: &files.RootConfig{},
		Vault:  &files.InstallVault{},
	}
}

func (g *InstallConfig) LoadInstallConfigFromFile(configPath string) error {
	file, err := g.fileIO.Open(configPath)
	if err != nil {
		return err
	}
	defer util.CloseFileIgnoreError(file)

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", configPath, err)
	}

	config := &files.RootConfig{}
	if err := config.Unmarshal(data); err != nil {
		return fmt.Errorf("failed to unmarshal %s: %w", configPath, err)
	}

	g.Config = config
	return nil
}

func (g *InstallConfig) LoadVaultFromFile(vaultPath string) error {
	vaultFile, err := g.fileIO.Open(vaultPath)
	if err != nil {
		return fmt.Errorf("error opening vault file: %v", err)
	}
	defer util.CloseFileIgnoreError(vaultFile)

	vaultData, err := io.ReadAll(vaultFile)
	if err != nil {
		return fmt.Errorf("failed to read vault.yaml: %v", err)
	}

	vault := &files.InstallVault{}
	if err := vault.Unmarshal(vaultData); err != nil {
		return fmt.Errorf("failed to parse vault.yaml: %v", err)
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

	return errors
}

func (g *InstallConfig) ValidateVault() []string {
	if g.Vault == nil {
		return []string{"vault not set, cannot validate"}
	}

	errors := []string{}
	requiredSecrets := []string{"cephSshPrivateKey", "selfSignedCaKeyPem", "domainAuthPrivateKey", "domainAuthPublicKey"}
	foundSecrets := make(map[string]bool)

	for _, secret := range g.Vault.Secrets {
		foundSecrets[secret.Name] = true
	}

	for _, required := range requiredSecrets {
		if !foundSecrets[required] {
			errors = append(errors, fmt.Sprintf("required secret missing: %s", required))
		}
	}

	return errors
}

func (g *InstallConfig) GetInstallConfig() *files.RootConfig {
	return g.Config
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

	vault := g.Config.ExtractVault()
	vaultYAML, err := vault.Marshal()
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
