// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package datacenters builds the values of dataCenter, dataCenters and defaultDataCenterId.
package datacenters

import (
	"fmt"
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// Topology is the installation's data centers as the values need them.
type Topology struct {
	// available are all data centers of the installation, sorted by id so the values do not depend
	// on the order each data center's config lists them in.
	available []files.DatacenterConfig
	current   files.DatacenterConfig
	defaultID int
}

// Resolve checks the configured data centers and finds the installed and the default one.
func Resolve(cfg *files.RootConfig) (Topology, error) {
	available := slices.Clone(cfg.DataCenters)
	if len(available) == 0 {
		available = []files.DatacenterConfig{cfg.Datacenter}
	}

	slices.SortFunc(available, func(a, b files.DatacenterConfig) int { return a.ID - b.ID })

	ids := make([]int, len(available))
	for i, dc := range available {
		ids[i] = dc.ID
	}

	for i := 1; i < len(ids); i++ {
		if ids[i] == ids[i-1] {
			return Topology{}, fmt.Errorf("duplicate data center id %d in .dataCenters", ids[i])
		}
	}

	local := slices.IndexFunc(available, func(dc files.DatacenterConfig) bool {
		return dc.ID == cfg.Datacenter.ID
	})
	if local < 0 {
		return Topology{}, fmt.Errorf(
			"the installed data center .dataCenter (id %d) is missing in .dataCenters (ids: %v)",
			cfg.Datacenter.ID,
			ids,
		)
	}

	if available[local] != cfg.Datacenter {
		return Topology{}, fmt.Errorf(
			"the .dataCenters entry with id %d differs from .dataCenter",
			cfg.Datacenter.ID,
		)
	}

	defaultID := cfg.Datacenter.ID
	if cfg.DefaultDataCenterID != 0 {
		if !slices.Contains(ids, cfg.DefaultDataCenterID) {
			return Topology{}, fmt.Errorf(
				".defaultDataCenterId %d is not one of the configured data centers (ids: %v)",
				cfg.DefaultDataCenterID,
				ids,
			)
		}

		defaultID = cfg.DefaultDataCenterID
	}

	return Topology{available: available, current: cfg.Datacenter, defaultID: defaultID}, nil
}

// Values builds the data center values.
func Values(topology Topology) map[string]any {
	ids := make([]int, len(topology.available))

	dataCenters := make([]any, len(topology.available))
	for i, dc := range topology.available {
		ids[i] = dc.ID
		dataCenters[i] = map[string]any{
			"id":          dc.ID,
			"name":        dc.Name,
			"city":        dc.City,
			"countryCode": dc.CountryCode,
		}
	}

	return map[string]any{
		"dataCenterId":         topology.current.ID,
		"dataCenterName":       topology.current.Name,
		"defaultDataCenterId":  topology.defaultID,
		"availableDataCenters": ids,
		"dataCenters":          dataCenters,
	}
}
