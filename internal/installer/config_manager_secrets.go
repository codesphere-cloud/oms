// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
)

func (g *InstallConfig) GenerateSecrets() error {
	if g.Vault == nil {
		g.Vault = &files.InstallVault{}
	}
	if err := secrets.EnsureSecrets(g.Vault, g.Config); err != nil {
		return err
	}
	// Sync vault → config struct fields so that ExtractVault and any code reading
	// yaml:"-" fields (private keys, passwords) sees the generated values.
	return g.MergeVaultIntoConfig()
}
