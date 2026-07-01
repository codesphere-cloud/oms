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

// VaultTemplatingSecretStore resolves secrets referenced from config templates
// against a SOPS-encrypted install vault. The vault can either be provided
// directly or loaded lazily from disk on first lookup.
type VaultTemplatingSecretStore struct {
	vault      *files.InstallVault
	vaultPath  string
	ageKeyPath string
}

// NewVaultTemplatingSecretStore returns a store backed by an already-decrypted vault.
func NewVaultTemplatingSecretStore(vault *files.InstallVault) *VaultTemplatingSecretStore {
	return &VaultTemplatingSecretStore{vault: vault}
}

// NewLazyVaultTemplatingSecretStore returns a store that decrypts and loads the
// vault from vaultPath using ageKeyPath on the first secret lookup.
func NewLazyVaultTemplatingSecretStore(vaultPath, ageKeyPath string) *VaultTemplatingSecretStore {
	return &VaultTemplatingSecretStore{
		vaultPath:  vaultPath,
		ageKeyPath: ageKeyPath,
	}
}

// NewVaultTemplatingSecretStoreFromFile decrypts and loads the vault from
// vaultPath using ageKeyPath and returns a store backed by it.
func NewVaultTemplatingSecretStoreFromFile(vaultPath, ageKeyPath string) (*VaultTemplatingSecretStore, error) {
	vault, err := LoadVaultData(vaultPath, ageKeyPath)
	if err != nil {
		return nil, err
	}
	return NewVaultTemplatingSecretStore(vault), nil
}

// LookupSecret returns the value of the named secret, optionally narrowed by a
// field selector (e.g. "password", "file.content"). The vault is loaded lazily
// on first use when the store was created without a preloaded vault.
func (s *VaultTemplatingSecretStore) LookupSecret(name string, selector ...string) (string, error) {
	if err := s.ensureVault(); err != nil {
		return "", fmt.Errorf("error ensuring the vault: %w", err)
	}

	for _, entry := range s.vault.Secrets {
		if entry.Name == name {
			return selectVaultSecretValue(entry, selector...)
		}
	}

	return "", fmt.Errorf("secret %q not found in vault", name)
}

// ensureVault lazily decrypts and loads the vault from disk when the store was
// created without a preloaded vault (see NewLazyVaultTemplatingSecretStore).
func (s *VaultTemplatingSecretStore) ensureVault() error {
	if s.vault != nil {
		return nil
	}
	if s.vaultPath == "" {
		return errors.New("vaultPath not set")
	}
	vault, err := LoadVaultData(s.vaultPath, s.ageKeyPath)
	if err != nil {
		return err
	}
	s.vault = vault
	return nil
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

// LoadVaultData reads, SOPS-decrypts, and parses the vault at vaultPath using
// the age key at ageKeyPath, returning the decoded install vault.
func LoadVaultData(vaultPath, ageKeyPath string) (*files.InstallVault, error) {
	data, err := os.ReadFile(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read vault file %s: %w", vaultPath, err)
	}

	encrypted, err := isSOPSEncryptedYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect vault file %s: %w", vaultPath, err)
	}

	if encrypted {
		decrypted, err := DecryptFileWithSOPS(vaultPath, ageKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt vault.yaml: %w", err)
		}
		data = decrypted
	}

	vault, err := parseVaultData(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse decrypted vault.yaml: %w", err)
	}

	return vault, nil
}

// IsSOPSEncryptedFile checks whether the file at path is a SOPS-encrypted YAML document.
func IsSOPSEncryptedFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return isSOPSEncryptedYAML(data)
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

	// A mapping node stores its keys and values as a flat list alternating
	// key, value, key, value, ... so we step by 2 to visit each key/value pair.
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "sops" && root.Content[i+1].Kind == yaml.MappingNode {
			return true, nil
		}
	}

	return false, nil
}

func parseVaultData(data []byte) (*files.InstallVault, error) {
	data = unwrapSOPSData(data)

	vault := &files.InstallVault{}
	if err := vault.Unmarshal(data); err != nil {
		return nil, err
	}
	return vault, nil
}
