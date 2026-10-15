// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package pcapps provides app specific logic to configure apps that are part of the pc-applications.
package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// RookCephPcAppsValues derives the pc-applications values for rook-ceph from the ceph block in the config.yaml,
// so distributions with a non-default kubelet root (e.g. k0s) get working CSI mounts.
// They are the base of the pc-apps values: an explicit `pcApps` block in config.yaml and any --pc-apps-values file
// still override them.
// Returns nil when no related value is set, leaving the chart default's untouched.
func RookCephPcAppsValues(config *files.RootConfig) files.ChartValues {
	if config.Ceph.CsiKubeletDir == "" {
		return nil
	}

	return files.ChartValues{
		"applications": files.ChartValues{
			"rook-ceph": files.ChartValues{
				"csi": files.ChartValues{
					"kubeletDirPath": config.Ceph.CsiKubeletDir,
				},
			},
		},
	}
}
