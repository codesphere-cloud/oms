// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package ceph builds the values of the ceph key.
package ceph

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// Values builds the CephFS settings. With workspacePVCStorage, workspaces use PVCs instead of
// CephFS.
func Values(ceph files.CephConfig, workspacePVCStorage bool) map[string]any {
	return map[string]any{"ceph": cephValues(ceph, workspacePVCStorage)}
}

func cephValues(ceph files.CephConfig, workspacePVCStorage bool) map[string]any {
	values := map[string]any{
		"credentialsSecretName":     "ceph-admin-credentials",
		"storageClass":              "codesphere",
		"monEndpointsConfigMapName": "ceph-mon-endpoints",
		"mdsNamespace":              "",
		"activeMds":                 "0",
	}
	if workspacePVCStorage {
		return values
	}

	name, active := "codesphere", 7
	// Codesphere only uses one volume. The installer rejects configs with more.
	for volumeName, volume := range ceph.Volumes {
		name, active = volumeName, volume.MDS.Active
	}

	values["mdsNamespace"] = name
	values["activeMds"] = fmt.Sprint(active)

	return values
}
