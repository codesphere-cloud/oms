// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"log"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type VaultSecretCreator struct {
	client client.Client
}

func NewVaultSecretCreator(c client.Client) *VaultSecretCreator {
	return &VaultSecretCreator{client: c}
}

// CreateSecretFromVault decrypts a SOPS-encrypted vault file and creates or
// updates a Kubernetes secret with its contents in the target cluster.
//
// Each vault entry is mapped to one or more secret keys:
//   - File entries produce a single key equal to the entry name.
//   - Field entries produce "entryName.password" and, when present, "entryName.username".
func (v *VaultSecretCreator) CreateSecretFromVault(ctx context.Context, vaultFile, ageKeyPath, namespace, secretName string) error {
	decrypted, err := DecryptFileWithSOPS(vaultFile, ageKeyPath)
	if err != nil {
		return fmt.Errorf("failed to decrypt vault file: %w", err)
	}

	vault := &files.InstallVault{}
	if err := vault.Unmarshal(decrypted); err != nil {
		return fmt.Errorf("failed to parse vault file: %w", err)
	}

	// Always create new service accounts tokens during creation to ensure they are always valid and updated.
	if err := secrets.EnsureServiceAccountTokens(vault); err != nil {
		return fmt.Errorf("failed to ensure service account tokens: %w", err)
	}

	secretData, err := vaultToSecretData(vault)
	if err != nil {
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
