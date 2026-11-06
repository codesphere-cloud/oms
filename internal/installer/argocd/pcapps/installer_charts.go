// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

// The charts that move from the PC installer to pc-applications. Each name is the key of the chart
// under `applications` in the pc-applications values and the Helm release the installer installs, so
// it is spelled once, here.
const (
	MsBackendS3     = "ms-backend-s3"
	Rgw             = "rgw"
	RgwLoadBalancer = "rgw-load-balancer"
)

// cephBlock is the config block that makes the installer install the charts that need ceph.
const cephBlock = "ceph"

// installerChart is a chart the PC installer installs and pc-applications deploys instead.
type installerChart struct {
	name string
	// step is the installer step that installs the chart. A run that skips it leaves the chart off.
	step string
	// block is the chart's block in the install config, which holds its `enabled` and `override`.
	block []string
	// installed reports whether the installer installs the chart for this config.
	installed func(installerConfig) bool
	// values are the chart values derived from the config. The override of the block wins over them.
	values func(installerConfig) files.ChartValues
}

var installerCharts = []installerChart{
	{name: MsBackendS3, step: installer.MsBackendsStep, block: msBackendS3Block, installed: msBackendS3Installed, values: msBackendS3Values},
	{name: Rgw, step: installer.SetUpClusterStep, block: rgwBlock, installed: rgwInstalled},
	{name: RgwLoadBalancer, step: installer.SetUpClusterStep, block: rgwLoadBalancerBlock, installed: rgwLoadBalancerInstalled, values: rgwLoadBalancerValues},
}

// InstallerChartsPcAppsValues supplies the pc-applications values of the charts the PC installer
// installs: the values derived from the install config for a chart the config asks for, and
// `enabled: false` for a chart it does not ask for or whose installer step the run skips. A chart the
// config asks for gets no `enabled`, so the pc-applications default decides. The values are the base
// of the pc-apps values: an explicit `pcApps` block in config.yaml and any --pc-apps-values file
// still override them.
func InstallerChartsPcAppsValues(config map[string]any, skipped func(step string) bool) files.ChartValues {
	applications := files.ChartValues{}

	for _, chart := range installerCharts {
		if application := chart.application(config, skipped); len(application) > 0 {
			applications[chart.name] = application
		}
	}

	return files.ChartValues{"applications": applications}
}

func (c installerChart) application(config installerConfig, skipped func(step string) bool) files.ChartValues {
	if skipped(c.step) || !c.installed(config) {
		return files.ChartValues{"enabled": false}
	}

	var values files.ChartValues
	if c.values != nil {
		values = c.values(config)
	}

	values = util.DeepMergeMaps(values, config.override(c.block...))
	if len(values) == 0 {
		return nil
	}

	return files.ChartValues{"valuesObject": values}
}
