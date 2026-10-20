// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"go.yaml.in/yaml/v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// configFileMode matches the merged install config, which can hold rendered vault values.
const configFileMode = 0o600

// ValidateExtraVaultSecretName rejects names Kubernetes would refuse for a Secret before any
// install step runs.
func ValidateExtraVaultSecretName(name string) error {
	if name == "" {
		return nil
	}

	if problems := validation.IsDNS1123Subdomain(name); len(problems) > 0 {
		return fmt.Errorf("invalid extra vault secret name %q: %s", name, strings.Join(problems, "; "))
	}

	return nil
}

// WriteExtraVaultSecret signs fresh service account tokens and writes the vault into secretName.
func WriteExtraVaultSecret(
	ctx context.Context,
	kubeClient ctrlclient.Client,
	installVault *files.InstallVault,
	secretName string,
) error {
	if err := secrets.EnsureServiceAccountTokens(installVault); err != nil {
		return fmt.Errorf("failed to ensure service account tokens: %w", err)
	}

	creator := vault.NewVaultSecretCreator(kubeClient)
	if err := creator.CreateSecretFromVault(ctx, installVault, vault.VaultSecretNamespace, secretName); err != nil {
		return fmt.Errorf("failed to write vault secret %q: %w", secretName, err)
	}

	return nil
}

// ChartVaultSecretName returns the vault Secret the chart's ExternalSecrets read.
func ChartVaultSecretName(extraVaultSecret string) string {
	if extraVaultSecret == "" {
		return vault.VaultSecretName
	}

	return extraVaultSecret
}

// VaultSecretChecksum reads the vault Secret back from the cluster, so the checksum matches
// what ESO reads, whichever phase or run wrote it. found is false when the Secret does not exist.
func VaultSecretChecksum(ctx context.Context, kubeClient ctrlclient.Client, secretName string) (checksum string, found bool, err error) {
	secret := &corev1.Secret{}

	err = kubeClient.Get(ctx, ctrlclient.ObjectKey{Namespace: vault.VaultSecretNamespace, Name: secretName}, secret)
	if apierrors.IsNotFound(err) {
		return "", false, nil
	}

	if err != nil {
		return "", false, fmt.Errorf("failed to read %q: %w", secretName, err)
	}

	return vault.SecretDataChecksum(secret.Data), true, nil
}

// AddExternalSecretsChecksum sets codesphere.override.global.externalSecretsChecksum in the
// merged config file OMS hands to the TS installer. The chart puts it on its ExternalSecrets,
// which ESO only re-syncs when they change, so a changed vault reaches the release in the
// same deploy. Rewriting the merged config is a stopgap: once OMS deploys through the
// cs-applications app of apps, it sets the checksum in the values it builds instead.
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
