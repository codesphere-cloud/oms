// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package datacenters builds the values of dataCenter, dataCenters and defaultDataCenterId.
package datacenters

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// DataCenters are the data center this config installs and all the data centers of the private
// cloud.
type DataCenters struct {
	DataCenterID         int                      `yaml:"dataCenterId"`
	DataCenterName       string                   `yaml:"dataCenterName"`
	DefaultDataCenterID  int                      `yaml:"defaultDataCenterId"`
	AvailableDataCenters []int                    `yaml:"availableDataCenters"`
	DataCenters          []files.DatacenterConfig `yaml:"dataCenters"`
}

// Values checks the configured data centers and builds their values. Without .dataCenters, the
// target data center is the only one.
func Values(cfg *files.RootConfig) (DataCenters, error) {
	if err := cfg.ValidateDataCenters(); err != nil {
		return DataCenters{}, fmt.Errorf("invalid data centers: %w", err)
	}

	dataCenters := sortedByID(cfg.AllDataCenters())

	return DataCenters{
		DataCenterID:         cfg.Datacenter.ID,
		DataCenterName:       cfg.Datacenter.Name,
		DefaultDataCenterID:  defaultDataCenterID(cfg),
		AvailableDataCenters: ids(dataCenters),
		DataCenters:          dataCenters,
	}, nil
}

func sortedByID(dataCenters []files.DatacenterConfig) []files.DatacenterConfig {
	return slices.SortedFunc(slices.Values(dataCenters), func(a, b files.DatacenterConfig) int {
		return cmp.Compare(a.ID, b.ID)
	})
}

func ids(dataCenters []files.DatacenterConfig) []int {
	ids := make([]int, len(dataCenters))
	for i, dc := range dataCenters {
		ids[i] = dc.ID
	}

	return ids
}

func defaultDataCenterID(cfg *files.RootConfig) int {
	if cfg.DefaultDataCenterID == 0 {
		return cfg.Datacenter.ID
	}

	return cfg.DefaultDataCenterID
}
