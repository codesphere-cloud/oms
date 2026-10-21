// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package vault

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	"go.yaml.in/yaml/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	VaultSecretNamespace = "codesphere"
	VaultSecretName      = "cs-vault"
)

type VaultSecretCreator struct {
	client client.Client
}

func NewVaultSecretCreator(c client.Client) *VaultSecretCreator {
	return &VaultSecretCreator{client: c}
}

// CreateSecretFromFile decrypts a SOPS-encrypted vault file and creates or
// updates a Kubernetes secret with its contents in the target cluster.
//
// Each vault entry is mapped to one or more secret keys:
//   - File entries produce a single key equal to the entry name.
//   - Field entries produce "entryName.password" and, when present, "entryName.username".
func (v *VaultSecretCreator) CreateSecretFromFile(ctx context.Context, vaultFile, ageKeyPath, namespace, secretName string) error {
	backend, err := New(TypeSOPS, Options{Path: vaultFile, AgeKey: ageKeyPath})
	if err != nil {
		return err
	}

	return v.CreateSecretFromStore(ctx, backend, namespace, secretName)
}

// CreateSecretFromStore loads secrets through the abstract vault and syncs them
// to a Kubernetes secret.
func (v *VaultSecretCreator) CreateSecretFromStore(ctx context.Context, store Vault, namespace, secretName string) error {
	data, err := store.Load()
	if err != nil {
		return fmt.Errorf("failed to load vault: %w", err)
	}

	return v.CreateSecretFromVault(ctx, data, namespace, secretName)
}

// CreateSecretFromVault signs fresh service account tokens into vault and creates or updates a
// Kubernetes secret with its contents in the target cluster.
//
// Each vault entry is mapped to one or more secret keys:
//   - File entries produce a single key equal to the entry name.
//   - Field entries produce "entryName.password" and, when present, "entryName.username".
func (v *VaultSecretCreator) CreateSecretFromVault(ctx context.Context, vault *files.InstallVault, namespace, secretName string) error {
	// Always create new service accounts tokens during creation to ensure they are always valid and updated.
	if err := secrets.EnsureServiceAccountTokens(vault); err != nil {
		return fmt.Errorf("failed to ensure service account tokens: %w", err)
	}

	secretData, err := vaultToSecretData(vault)
	if err != nil {
		return err
	}

	if err := v.ensureNamespace(ctx, namespace); err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, v.client, secret, func() error {
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = secretData
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to apply secret to cluster: %w", err)
	}

	log.Printf("Successfully created secret '%s' in namespace '%s' with %d entries", secretName, namespace, len(secretData))
	return nil
}

func (v *VaultSecretCreator) ensureNamespace(ctx context.Context, namespace string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, v.client, ns, func() error { return nil }); err != nil {
		return fmt.Errorf("failed to ensure namespace %q: %w", namespace, err)
	}

	return nil
}

// vaultToSecretData converts the entries of an InstallVault into Kubernetes secret data.
// File entries produce a single key equal to the entry name containing the file content.
// Field entries produce "entryName.password" and, when a username is present, "entryName.username".
func vaultToSecretData(vault *files.InstallVault) (map[string][]byte, error) {
	data := make(map[string][]byte)
	for _, entry := range vault.Secrets {
		if entry.File != nil {
			data[entry.Name] = []byte(entry.File.Content)
		} else if entry.Fields != nil {
			data[entry.Name+".password"] = []byte(entry.Fields.Password)
			if entry.Fields.Username != "" {
				data[entry.Name+".username"] = []byte(entry.Fields.Username)
			}
		}
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("no secrets found in vault file")
	}
	return data, nil
}

// ValidateSecretName rejects names Kubernetes would refuse for a Secret.
func ValidateSecretName(name string) error {
	if problems := validation.IsDNS1123Subdomain(name); len(problems) > 0 {
		return fmt.Errorf("invalid secret name %q: %s", name, strings.Join(problems, "; "))
	}

	return nil
}

// WriteSyncTriggerToConfig sets codesphere.override.global.externalSecretsSyncTrigger in the
// merged config file OMS hands to the TS installer. The chart puts it on its ExternalSecrets,
// which ESO only re-syncs when they change, so a changed vault reaches the release in the
// same deploy.
// TODO(CU-869fkgjbv): remove once OMS deploys through the cs-applications app of apps and sets the
// trigger in the values it builds.
func WriteSyncTriggerToConfig(configPath, trigger string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config %s: %w", configPath, err)
	}

	config := map[string]any{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config %s: %w", configPath, err)
	}

	global := childMap(childMap(childMap(config, "codesphere"), "override"), "global")
	global["externalSecretsSyncTrigger"] = trigger

	updated, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 0o600: Same as what the file already has but has no effect here anyway, because the file already exists.
	if err := os.WriteFile(configPath, updated, 0o600); err != nil {
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
