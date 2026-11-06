// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
)

const (
	workspacePvcStorageFlag = "workspace-pvc-storage"
	// clusterSecretStore is the ClusterSecretStore that reads the cs-vault secret.
	clusterSecretStore = "kubernetes-in-cluster"
)

var msBackendS3Block = []string{"managedServiceBackends", "s3"}

// msBackendS3Installed: unless the config mentions it, the installer turns the backend off on
// installations that keep workspaces on PVCs and have no ceph.
func msBackendS3Installed(config installerConfig) bool {
	if config.has(msBackendS3Block...) {
		return config.componentEnabled(msBackendS3Block...)
	}

	return !config.internalFlagEnabled(workspacePvcStorageFlag) || config.has(cephBlock)
}

// msBackendS3Values points the chart at the vault entries a private-cloud vault has: the chart's
// own msBackendS3accessKeyId and msBackendS3secretAccessKey exist only in the SaaS vault. Helm
// replaces lists instead of merging them, so the secret is written out in full.
func msBackendS3Values(installerConfig) files.ChartValues {
	return files.ChartValues{
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
}

// vaultSecretKey reads one key of a chart's ExternalSecret from the password of a vault entry,
// which the cs-vault secret holds under vault.PasswordKey.
func vaultSecretKey(secretKey, entry string) map[string]any {
	return map[string]any{"secretKey": secretKey, "remoteRef": vault.VaultSecretName, "property": vault.PasswordKey(entry)}
}
