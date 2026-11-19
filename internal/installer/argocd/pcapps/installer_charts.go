// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// Each name is the key of the chart under `applications` in the pc-applications values.
const (
	MsBackendS3       = "ms-backend-s3"
	Rgw               = "rgw"
	RgwLoadBalancer   = "rgw-load-balancer"
	PostgresOperator  = "postgres-operator"
	BarmanCloudPlugin = "barman-cloud-plugin"
)

// InstallerChartsPcAppsValues returns the pc-applications values of the charts the PC installer
// installs: `enabled: false` for a chart the config does not ask for or whose step skipSteps or
// operations.skip skips, otherwise the chart's values with `enabled` left to the pc-applications default.
// The images of the charts come from packageBom, which may be nil to leave the chart defaults.
func InstallerChartsPcAppsValues(config *files.RootConfig, skipSteps []string, packageBom *bom.Config) (files.ChartValues, error) {
	msBackendsRuns := !installer.IsStepSkipped(*config, skipSteps, installer.MsBackendsStep)
	setUpClusterRuns := !installer.IsStepSkipped(*config, skipSteps, installer.SetUpClusterStep)

	applications := files.ChartValues{}
	setApplication(applications, MsBackendS3, msBackendsRuns && msBackendS3Installed(config), msBackendS3Values(config))
	setApplication(applications, Rgw, setUpClusterRuns && rgwInstalled(config), rgwValues(config))
	setApplication(applications, RgwLoadBalancer, setUpClusterRuns && rgwLoadBalancerInstalled(config), rgwLoadBalancerValues(config))

	postgresOperatorRuns := setUpClusterRuns && postgresOperatorInstalled(config)
	barmanCloudPluginRuns := msBackendsRuns && barmanCloudPluginInstalled(config)

	// The values of an app the run does not install are not derived, so a BOM image they cannot be
	// built from cannot fail a run that never uses it.
	var postgresOperator, barmanCloudPlugin files.ChartValues

	if postgresOperatorRuns {
		var err error
		if postgresOperator, err = postgresOperatorValues(config, packageBom); err != nil {
			return nil, fmt.Errorf("configuring %s application: %w", PostgresOperator, err)
		}
	}

	if barmanCloudPluginRuns {
		var err error
		if barmanCloudPlugin, err = barmanCloudPluginValues(config, packageBom); err != nil {
			return nil, fmt.Errorf("configuring %s application: %w", BarmanCloudPlugin, err)
		}
	}

	setApplication(applications, PostgresOperator, postgresOperatorRuns, postgresOperator)
	setApplication(applications, BarmanCloudPlugin, barmanCloudPluginRuns, barmanCloudPlugin)

	return files.ChartValues{"applications": applications}, nil
}

// setApplication sets the entry of a chart: `enabled: false` when the run does not install it,
// otherwise its values, and no entry when it has none.
func setApplication(applications files.ChartValues, name string, installed bool, values files.ChartValues) {
	switch {
	case !installed:
		applications[name] = files.ChartValues{"enabled": false}
	case len(values) > 0:
		applications[name] = files.ChartValues{"valuesObject": values}
	}
}

// enabled mirrors the installer's isComponentEnabled: a component without `enabled` is enabled.
func enabled(flag *bool) bool {
	return flag == nil || *flag
}

// hasCeph reports whether the config has a ceph cluster to serve rgw.
func hasCeph(config *files.RootConfig) bool {
	return len(config.Ceph.Hosts) > 0
}
