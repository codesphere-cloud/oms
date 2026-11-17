// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/vault"
	intutil "github.com/codesphere-cloud/oms/internal/util"
)

// resolveVaultAccess resolves the age key needed to read the vault at secretsFilePath.
//
// Recovery skips the preflight and keeps the flag, since recoverVault replaces the local vault
// with a plaintext copy from the jumpbox, so a stale SOPS vault whose key is gone must not block it.
func resolveVaultAccess(fw intutil.FileIO, secretsFilePath, ageKeyFlag string, recoverConfig bool) (string, error) {
	if recoverConfig {
		return ageKeyFlag, nil
	}

	ageKey, err := vault.ResolveExistingVaultAgeKey(fw, secretsFilePath, ageKeyFlag)
	if err != nil {
		return "", fmt.Errorf("failed to resolve vault access: %w", err)
	}

	return ageKey, nil
}
