// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/installer/vault/sops"
	intutil "github.com/codesphere-cloud/oms/internal/util"
)

// resolveVaultAccess detects the vault format at secretsFilePath and resolves the age key to read
// it. An encrypted vault requires an existing key, since a freshly generated one cannot decrypt it.
//
// Recovery skips the preflight: recoverVault replaces the local vault with a plaintext copy from
// the jumpbox, so a stale SOPS vault whose key is gone must not block it.
func resolveVaultAccess(fw intutil.FileIO, secretsFilePath, ageKeyFlag string, recoverConfig bool) (vault.Type, string, error) {
	if recoverConfig {
		return vault.TypePlain, "", nil
	}

	vaultType, err := vault.DetectType(fw, secretsFilePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to detect vault type of %s: %w", secretsFilePath, err)
	}

	if vaultType != vault.TypeSOPS {
		return vaultType, ageKeyFlag, nil
	}

	if _, err := exec.LookPath("sops"); err != nil {
		return "", "", fmt.Errorf("vault %s is SOPS-encrypted but the sops binary is not in PATH: %w", secretsFilePath, err)
	}

	ageKey, err := sops.ResolveExistingAgeKey(ageKeyFlag, filepath.Dir(secretsFilePath))
	if err != nil {
		return "", "", fmt.Errorf("vault %s is SOPS-encrypted, but no usable age key was found: %w", secretsFilePath, err)
	}

	return vaultType, ageKey, nil
}
