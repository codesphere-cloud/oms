// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// applicationValues returns the pc-applications values of one chart of the PC installer:
// `enabled: false` when the run does not install it, otherwise its values under `valuesObject`
// with `enabled` left to the pc-applications default. A chart that is installed and has no values
// gets no entry.
func applicationValues(name string, installed bool, values files.ChartValues) files.ChartValues {
	switch {
	case !installed:
		return files.ChartValues{"applications": files.ChartValues{name: files.ChartValues{"enabled": false}}}
	case len(values) > 0:
		return files.ChartValues{"applications": files.ChartValues{name: files.ChartValues{"valuesObject": values}}}
	default:
		return files.ChartValues{"applications": files.ChartValues{}}
	}
}

// stepRuns reports whether the run executes the installer step, which neither skipSteps nor
// operations.skip of the config skip.
func stepRuns(config *files.RootConfig, skipSteps []string, step string) bool {
	return !installer.IsStepSkipped(*config, skipSteps, step)
}

// enabled mirrors the installer's isComponentEnabled: a component without `enabled` is enabled.
func enabled(flag *bool) bool {
	return flag == nil || *flag
}

// hasCeph reports whether the config has a ceph cluster to serve rgw.
func hasCeph(config *files.RootConfig) bool {
	return len(config.Ceph.Hosts) > 0
}
