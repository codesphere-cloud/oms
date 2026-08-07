// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package vault handles all interactions with vaults across vault types. Currently supported types are plain and sops
package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// PlainFileVault stores vault YAML as an unencrypted file.
type PlainFileVault struct{ options FileOptions }

// NewPlainFileVault returns a vault handler for interacting with a plain (unencrypted) file
func NewPlainFileVault(opts FileOptions) (*PlainFileVault, error) {
	if strings.TrimSpace(opts.Path) == "" {
		return nil, fmt.Errorf("plain vault requires a file path")
	}

	opts.FileIO = fileIOOrDefault(opts.FileIO)

	return &PlainFileVault{options: opts}, nil
}

// Load reads the unencrypted vault file and verifies it's not accidentally SOPS-encrypted
func (v *PlainFileVault) Load() (*files.InstallVault, error) {
	data, err := v.options.FileIO.ReadFile(v.options.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read vault file %s: %w", v.options.Path, err)
	}

	encrypted, err := isSOPSEncryptedYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect vault file %s: %w", v.options.Path, err)
	}

	if encrypted {
		return nil, fmt.Errorf("vault file %s is SOPS-encrypted, but vault type is %q", v.options.Path, TypePlain)
	}

	result, err := parseVaultData(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse vault file %s: %w", v.options.Path, err)
	}

	return result, nil
}

// LoadOrCreate reads the vault from disk and returns an empty vault object if it doesn't exist
func (v *PlainFileVault) LoadOrCreate() (*files.InstallVault, error) {
	result, err := v.Load()
	if errors.Is(err, fs.ErrNotExist) {
		return &files.InstallVault{}, nil
	}

	return result, err
}

// Save writes the vault data to an unencrypted file
func (v *PlainFileVault) Save(data *files.InstallVault) error {
	plain, err := marshalVault(data, v.options.WithComments)
	if err != nil {
		return err
	}

	return writeVaultFile(v.options.FileIO, v.options.Path, plain)
}
