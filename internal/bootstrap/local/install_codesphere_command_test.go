// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"testing"

	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
)

func TestLocalInstallCodesphereOptions(t *testing.T) {
	b := &LocalBootstrapper{Env: &CodesphereEnvironment{ArgoCDRegistryURL: "oci://ghcr.io/codesphere-cloud/charts"}, Verbose: true}
	opts := b.localInstallCodesphereOptions("/bundle", "/config.yaml", "/vault.yaml", "/age-key", "/config")
	if !opts.LocalComponents || !opts.AutoApprove || !opts.Verbose || opts.Package != "/bundle" ||
		len(opts.Configs) != 1 || opts.Configs[0] != "/config.yaml" || opts.Vault != "/vault.yaml" ||
		opts.PrivKey != "/age-key" || opts.VaultType != string(vault.TypeSOPS) ||
		opts.LocalConfigDir != "/config" || opts.ArgoCDRegistryURL != "ghcr.io/codesphere-cloud/charts" ||
		opts.ArgoCDVersion != "9.5.21" || !opts.ArgoCDForceConflicts || opts.ArgoCDRepoURL != argocd.DefaultRepoURL {
		t.Fatalf("unexpected local install options: %+v", opts)
	}
}
