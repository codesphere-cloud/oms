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

// resolveBootstrapVaultAccess resolves the vault type and age key for a bootstrap run.
// A recovery run skips the local preflight entirely: recoverVault replaces the local file with
// the plaintext copy it decrypts on the jumpbox, so a stale SOPS vault whose key is gone must
// not block the very recovery that is meant to resolve it.
func resolveBootstrapVaultAccess(fw intutil.FileIO, secretsFilePath, ageKeyFlag string, recoverConfig bool) (vault.Type, string, error) {
	if recoverConfig {
		return vault.TypePlain, "", nil
	}

	return resolveVaultAccess(fw, secretsFilePath, ageKeyFlag)
}

// resolveVaultAccess detects the on-disk format of the install vault at secretsFilePath and
// resolves the age key needed to read it. A plaintext vault, including a file that does not
// exist yet, needs no key. An encrypted vault requires an existing key: the CLI must not
// generate one, because a freshly generated key cannot decrypt the vault and would turn a
// clear error into a confusing decryption failure.
func resolveVaultAccess(fw intutil.FileIO, secretsFilePath, ageKeyFlag string) (vault.Type, string, error) {
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
