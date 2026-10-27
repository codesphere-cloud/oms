// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/util"
	"go.yaml.in/yaml/v3"
)

// InstallVaultLoader reads the encrypted seed vault supplied to either bootstrap mode.
//
//mockery:generate: true
type InstallVaultLoader interface {
	Load(path, privateKey string) (*files.InstallVault, error)
}

type sopsInstallVaultLoader struct {
	fileIO util.FileIO
}

func (l *sopsInstallVaultLoader) Load(path, privateKey string) (*files.InstallVault, error) {
	store, err := vault.New(vault.TypeSOPS, vault.Options{Path: path, AgeKey: privateKey, FileIO: l.fileIO})
	if err != nil {
		return nil, fmt.Errorf("failed to open install vault: %w", err)
	}

	loaded, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load install vault: %w", err)
	}

	return loaded, nil
}

func (b *GCPBootstrapper) installInputPaths() []string {
	paths := append([]string(nil), b.Env.InstallConfigs...)
	if b.Env.InstallConfigTemplatePath != "" {
		paths = append([]string{b.Env.InstallConfigTemplatePath}, paths...)
	}

	return paths
}

// validateBootstrapFiles checks supplied files consistently in both modes. A missing
// binary selects the normal remote OMS reuse/download behavior.
func (b *GCPBootstrapper) validateBootstrapFiles() error {
	if b.Env.RemoteOmsBinaryPath != "" && !b.fw.Exists(b.Env.RemoteOmsBinaryPath) {
		return fmt.Errorf("remote OMS binary not found at path: %s", b.Env.RemoteOmsBinaryPath)
	}

	paths := b.installInputPaths()
	if b.Env.InstallVaultPath != "" {
		paths = append(paths, b.Env.InstallVaultPath)
	}

	for _, path := range paths {
		if !b.fw.Exists(path) {
			return fmt.Errorf("bootstrap input file not found at path: %s", path)
		}
	}

	if b.Env.InstallVaultPrivKey != "" && b.Env.InstallVaultPath == "" {
		return fmt.Errorf("--priv-key requires --vault")
	}

	return nil
}

// loadInstallVault seeds missing secrets while keeping active/recovered vault values.
func (b *GCPBootstrapper) loadInstallVault(manager installer.InstallConfigManager) error {
	if b.Env.InstallVaultPath == "" {
		return nil
	}

	if b.installVault == nil {
		loader := b.InstallVaultLoader
		if loader == nil {
			loader = &sopsInstallVaultLoader{fileIO: b.fw}
		}

		loaded, err := loader.Load(b.Env.InstallVaultPath, b.Env.InstallVaultPrivKey)
		if err != nil {
			return fmt.Errorf("failed to load install seed vault: %w", err)
		}

		b.installVault = loaded
	}

	target := manager.GetVault()
	for _, secret := range b.installVault.Secrets {
		if target.GetSecret(secret.Name) == nil {
			target.SetSecret(secret)
		}
	}

	return nil
}

// loadInstallInputs overlays the template and repeated configs on the existing config
// or profile. Infrastructure-specific settings are applied by the caller afterwards.
func (b *GCPBootstrapper) loadInstallInputs(base files.RootConfig, manager installer.InstallConfigManager) (files.RootConfig, error) {
	if len(b.installInputPaths()) == 0 {
		return base, nil
	}

	data, err := base.Marshal()
	if err != nil {
		return base, fmt.Errorf("failed to marshal base install config: %w", err)
	}

	var merged map[string]any
	if err := yaml.Unmarshal(data, &merged); err != nil {
		return base, fmt.Errorf("failed to parse base install config: %w", err)
	}

	store := vault.NewVaultTemplatingSecretStore(manager.GetVault())
	for _, path := range b.installInputPaths() {
		partial, err := b.readInstallInput(path, store)
		if err != nil {
			return base, err
		}

		merged = util.DeepMergeMaps(merged, partial)
	}

	data, err = yaml.Marshal(merged)
	if err != nil {
		return base, fmt.Errorf("failed to marshal merged install config: %w", err)
	}

	config := files.NewRootConfig()
	if err := config.Unmarshal(data); err != nil {
		return base, fmt.Errorf("failed to load merged install config: %w", err)
	}

	return config, nil
}

func (b *GCPBootstrapper) readInstallInput(path string, store configtemplating.SecretStore) (map[string]any, error) {
	data, err := b.fw.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read install config %s: %w", path, err)
	}

	data, err = configtemplating.RenderInstallConfigTemplate(data, store)
	if err != nil {
		return nil, fmt.Errorf("failed to render install config %s: %w", path, err)
	}

	var partial map[string]any
	if err := yaml.Unmarshal(data, &partial); err != nil {
		return nil, fmt.Errorf("failed to parse install config %s: %w", path, err)
	}

	return partial, nil
}
