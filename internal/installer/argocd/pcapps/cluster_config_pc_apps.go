// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

// ClusterConfigPcAppsValues hands cluster configuration to ArgoCD after the
// package removes the legacy TS component. Older package BOMs retain that
// component, so their TS installation remains in charge.
func ClusterConfigPcAppsValues(config *files.RootConfig, vault *files.InstallVault, packageBOM *bom.Config) (files.ChartValues, error) {
	if packageBOM == nil {
		return nil, nil
	}

	if _, legacy := packageBOM.Components["cluster-config"]; legacy {
		return nil, nil
	}

	if vault == nil {
		return nil, fmt.Errorf("vault is required for cluster-config registry credentials")
	}

	password := vault.GetSecret(files.SecretRegistryPassword)
	if password == nil || password.Fields == nil || password.Fields.Password == "" {
		return nil, fmt.Errorf("registry password is required for cluster-config")
	}

	credentials := files.ChartValues{"password": password.Fields.Password}
	if config.Registry != nil && config.Registry.Server != "" {
		credentials["registry"] = config.Registry.Server
	}

	if username := vault.GetSecret(files.SecretRegistryUsername); username != nil && username.Fields != nil && username.Fields.Password != "" {
		credentials["username"] = username.Fields.Password
	}

	chartValues := files.ChartValues{"registryCredentials": credentials}
	if legacy := config.Cluster.ClusterConfig; legacy != nil {
		chartValues = util.DeepMergeMaps(chartValues, legacy.Override)
	}

	return files.ChartValues{"applications": files.ChartValues{"cluster-config": files.ChartValues{"enabled": true, "valuesObject": chartValues}}}, nil
}
