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
	return secrets.EnsureSecrets(g.Vault, g.Config)
}
