// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"fmt"
	"time"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	csappsvalues "github.com/codesphere-cloud/oms/internal/installer/csapps/values"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
)

// ErrorPageServerPcAppsValues passes the error-page-server application the Codesphere globals,
// as the chart read them as a subchart of the Codesphere umbrella chart.
// It leaves enabled to the pc-applications chart:
// the pc-applications version enables the application in step with the umbrella chart dropping it,
// since both would render the same error-page-server Service.
func ErrorPageServerPcAppsValues(config *files.RootConfig, installerBom *bom.Config) (files.ChartValues, error) {
	codesphereValues, err := csappsvalues.Build(config, installerBom)
	if err != nil {
		return nil, fmt.Errorf("failed to build the Codesphere values: %w", err)
	}

	global := csappsvalues.MergeOverride(map[string]any{"global": codesphereValues["global"]}, map[string]any{
		"global": map[string]any{
			"externalSecretsVaultSecret": vault.VaultSecretName,
			// ESO only re-reads the vault when the ExternalSecret changes,
			// so a new trigger on every install brings refreshed service user tokens to the pod.
			"externalSecretsSyncTrigger": time.Now().UTC().Format(time.RFC3339Nano),
		},
	})

	return files.ChartValues{
		"applications": files.ChartValues{
			"error-page-server": files.ChartValues{
				"valuesObject": global,
			},
		},
	}, nil
}
