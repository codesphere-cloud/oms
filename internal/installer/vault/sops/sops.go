// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package sops implements a SOPS-encrypted, age-backed vault.
package sops

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault/internal/filebackend"
	"github.com/codesphere-cloud/oms/internal/util"
	sopsage "github.com/getsops/sops/v3/age"
)

// Options configures a SOPS-encrypted file-backed vault.
type Options struct {
	Path         string
	AgeKey       string
	WithComments bool
	FileIO       util.FileIO
}

// SopsVault stores vault YAML encrypted with SOPS and age.
//
//revive:disable-next-line:exported // The backend-specific name is intentional.
type SopsVault struct {
	fileOptions filebackend.Options
	ageKey      string
}

// New creates a SOPS-encrypted file-backed vault.
func New(opts Options) (*SopsVault, error) {
	if strings.TrimSpace(opts.Path) == "" {
		return nil, fmt.Errorf("SOPS vault requires a file path")
	}

	if err := ValidateConfiguration(opts.AgeKey); err != nil {
		return nil, err
	}

	fileOpts := filebackend.WithDefaults(filebackend.Options{
		Path: opts.Path, WithComments: opts.WithComments, FileIO: opts.FileIO,
	})

	return &SopsVault{fileOptions: fileOpts, ageKey: opts.AgeKey}, nil
}

// NewLazy creates a vault whose path and key configuration are validated on first use.
func NewLazy(opts Options) *SopsVault {
	fileOpts := filebackend.WithDefaults(filebackend.Options{
		Path: opts.Path, WithComments: opts.WithComments, FileIO: opts.FileIO,
	})

	return &SopsVault{fileOptions: fileOpts, ageKey: opts.AgeKey}
}

// Load reads, validates, decrypts, and parses the configured vault file.
func (v *SopsVault) Load() (*files.InstallVault, error) {
	keyPath, err := v.getAgeKey()
	if err != nil {
		return nil, err
	}

	data, err := v.fileOptions.FileIO.ReadFile(v.fileOptions.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read vault file %s: %w", v.fileOptions.Path, err)
	}

	encrypted, err := filebackend.IsSOPSEncryptedYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect vault file %s: %w", v.fileOptions.Path, err)
	}

	if !encrypted {
		return nil, fmt.Errorf("vault file %s is not SOPS-encrypted", v.fileOptions.Path)
	}

	plain, err := DecryptFile(v.fileOptions.Path, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt vault file %s: %w", v.fileOptions.Path, err)
	}

	result, err := filebackend.Parse(plain)
	if err != nil {
		return nil, fmt.Errorf("failed to parse decrypted vault file %s: %w", v.fileOptions.Path, err)
	}

	return result, nil
}

// LoadOrCreate loads the vault or returns an empty vault if the file does not exist.
func (v *SopsVault) LoadOrCreate() (*files.InstallVault, error) {
	result, err := v.Load()
	if errors.Is(err, fs.ErrNotExist) {
		return &files.InstallVault{}, nil
	}

	return result, err
}

// Save encrypts and writes vault data to the configured path.
func (v *SopsVault) Save(data *files.InstallVault) error {
	if _, err := v.getAgeKey(); err != nil {
		return err
	}

	recipient, _, err := resolveConfiguredAgeKey(v.fileOptions.FileIO, v.ageKey)
	if err != nil {
		return err
	}

	plain, err := filebackend.Marshal(data, v.fileOptions.WithComments)
	if err != nil {
		return fmt.Errorf("failed to marshal SOPS vault: %w", err)
	}

	if err := v.fileOptions.FileIO.MkdirAll(filepath.Dir(v.fileOptions.Path), 0700); err != nil {
		return fmt.Errorf("failed to create vault directory: %w", err)
	}

	plainPath, err := v.fileOptions.FileIO.CreateTemp(filepath.Dir(v.fileOptions.Path), ".vault-plaintext-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary plaintext vault: %w", err)
	}
	defer func() { _ = v.fileOptions.FileIO.Remove(plainPath) }()

	if err := v.fileOptions.FileIO.WriteFile(plainPath, plain, 0600); err != nil {
		return fmt.Errorf("failed to write temporary plaintext vault: %w", err)
	}

	encryptedPath, err := v.fileOptions.FileIO.CreateTemp(filepath.Dir(v.fileOptions.Path), ".vault-encrypted-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary encrypted vault: %w", err)
	}
	defer func() { _ = v.fileOptions.FileIO.Remove(encryptedPath) }()

	if err := EncryptFile(plainPath, encryptedPath, recipient); err != nil {
		return err
	}

	if err := v.fileOptions.FileIO.Chmod(encryptedPath, 0600); err != nil {
		return fmt.Errorf("failed to set encrypted vault permissions: %w", err)
	}

	if err := v.fileOptions.FileIO.Rename(encryptedPath, v.fileOptions.Path); err != nil {
		return fmt.Errorf("failed to replace encrypted vault: %w", err)
	}

	return nil
}

func (v *SopsVault) getAgeKey() (string, error) {
	if v.ageKey != "" {
		return v.ageKey, nil
	}

	if os.Getenv(sopsage.SopsAgeKeyEnv) != "" {
		return "", nil
	}

	if keyFile := os.Getenv(sopsage.SopsAgeKeyFileEnv); keyFile != "" {
		return keyFile, nil
	}

	return "", ValidateConfiguration("")
}

// ValidateConfiguration ensures an age key is available for SOPS operations.
func ValidateConfiguration(ageKey string) error {
	if ageKey != "" || os.Getenv(sopsage.SopsAgeKeyEnv) != "" || os.Getenv(sopsage.SopsAgeKeyFileEnv) != "" {
		return nil
	}

	return fmt.Errorf("SOPS vault requires an age key; set an age key argument or %s/%s", sopsage.SopsAgeKeyEnv, sopsage.SopsAgeKeyFileEnv)
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
