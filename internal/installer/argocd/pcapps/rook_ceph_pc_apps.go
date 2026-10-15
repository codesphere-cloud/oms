// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// RookCephPcAppsValues derives the pc-applications values for rook-ceph from ceph.csiKubeletDir,
// so distributions with a non-default kubelet root (e.g. k0s) get working CSI mounts. Like
// OpenFgaPcAppsValues they are only a *base*: a kubeletDirPath set under `pcApps` in config.yaml
// or in a --pc-apps-values file wins. Returns nil when the directory is not configured, leaving
// the chart default untouched.
func RookCephPcAppsValues(config *files.RootConfig) files.ChartValues {
	if config.Ceph.CsiKubeletDir == "" {
		return nil
	}

	return files.ChartValues{
		"applications": files.ChartValues{
			"rook-ceph": files.ChartValues{
				"valuesObject": files.ChartValues{
					"csi": files.ChartValues{
						"kubeletDirPath": config.Ceph.CsiKubeletDir,
					},
				},
			},
		},
	}
}
