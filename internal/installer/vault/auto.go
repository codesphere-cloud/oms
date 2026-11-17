// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

// autoVault picks the backend matching the file on disk, redetected on every Load and Save so a
// vault that changes format between runs (e.g. after --recover-config) keeps working.
type autoVault struct{ options Options }

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

// Save writes the vault in the format already on disk. A vault that does not exist yet is
// written as plaintext, since the bootstrap flow encrypts it on the jumpbox afterwards.
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

// backendForFile treats a missing file as plaintext so a new vault can be created.
func (v *autoVault) backendForFile() (Vault, error) {
	fileIO := v.options.FileIO
	if fileIO == nil {
		fileIO = util.NewFilesystemWriter()
	}

	encrypted, err := IsEncryptedFile(fileIO, v.options.Path)
	if err != nil {
		return nil, err
	}

	if encrypted {
		return New(TypeSOPS, v.options)
	}

	return New(TypePlain, v.options)
}
