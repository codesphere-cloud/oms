// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"os"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"go.yaml.in/yaml/v3"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// defaultReleaseName matches the release name the TS installer falls back to.
	defaultReleaseName = "codesphere"
	// releaseLabelKey marks objects that belong to one Codesphere release.
	releaseLabelKey = "codesphere.com/release"

	// configFileMode matches the merged install config, which can hold rendered vault values.
	configFileMode = 0o600
)

func releaseName(cfg files.CodesphereConfig) string {
	if cfg.ReleaseName == "" {
		return defaultReleaseName
	}

	return cfg.ReleaseName
}

// releaseVaultSecretName returns the name of the secret ESO reads the release's secrets from.
func releaseVaultSecretName(releaseName string) string {
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

// WriteReleaseVaultSecret signs fresh service account tokens, writes the vault into the
// release's own secret and returns the checksum of the written data. Each release has its
// own secret, so deploying one release never changes or removes what another release reads.
func WriteReleaseVaultSecret(
	ctx context.Context,
	kubeClient ctrlclient.Client,
	installVault *files.InstallVault,
	cfg files.CodesphereConfig,
) (string, error) {
	if err := secrets.EnsureServiceAccountTokens(installVault); err != nil {
		return "", fmt.Errorf("failed to ensure service account tokens: %w", err)
	}

	release := releaseName(cfg)
	secretName := releaseVaultSecretName(release)
	labels := map[string]string{releaseLabelKey: release}

	creator := vault.NewVaultSecretCreator(kubeClient)
	if err := creator.CreateSecretFromVault(ctx, installVault, vault.VaultSecretNamespace, secretName, labels); err != nil {
		return "", fmt.Errorf("failed to write vault secret %q: %w", secretName, err)
	}

	checksum, err := vault.SecretDataChecksum(installVault)
	if err != nil {
		return "", fmt.Errorf("failed to checksum vault secret %q: %w", secretName, err)
	}

	return checksum, nil
}

// AddExternalSecretsChecksum sets codesphere.override.global.externalSecretsChecksum in the
// config file. The chart puts it on its ExternalSecrets, which ESO only re-syncs when they
// change, so a changed vault reaches the release in the same deploy.
func AddExternalSecretsChecksum(configPath, checksum string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config %s: %w", configPath, err)
	}

	config := map[string]any{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config %s: %w", configPath, err)
	}

	global := childMap(childMap(childMap(config, "codesphere"), "override"), "global")
	global["externalSecretsChecksum"] = checksum

	updated, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, updated, configFileMode); err != nil {
		return fmt.Errorf("failed to write config %s: %w", configPath, err)
	}

	return nil
}

// childMap returns parent[key], replacing anything that is not a map with an empty one.
func childMap(parent map[string]any, key string) map[string]any {
	child, ok := parent[key].(map[string]any)
	if !ok {
		child = map[string]any{}
		parent[key] = child
	}

	return child
}
