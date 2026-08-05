// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
	sopsage "github.com/getsops/sops/v3/age"
)

// SOPSVault stores vault YAML encrypted with SOPS and age.
type SOPSVault struct{ options SOPSOptions }

// SOPSOptions contains configuration specific to the SOPS file backend.
type SOPSOptions struct {
	File   FileOptions
	AgeKey string
}

// NewSOPSVault creates a new vault handler for interacting with SOPS-encrypted vault files
func NewSOPSVault(opts SOPSOptions) (*SOPSVault, error) {
	if strings.TrimSpace(opts.File.Path) == "" {
		return nil, fmt.Errorf("SOPS vault requires a file path")
	}

	if err := ValidateConfiguration(TypeSOPS, opts.AgeKey); err != nil {
		return nil, err
	}

	opts.File.FileIO = fileIOOrDefault(opts.File.FileIO)

	return &SOPSVault{options: opts}, nil
}

// Load reads the validates the referenced file is SOPS-encrypted and load its data
func (v *SOPSVault) Load() (*files.InstallVault, error) {
	keyPath, err := v.getAgeKey()
	if err != nil {
		return nil, err
	}

	data, err := v.options.File.FileIO.ReadFile(v.options.File.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read vault file %s: %w", v.options.File.Path, err)
	}

	encrypted, err := isSOPSEncryptedYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect vault file %s: %w", v.options.File.Path, err)
	}

	if !encrypted {
		return nil, fmt.Errorf("vault file %s is not SOPS-encrypted", v.options.File.Path)
	}

	plain, err := DecryptFileWithSOPS(v.options.File.Path, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt vault file %s: %w", v.options.File.Path, err)
	}

	result, err := parseVaultData(plain)
	if err != nil {
		return nil, fmt.Errorf("failed to parse decrypted vault file %s: %w", v.options.File.Path, err)
	}

	return result, nil
}

// LoadOrCreate loads the referenced file from disk or returns an empty InstallVault
func (v *SOPSVault) LoadOrCreate() (*files.InstallVault, error) {
	result, err := v.Load()
	if errors.Is(err, fs.ErrNotExist) {
		return &files.InstallVault{}, nil
	}

	return result, err
}

// Save encrypts the vault data and writes it to the configured path
func (v *SOPSVault) Save(data *files.InstallVault) error {
	if _, err := v.getAgeKey(); err != nil {
		return err
	}

	recipient, _, err := resolveConfiguredAgeKey(v.options.File.FileIO, v.options.AgeKey)
	if err != nil {
		return err
	}

	plain, err := marshalVault(data, v.options.File.WithComments)
	if err != nil {
		return err
	}

	if err := v.options.File.FileIO.MkdirAll(filepath.Dir(v.options.File.Path), 0700); err != nil {
		return fmt.Errorf("failed to create vault directory: %w", err)
	}

	plainPath, err := v.options.File.FileIO.CreateTemp(filepath.Dir(v.options.File.Path), ".vault-plaintext-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary plaintext vault: %w", err)
	}
	defer func() { _ = v.options.File.FileIO.Remove(plainPath) }()

	if err := v.options.File.FileIO.WriteFile(plainPath, plain, 0600); err != nil {
		return fmt.Errorf("failed to write temporary plaintext vault: %w", err)
	}

	encryptedPath, err := v.options.File.FileIO.CreateTemp(filepath.Dir(v.options.File.Path), ".vault-encrypted-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary encrypted vault: %w", err)
	}
	defer func() { _ = v.options.File.FileIO.Remove(encryptedPath) }()

	if err := EncryptFileWithSOPS(plainPath, encryptedPath, recipient); err != nil {
		return err
	}

	if err := v.options.File.FileIO.Chmod(encryptedPath, 0600); err != nil {
		return fmt.Errorf("failed to set encrypted vault permissions: %w", err)
	}

	if err := v.options.File.FileIO.Rename(encryptedPath, v.options.File.Path); err != nil {
		return fmt.Errorf("failed to replace encrypted vault: %w", err)
	}

	return nil
}

func (v *SOPSVault) getAgeKey() (string, error) {
	if v.options.AgeKey != "" {
		return v.options.AgeKey, nil
	}

	if os.Getenv(sopsage.SopsAgeKeyEnv) != "" {
		return "", nil
	}

	if keyFile := os.Getenv(sopsage.SopsAgeKeyFileEnv); keyFile != "" {
		return keyFile, nil
	}

	return "", ValidateConfiguration(TypeSOPS, "")
}

func resolveConfiguredAgeKey(fileIO util.FileIO, explicit string) (recipient, keyPath string, err error) {
	if explicit != "" {
		recipient, err := readRecipientFromFile(fileIO, explicit)
		if err != nil {
			return "", "", fmt.Errorf("failed to read age key from %s: %w", explicit, err)
		}

		return recipient, explicit, nil
	}

	if raw := os.Getenv(sopsage.SopsAgeKeyEnv); raw != "" {
		recipient, err := parseAgeRecipient(strings.NewReader(raw))
		if err != nil {
			return "", "", fmt.Errorf("failed to parse age key from %s: %w", sopsage.SopsAgeKeyEnv, err)
		}

		return recipient, "", nil
	}

	if keyFile := os.Getenv(sopsage.SopsAgeKeyFileEnv); keyFile != "" {
		recipient, err := readRecipientFromFile(fileIO, keyFile)
		if err != nil {
			return "", "", fmt.Errorf("failed to read age key from %s: %w", keyFile, err)
		}

		return recipient, keyFile, nil
	}

	return "", "", fmt.Errorf("SOPS vault requires an age key")
}
