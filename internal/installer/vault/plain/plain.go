// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package plain implements an unencrypted file-backed vault.
package plain

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault/internal/filebackend"
	"github.com/codesphere-cloud/oms/internal/util"
)

// Options configures a plain file-backed vault.
type Options struct {
	Path         string
	WithComments bool
	FileIO       util.FileIO
}

// PlainVault stores vault YAML as an unencrypted file.
//
//revive:disable-next-line:exported // The backend-specific name is intentional.
type PlainVault struct{ options filebackend.Options }

// New creates a plain file-backed vault.
func New(opts Options) (*PlainVault, error) {
	if strings.TrimSpace(opts.Path) == "" {
		return nil, fmt.Errorf("plain vault requires a file path")
	}

	fileOpts := filebackend.WithDefaults(filebackend.Options(opts))

	return &PlainVault{options: fileOpts}, nil
}

// Load reads the unencrypted vault file and verifies it is not SOPS-encrypted.
func (v *PlainVault) Load() (*files.InstallVault, error) {
	data, err := v.options.FileIO.ReadFile(v.options.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read vault file %s: %w", v.options.Path, err)
	}

	encrypted, err := filebackend.IsSOPSEncryptedYAML(data)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect vault file %s: %w", v.options.Path, err)
	}

	if encrypted {
		return nil, fmt.Errorf("vault file %s is SOPS-encrypted, but vault type is %q", v.options.Path, "plain")
	}

	result, err := filebackend.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse vault file %s: %w", v.options.Path, err)
	}

	return result, nil
}

// LoadOrCreate loads the vault or returns an empty vault if the file does not exist.
func (v *PlainVault) LoadOrCreate() (*files.InstallVault, error) {
	result, err := v.Load()
	if errors.Is(err, fs.ErrNotExist) {
		return &files.InstallVault{}, nil
	}

	return result, err
}

// Save writes vault data to an unencrypted file.
func (v *PlainVault) Save(data *files.InstallVault) error {
	plain, err := filebackend.Marshal(data, v.options.WithComments)
	if err != nil {
		return fmt.Errorf("failed to marshal plain vault: %w", err)
	}

	if err := filebackend.Write(v.options.FileIO, v.options.Path, plain); err != nil {
		return fmt.Errorf("failed to write plain vault: %w", err)
	}

	return nil
}
