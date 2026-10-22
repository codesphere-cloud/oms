// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package ceph builds the values of the ceph key.
package ceph

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const (
	defaultVolumeName = "codesphere"
	defaultActiveMDS  = 7
)

// Values builds the CephFS settings. With workspacePVCStorage, workspaces use PVCs instead of
// CephFS.
func Values(ceph files.CephConfig, workspacePVCStorage bool) (map[string]any, error) {
	values, err := cephValues(ceph, workspacePVCStorage)
	if err != nil {
		return nil, err
	}

	return map[string]any{"ceph": values}, nil
}

func cephValues(ceph files.CephConfig, workspacePVCStorage bool) (map[string]any, error) {
	values := map[string]any{
		"credentialsSecretName":     "ceph-admin-credentials",
		"storageClass":              "codesphere",
		"monEndpointsConfigMapName": "ceph-mon-endpoints",
		"mdsNamespace":              "",
		"activeMds":                 "0",
	}
	if workspacePVCStorage {
		return values, nil
	}

	name, active, err := codesphereVolume(ceph.Volumes)
	if err != nil {
		return nil, err
	}

	values["mdsNamespace"] = name
	values["activeMds"] = fmt.Sprint(active)

	return values, nil
}

// codesphereVolume returns the name and active MDS count of the one CephFS volume Codesphere uses.
func codesphereVolume(volumes map[string]files.CephVolume) (string, int, error) {
	if len(volumes) > 1 {
		return "", 0, fmt.Errorf("only one ceph volume is supported, got %d", len(volumes))
	}

	for name, volume := range volumes {
		return name, volume.MDS.Active, nil
	}

	return defaultVolumeName, defaultActiveMDS, nil
}
