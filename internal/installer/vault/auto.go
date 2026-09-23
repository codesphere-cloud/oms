// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault/internal/filebackend"
	"github.com/codesphere-cloud/oms/internal/util"
)

// autoVault selects the backend that matches the vault file on disk. The format is detected
// on every Load and Save, so a vault that changes format between runs (for example after
// --recover-config downloads a decrypted copy) keeps working.
type autoVault struct{ options Options }

// DetectType reports the on-disk vault type of path. A missing file counts as plaintext so
// callers can create a new vault; any other read failure is returned as an error.
func DetectType(fileIO util.FileIO, path string) (Type, error) {
	if fileIO == nil {
		fileIO = util.NewFilesystemWriter()
	}

	data, err := fileIO.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return TypePlain, nil
	}

	if err != nil {
		return "", fmt.Errorf("failed to read vault file %s: %w", path, err)
	}

	encrypted, err := filebackend.IsSOPSEncryptedYAML(data)
	if err != nil {
		return "", fmt.Errorf("failed to inspect vault file %s: %w", path, err)
	}

	if encrypted {
		return TypeSOPS, nil
	}

	return TypePlain, nil
}

// Load reads the vault using the backend matching the file on disk.
func (v *autoVault) Load() (*files.InstallVault, error) {
	return v.load(Vault.Load)
}

// LoadOrCreate loads the vault or returns an empty vault if the file does not exist.
func (v *autoVault) LoadOrCreate() (*files.InstallVault, error) {
	return v.load(Vault.LoadOrCreate)
}

func (v *autoVault) load(load func(Vault) (*files.InstallVault, error)) (*files.InstallVault, error) {
	backend, err := v.backendForFile()
	if err != nil {
		return nil, err
	}

	loaded, err := load(backend)
	if err != nil {
		return nil, fmt.Errorf("failed to load vault %s: %w", v.options.Path, err)
	}

	return loaded, nil
}

// Save writes the vault in the format that is already on disk. A vault that does not exist
// yet is written as plaintext, matching the pre-existing bootstrap behaviour of encrypting it
// on the jumpbox instead.
func (v *autoVault) Save(data *files.InstallVault) error {
	backend, err := v.backendForFile()
	if err != nil {
		return err
	}

	if err := backend.Save(data); err != nil {
		return fmt.Errorf("failed to write vault %s: %w", v.options.Path, err)
	}

	return nil
}

func (v *autoVault) backendForFile() (Vault, error) {
	vaultType, err := DetectType(v.options.FileIO, v.options.Path)
	if err != nil {
		return nil, err
	}

	return New(vaultType, v.options)
}
