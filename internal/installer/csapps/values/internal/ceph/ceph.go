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

// Ceph are the CephFS settings of the ceph key.
type Ceph struct {
	CredentialsSecretName     string `yaml:"credentialsSecretName"`
	StorageClass              string `yaml:"storageClass"`
	MonEndpointsConfigMapName string `yaml:"monEndpointsConfigMapName"`
	MDSNamespace              string `yaml:"mdsNamespace"`
	// ActiveMDS is a string, as the old pc-installer passed it.
	ActiveMDS string `yaml:"activeMds"`
}

// Values builds the CephFS settings. With workspacePVCStorage, workspaces use PVCs instead of
// CephFS.
func Values(ceph files.CephConfig, workspacePVCStorage bool) (Ceph, error) {
	values := Ceph{
		CredentialsSecretName:     "ceph-admin-credentials",
		StorageClass:              "codesphere",
		MonEndpointsConfigMapName: "ceph-mon-endpoints",
		MDSNamespace:              "",
		ActiveMDS:                 "0",
	}
	if workspacePVCStorage {
		return values, nil
	}

	name, active, err := codesphereVolume(ceph.Volumes)
	if err != nil {
		return Ceph{}, err
	}

	values.MDSNamespace = name
	values.ActiveMDS = fmt.Sprint(active)

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
