// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer/vault/sops"
	"github.com/codesphere-cloud/oms/internal/util"
)

// ResolveExistingVaultAgeKey resolves the age key needed to read the vault at vaultPath. A
// missing or plaintext vault needs no key, so ageKey is returned unchanged. An encrypted vault
// requires an existing key; a freshly generated one cannot decrypt it.
func ResolveExistingVaultAgeKey(fileIO util.FileIO, vaultPath, ageKey string) (string, error) {
	encrypted, err := IsEncryptedFile(fileIO, vaultPath)
	if err != nil {
		return "", fmt.Errorf("failed to detect vault type of %s: %w", vaultPath, err)
	}

	if !encrypted {
		return ageKey, nil
	}

	if _, err := exec.LookPath("sops"); err != nil {
		return "", fmt.Errorf("vault %s is SOPS-encrypted but the sops binary is not in PATH: %w", vaultPath, err)
	}

	resolved, err := sops.ResolveExistingAgeKey(ageKey, filepath.Dir(vaultPath))
	if err != nil {
		return "", fmt.Errorf("vault %s is SOPS-encrypted, but no usable age key was found: %w", vaultPath, err)
	}

	return resolved, nil
}
