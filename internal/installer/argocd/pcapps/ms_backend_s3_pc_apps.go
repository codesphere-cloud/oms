// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/util"
)

// clusterSecretStore is the ClusterSecretStore that reads the cs-vault secret.
const clusterSecretStore = "kubernetes-in-cluster"

func msBackendS3Config(config *files.RootConfig) *files.S3ManagedServiceConfig {
	if config.ManagedServiceBackends == nil {
		return nil
	}

	return config.ManagedServiceBackends.S3
}

// msBackendS3Installed: unless the config mentions it, the installer turns the backend off on
// installations that keep workspaces on PVCs and have no ceph. Mirrors the managedServiceBackends.s3
// default passed to installMsBackends in codesphere-monorepo
// packages/script-lib/src/ts/private-cloud/private-cloud-installer.ts.
func msBackendS3Installed(config *files.RootConfig) bool {
	if s3 := msBackendS3Config(config); s3 != nil {
		return enabled(s3.Enabled)
	}

	return !config.Codesphere.IsEnabled(files.WorkspacePVCStorageFlag) || hasCeph(config)
}

// msBackendS3Values reads the admin credentials from the vault entries a private cloud has; the
// chart's default entries exist only in the SaaS vault. Helm replaces lists, so the secret is complete.
func msBackendS3Values(config *files.RootConfig) files.ChartValues {
	values := files.ChartValues{
		"extraSecrets": []any{
			map[string]any{
				"name":  "app-secrets",
				"store": clusterSecretStore,
				"secretKeys": []any{
					vaultSecretKey("RGW_ADMIN_ACCESS_KEY_ID", files.SecretRgwAdminAccessKey),
					vaultSecretKey("RGW_ADMIN_SECRET_ACCESS_KEY", files.SecretRgwAdminSecretKey),
				},
			},
		},
	}

	if s3 := msBackendS3Config(config); s3 != nil {
		values = util.DeepMergeMaps(values, s3.Override)
	}

	return values
}

// vaultSecretKey reads one key of a chart's ExternalSecret from the password of a vault entry,
// which the cs-vault secret holds under vault.PasswordKey.
func vaultSecretKey(secretKey, entry string) map[string]any {
	return map[string]any{"secretKey": secretKey, "remoteRef": vault.VaultSecretName, "property": vault.PasswordKey(entry)}
}
