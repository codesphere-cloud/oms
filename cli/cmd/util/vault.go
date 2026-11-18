// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/vault"
	intutil "github.com/codesphere-cloud/oms/internal/util"
)

// ResolveVaultAgeKey resolves the age key needed to read the existing vault at vaultPath, so
// commands can open a vault they did not create.
//
// Recovery skips the lookup and keeps the flag, since recovery replaces the local vault with a
// plaintext copy from the jumpbox, so a stale SOPS vault whose key is gone must not block it.
func ResolveVaultAgeKey(fw intutil.FileIO, vaultPath, ageKeyFlag string, recoverConfig bool) (string, error) {
	if recoverConfig {
		return ageKeyFlag, nil
	}

	ageKey, err := vault.ResolveExistingVaultAgeKey(fw, vaultPath, ageKeyFlag)
	if err != nil {
		return "", fmt.Errorf("failed to resolve age key of vault %s: %w", vaultPath, err)
	}

	return ageKey, nil
}
