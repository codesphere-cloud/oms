// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

const (
	barmanCloudPluginBomComponent = "barman-cloud-plugin"
	barmanCloudPluginImageKey     = "barmanCloudPlugin"
	barmanCloudSidecarImageKey    = "barmanCloudPluginSidecar"
)

// barmanCloudPluginInstalled reports whether the config deploys the barman cloud plugin. The
// installer also gates it on managedServiceBackends.pgOperator, a key oms does not read, so it is
// never off here, not even where cluster.pgOperator is managed externally. Mirrors installMsBackends
// in codesphere-monorepo packages/script-lib/src/ts/private-cloud/ms-backends.ts.
func barmanCloudPluginInstalled(config *files.RootConfig) bool {
	backends := config.ManagedServiceBackends

	return backends == nil || backends.BarmanCloudPlugin == nil || backends.BarmanCloudPlugin.Enabled
}

// barmanCloudPluginValues points the plugin and its sidecar at the images of the package BOM and
// the registry pull secret, then applies managedServiceBackends.barmanCloudPlugin.override on top.
func barmanCloudPluginValues(config *files.RootConfig, packageBom *bom.Config) (files.ChartValues, error) {
	chart := files.ChartValues{"imagePullSecrets": imagePullSecrets(config)}

	for key, value := range map[string]string{"image": barmanCloudPluginImageKey, "sidecarImage": barmanCloudSidecarImageKey} {
		image, found, err := bomImageOf(packageBom, config, barmanCloudPluginBomComponent, value)
		if err != nil {
			return nil, err
		}

		if found {
			chart[key] = files.ChartValues{"registry": image.Registry, "repository": image.Repository, "tag": image.Tag}
		}
	}

	values := files.ChartValues{"plugin-barman-cloud": chart}
	if backends := config.ManagedServiceBackends; backends != nil && backends.BarmanCloudPlugin != nil {
		values = util.DeepMergeMaps(values, backends.BarmanCloudPlugin.Override)
	}

	return values, nil
}
