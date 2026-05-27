// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"errors"
	"fmt"
	"os"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"go.yaml.in/yaml/v3"
)

type VaultTemplatingSecretStore struct {
	vault      *files.InstallVault
	vaultPath  string
	ageKeyPath string
}

func NewVaultTemplatingSecretStore(vault *files.InstallVault) *VaultTemplatingSecretStore {
	return &VaultTemplatingSecretStore{vault: vault}
}

func NewLazyVaultTemplatingSecretStore(vaultPath, ageKeyPath string) *VaultTemplatingSecretStore {
	return &VaultTemplatingSecretStore{
		vaultPath:  vaultPath,
		ageKeyPath: ageKeyPath,
	}
}

func NewVaultTemplatingSecretStoreFromFile(vaultPath, ageKeyPath string) (*VaultTemplatingSecretStore, error) {
	vault, err := LoadVaultData(vaultPath, ageKeyPath)
	if err != nil {
		return nil, err
	}
	return NewVaultTemplatingSecretStore(vault), nil
}

func (s *VaultTemplatingSecretStore) LookupSecret(name string, selector ...string) (string, error) {
	if s == nil {
		return "", errors.New("vault secret store is not configured")
	}
	if s.vault == nil {
		if s.vaultPath == "" {
			return "", errors.New("vault secret store is not configured")
		}
		vault, err := LoadVaultData(s.vaultPath, s.ageKeyPath)
		if err != nil {
			return "", err
		}
		s.vault = vault
	}

	for _, entry := range s.vault.Secrets {
		if entry.Name == name {
			return selectVaultSecretValue(entry, selector...)
		}
	}

	return "", fmt.Errorf("secret %q not found in vault", name)
}

func selectVaultSecretValue(entry files.SecretEntry, selector ...string) (string, error) {
	field := ""
	if len(selector) > 0 {
		field = selector[0]
	}

	switch field {
	case "", "content", "file.content":
		if entry.File != nil {
			return entry.File.Content, nil
		}
		if entry.Fields != nil {
			return entry.Fields.Password, nil
		}
	case "name", "file.name":
		if entry.File != nil {
			return entry.File.Name, nil
		}
	case "password", "fields.password":
		if entry.Fields != nil {
			return entry.Fields.Password, nil
		}
	case "username", "fields.username":
		if entry.Fields != nil {
			return entry.Fields.Username, nil
		}
	default:
		return "", fmt.Errorf("unsupported selector %q for secret %q", field, entry.Name)
	}

	return "", fmt.Errorf("selector %q is not available on secret %q", field, entry.Name)
}

func LoadVaultData(vaultPath, ageKeyPath string) (*files.InstallVault, error) {
	data, err := os.ReadFile(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read vault file %s: %w", vaultPath, err)
	}

	encrypted, err := isSOPSEncryptedYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect vault file %s: %w", vaultPath, err)
	}
	if !encrypted {
		return nil, fmt.Errorf("vault file %s is not SOPS-encrypted", vaultPath)
	}

	decrypted, err := DecryptFileWithSOPS(vaultPath, ageKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt vault.yaml: %w", err)
	}

	vault, err := parseVaultData(decrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to parse decrypted vault.yaml: %w", err)
	}

	return vault, nil
}

// isSOPSEncryptedYAML checks whether the YAML document contains SOPS metadata.
// SOPS-encrypted YAML files have a top-level "sops" mapping that stores
// encryption metadata such as age recipients, encrypted data keys, and MACs.
func isSOPSEncryptedYAML(data []byte) (bool, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return false, err
	}
	if len(doc.Content) == 0 {
		return false, nil
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return false, nil
	}

	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "sops" && root.Content[i+1].Kind == yaml.MappingNode {
			return true, nil
		}
	}

	return false, nil
}

func parseVaultData(data []byte) (*files.InstallVault, error) {
	vault := &files.InstallVault{}
	if err := vault.Unmarshal(data); err != nil {
		return nil, err
	}
	return vault, nil
}
