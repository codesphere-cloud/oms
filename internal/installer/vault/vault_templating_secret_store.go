// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"errors"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault/sops"
)

// VaultTemplatingSecretStore resolves secrets referenced from config templates
// against a SOPS-encrypted install vault. The vault can either be provided
// directly or loaded lazily from disk on first lookup.
type VaultTemplatingSecretStore struct {
	vault   *files.InstallVault
	backend Vault
}

// NewVaultTemplatingSecretStore returns a store backed by an already-decrypted vault.
func NewVaultTemplatingSecretStore(vault *files.InstallVault) *VaultTemplatingSecretStore {
	return &VaultTemplatingSecretStore{vault: vault}
}

// NewLazyVaultTemplatingSecretStore returns a store that decrypts and loads the
// vault from vaultPath using ageKeyPath on the first secret lookup.
func NewLazyVaultTemplatingSecretStore(vaultPath, ageKeyPath string) *VaultTemplatingSecretStore {
	backend := sops.NewLazy(sops.Options{Path: vaultPath, AgeKey: ageKeyPath})
	return &VaultTemplatingSecretStore{
		backend: backend,
	}
}

// NewLazyVaultTemplatingSecretStoreWithVault returns a lazily loaded secret
// store backed by any Vault implementation.
func NewLazyVaultTemplatingSecretStoreWithVault(backend Vault) *VaultTemplatingSecretStore {
	return &VaultTemplatingSecretStore{backend: backend}
}

// NewVaultTemplatingSecretStoreFromFile decrypts and loads the vault from
// vaultPath using ageKeyPath and returns a store backed by it.
func NewVaultTemplatingSecretStoreFromFile(vaultPath, ageKeyPath string) (*VaultTemplatingSecretStore, error) {
	backend, err := New(TypeSOPS, Options{Path: vaultPath, AgeKey: ageKeyPath})
	if err != nil {
		return nil, err
	}

	vault, err := backend.Load()
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

	if s.backend == nil {
		return errors.New("vault backend not set")
	}

	vault, err := s.backend.Load()
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
