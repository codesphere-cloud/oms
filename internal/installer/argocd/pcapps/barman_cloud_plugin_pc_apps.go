// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

// BarmanCloudPlugin is the key of the barman-cloud-plugin chart under `applications` in the pc-applications values.
const BarmanCloudPlugin = "barman-cloud-plugin"

// BarmanCloudPluginPcAppsValues derives the pc-applications values for the barman cloud plugin:
// `enabled: false` when the config turns it off, otherwise the registry pull secret with the
// override of managedServiceBackends.barmanCloudPlugin on top. The images are the chart defaults.
func BarmanCloudPluginPcAppsValues(config *files.RootConfig) files.ChartValues {
	if !barmanCloudPluginInstalled(config) {
		return applicationValues(BarmanCloudPlugin, false, nil)
	}

	values := files.ChartValues{"plugin-barman-cloud": files.ChartValues{"imagePullSecrets": imagePullSecrets(config)}}
	if backends := config.ManagedServiceBackends; backends != nil && backends.BarmanCloudPlugin != nil {
		values = util.DeepMergeMaps(values, backends.BarmanCloudPlugin.Override)
	}

	return applicationValues(BarmanCloudPlugin, true, values)
}

// barmanCloudPluginInstalled reports whether the config deploys the barman cloud plugin: it needs
// the CloudNativePG CRDs, so a cluster.pgOperator that is disabled and not managed externally turns
// it off, and managedServiceBackends.barmanCloudPlugin can turn it off too. See
// files.BarmanCloudPluginConfig.
func barmanCloudPluginInstalled(config *files.RootConfig) bool {
	if pg := config.Cluster.PgOperator; pg != nil && !pg.Enabled && !pg.ManagedExternally {
		return false
	}

	backends := config.ManagedServiceBackends

	return backends == nil || backends.BarmanCloudPlugin == nil || backends.BarmanCloudPlugin.Enabled
}
