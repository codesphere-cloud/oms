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

// errorPageServerImage is the image the Codesphere umbrella chart deploys error-page-server with.
const errorPageServerImage = "error-page-server"

// ErrorPageServerPcAppsValues enables the error-page-server application,
// once the package's Codesphere umbrella chart no longer deploys error-page-server.
// The package shows that by leaving the image out of the codesphere component of its BOM.
// Until then it returns nil and the application stays disabled,
// since both would render the same error-page-server Service.
// The chart reads the Codesphere globals, as it did as a subchart of the umbrella chart.
func ErrorPageServerPcAppsValues(config *files.RootConfig, installerBom *bom.Config) (files.ChartValues, error) {
	if umbrellaDeploysErrorPageServer(installerBom) {
		return nil, nil
	}

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
				"enabled":      true,
				"valuesObject": global,
			},
		},
	}, nil
}

// umbrellaDeploysErrorPageServer reports whether the package's Codesphere umbrella chart
// still deploys error-page-server.
// A BOM without a codesphere component counts as yes,
// so a broken package never enables a second error-page-server.
func umbrellaDeploysErrorPageServer(installerBom *bom.Config) bool {
	images, err := installerBom.GetCodesphereContainerImages()
	if err != nil {
		return true
	}

	return images[errorPageServerImage] != ""
}
