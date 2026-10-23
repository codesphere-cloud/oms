// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package datacenters builds the values of dataCenter, dataCenters and defaultDataCenterId.
package datacenters

import (
	"fmt"
	"maps"
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// DataCenters are the data center this config installs and all the data centers of the private
// cloud.
type DataCenters struct {
	DataCenterID         int          `yaml:"dataCenterId"`
	DataCenterName       string       `yaml:"dataCenterName"`
	DefaultDataCenterID  int          `yaml:"defaultDataCenterId"`
	AvailableDataCenters []int        `yaml:"availableDataCenters"`
	DataCenters          []DataCenter `yaml:"dataCenters"`
}

// DataCenter is one of the private cloud's data centers.
type DataCenter struct {
	ID          int    `yaml:"id"`
	Name        string `yaml:"name"`
	City        string `yaml:"city"`
	CountryCode string `yaml:"countryCode"`
}

// Values checks the configured data centers and builds their values. Without .dataCenters, the
// target data center is the only one.
func Values(cfg *files.RootConfig) (DataCenters, error) {
	configured := cfg.DataCenters
	// For single DC installs cfg.DataCenters is optional
	if len(configured) == 0 {
		configured = []files.DatacenterConfig{cfg.Datacenter}
	}

	byID := make(map[int]files.DatacenterConfig, len(configured))
	for _, dc := range configured {
		// 0 means an unset .defaultDataCenterId, so no data center may have it.
		if dc.ID == 0 {
			return DataCenters{}, fmt.Errorf("data center %q has id 0, ids start at 1", dc.Name)
		}

		if _, ok := byID[dc.ID]; ok {
			return DataCenters{}, fmt.Errorf("duplicate data center id %d in .dataCenters", dc.ID)
		}

		byID[dc.ID] = dc
	}

	ids := slices.Sorted(maps.Keys(byID))

	target, ok := byID[cfg.Datacenter.ID]
	if !ok {
		return DataCenters{}, fmt.Errorf(
			".dataCenters (ids: %v) is missing .dataCenter (id %d)",
			ids,
			cfg.Datacenter.ID,
		)
	}

	if target != cfg.Datacenter {
		return DataCenters{}, fmt.Errorf("the .dataCenters entry with id %d differs from .dataCenter", cfg.Datacenter.ID)
	}

	defaultID := cfg.Datacenter.ID
	if cfg.DefaultDataCenterID != 0 {
		if _, ok := byID[cfg.DefaultDataCenterID]; !ok {
			return DataCenters{}, fmt.Errorf(
				".defaultDataCenterId %d is not one of the configured data centers (ids: %v)",
				cfg.DefaultDataCenterID,
				ids,
			)
		}

		defaultID = cfg.DefaultDataCenterID
	}

	dataCenters := make([]DataCenter, len(ids))
	for i, id := range ids {
		dc := byID[id]
		dataCenters[i] = DataCenter{ID: dc.ID, Name: dc.Name, City: dc.City, CountryCode: dc.CountryCode}
	}

	return DataCenters{
		DataCenterID:         cfg.Datacenter.ID,
		DataCenterName:       cfg.Datacenter.Name,
		DefaultDataCenterID:  defaultID,
		AvailableDataCenters: ids,
		DataCenters:          dataCenters,
	}, nil
}
