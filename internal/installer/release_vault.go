// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultReleaseName matches the release name the TS installer falls back to.
	DefaultReleaseName = "codesphere"
	// ReleaseLabel marks objects that belong to one Codesphere release.
	ReleaseLabel = "codesphere.com/release"
)

// ReleaseName returns the Helm release name of the Codesphere umbrella chart.
func ReleaseName(cfg files.CodesphereConfig) string {
	if cfg.ReleaseName == "" {
		return DefaultReleaseName
	}

	return cfg.ReleaseName
}

// ReleaseVaultSecretName returns the name of the secret the release's ExternalSecrets read from.
func ReleaseVaultSecretName(releaseName string) string {
	return releaseName + "-vault"
}

// UsesExternalSecrets reports whether the umbrella chart is set to read its secrets through ESO.
func UsesExternalSecrets(cfg files.CodesphereConfig) bool {
	global, ok := cfg.Override["global"].(map[string]interface{})
	if !ok {
		return false
	}

	enabled, ok := global["externalSecrets"].(bool)

	return ok && enabled
}

// EnsureReleaseVaultSecret writes the vault into the secret the umbrella chart's
// ExternalSecrets read from. It is a no-op unless the chart uses external secrets.
func EnsureReleaseVaultSecret(ctx context.Context, vaultPath, privKey, vaultType string, cfg files.RootConfig) error {
	if !UsesExternalSecrets(cfg.Codesphere) {
		return nil
	}

	installVault, restConfig, err := VaultAndRESTConfig(vaultPath, privKey, vaultType, cfg)
	if err != nil {
		return err
	}

	kubeClient, err := NewKubeClient(restConfig)
	if err != nil {
		return err
	}

	return WriteReleaseVaultSecret(ctx, kubeClient, installVault, ReleaseName(cfg.Codesphere))
}

// WriteReleaseVaultSecret signs fresh service account tokens and writes the vault
// into the release's own secret. Each release has its own secret, so deploying one
// release never changes or removes what another release reads.
func WriteReleaseVaultSecret(ctx context.Context, kubeClient ctrlclient.Client, installVault *files.InstallVault, releaseName string) error {
	if err := secrets.EnsureServiceAccountTokens(installVault); err != nil {
		return fmt.Errorf("failed to ensure service account tokens: %w", err)
	}

	creator := vault.NewVaultSecretCreator(kubeClient)
	secretName := ReleaseVaultSecretName(releaseName)

	labels := map[string]string{ReleaseLabel: releaseName}
	if err := creator.CreateSecretFromVault(ctx, installVault, vault.VaultSecretNamespace, secretName, labels); err != nil {
		return fmt.Errorf("failed to write vault secret %q: %w", secretName, err)
	}

	return nil
}
