// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// Each name is the key of the chart under `applications` in the pc-applications values.
const (
	MsBackendS3     = "ms-backend-s3"
	Rgw             = "rgw"
	RgwLoadBalancer = "rgw-load-balancer"
)

// InstallerChartsPcAppsValues returns the pc-applications values of the charts the PC installer
// installs: `enabled: false` for a chart the config does not ask for or whose step skipSteps or
// operations.skip skips, otherwise the chart's values with `enabled` left to the pc-applications default.
func InstallerChartsPcAppsValues(config *files.RootConfig, skipSteps []string) files.ChartValues {
	msBackendsRuns := !installer.IsStepSkipped(*config, skipSteps, installer.MsBackendsStep)
	setUpClusterRuns := !installer.IsStepSkipped(*config, skipSteps, installer.SetUpClusterStep)

	applications := files.ChartValues{}
	setApplication(applications, MsBackendS3, msBackendsRuns && msBackendS3Installed(config), msBackendS3Values(config))
	setApplication(applications, Rgw, setUpClusterRuns && rgwInstalled(config), rgwValues(config))
	setApplication(applications, RgwLoadBalancer, setUpClusterRuns && rgwLoadBalancerInstalled(config), rgwLoadBalancerValues(config))

	return files.ChartValues{"applications": applications}
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
