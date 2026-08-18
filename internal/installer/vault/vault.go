// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package vault defines the installer vault interface and backend factory.
package vault

import (
	"fmt"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault/plain"
	"github.com/codesphere-cloud/oms/internal/installer/vault/sops"
	"github.com/codesphere-cloud/oms/internal/util"
)

// Type identifies the on-disk vault format.
type Type string

// Supported Vault types
const (
	TypeSOPS    Type = "sops"
	TypePlain   Type = "plain"
	DefaultType      = TypeSOPS
)

// Vault is the persistence boundary for installer secrets. Callers work with
// InstallVault values and do not need to know how those values are represented
// or protected on disk.
type Vault interface {
	Load() (*files.InstallVault, error)
	LoadOrCreate() (*files.InstallVault, error)
	Save(*files.InstallVault) error
}

// Options contains the parameters currently accepted by the vault factory.
// The factory only forwards file parameters to implementations that use them;
// future non-file vaults can ignore Path and WithComments entirely.
type Options struct {
	Path         string
	AgeKey       string
	WithComments bool
	FileIO       util.FileIO
}

// ParseType validates a user supplied vault type.
func ParseType(value string) (Type, error) {
	switch Type(strings.ToLower(strings.TrimSpace(value))) {
	case "", TypeSOPS:
		return TypeSOPS, nil
	case TypePlain:
		return TypePlain, nil
	default:
		return "", fmt.Errorf("unsupported vault type %q (must be %q or %q)", value, TypeSOPS, TypePlain)
	}
}

// ValidateConfiguration validates backend-specific, non-resource parameters.
// File paths are intentionally validated by the file-backed constructors.
func ValidateConfiguration(vaultType Type, ageKey string) error {
	if vaultType != TypeSOPS {
		return nil
	}

	if err := sops.ValidateConfiguration(ageKey); err != nil {
		return fmt.Errorf("failed to validate SOPS vault configuration: %w", err)
	}

	return nil
}

// New creates a vault implementation for the requested type.
func New(vaultType Type, opts Options) (Vault, error) {
	switch vaultType {
	case TypeSOPS:
		backend, err := sops.New(sops.Options{
			Path: opts.Path, AgeKey: opts.AgeKey, WithComments: opts.WithComments, FileIO: opts.FileIO,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create SOPS vault: %w", err)
		}

		return backend, nil
	case TypePlain:
		backend, err := plain.New(plain.Options{
			Path: opts.Path, WithComments: opts.WithComments, FileIO: opts.FileIO,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create plain vault: %w", err)
		}

		return backend, nil
	default:
		return nil, fmt.Errorf("unsupported vault type %q", vaultType)
	}
}

// NewFromString parses vaultType and creates the matching implementation.
func NewFromString(vaultType string, opts Options) (Vault, error) {
	t, err := ParseType(vaultType)
	if err != nil {
		return nil, err
	}

	return New(t, opts)
}
