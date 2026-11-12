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

// installerChart is a chart the PC installer installs and pc-applications deploys instead.
type installerChart struct {
	// installed reports whether the installer installs the chart for this config.
	installed func(*files.RootConfig) bool
	name      string
	// step is the installer step that installs the chart. A run that skips it leaves the chart off.
	step string
	// values are the chart values for this config, the chart's override included.
	values func(*files.RootConfig) files.ChartValues
}

func (c installerChart) application(config *files.RootConfig, skipSteps []string) files.ChartValues {
	if installer.IsStepSkipped(*config, skipSteps, c.step) || !c.installed(config) {
		return files.ChartValues{"enabled": false}
	}

	values := c.values(config)
	if len(values) == 0 {
		return nil
	}

	return files.ChartValues{"valuesObject": values}
}

var installerCharts = []installerChart{
	{name: MsBackendS3, step: installer.MsBackendsStep, installed: msBackendS3Installed, values: msBackendS3Values},
	{name: Rgw, step: installer.SetUpClusterStep, installed: rgwInstalled, values: rgwValues},
	{name: RgwLoadBalancer, step: installer.SetUpClusterStep, installed: rgwLoadBalancerInstalled, values: rgwLoadBalancerValues},
}

// InstallerChartsPcAppsValues returns the pc-applications values of the charts the PC installer
// installs: `enabled: false` for a chart the config does not ask for or whose step skipSteps or
// operations.skip skips, otherwise the chart's values with `enabled` left to the pc-applications default.
func InstallerChartsPcAppsValues(config *files.RootConfig, skipSteps []string) files.ChartValues {
	applications := files.ChartValues{}

	for _, chart := range installerCharts {
		if application := chart.application(config, skipSteps); len(application) > 0 {
			applications[chart.name] = application
		}
	}

	return files.ChartValues{"applications": applications}
}

// enabled mirrors the installer's isComponentEnabled: a component without `enabled` is enabled.
func enabled(flag *bool) bool {
	return flag == nil || *flag
}

// hasCeph reports whether the config has a ceph cluster to serve rgw.
func hasCeph(config *files.RootConfig) bool {
	return len(config.Ceph.Hosts) > 0
}
