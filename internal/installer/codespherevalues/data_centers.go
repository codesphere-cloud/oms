// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codespherevalues

import (
	"fmt"
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

type dataCenterTopology struct {
	// available are all data centers of the installation, sorted by id so the values do not depend
	// on the order each data center's config lists them in.
	available []files.DatacenterConfig
	current   files.DatacenterConfig
	defaultID int
}

func resolveDataCenterTopology(cfg *files.RootConfig) (dataCenterTopology, error) {
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
			return dataCenterTopology{}, fmt.Errorf("duplicate data center id %d in .dataCenters", ids[i])
		}
	}

	local := slices.IndexFunc(available, func(dc files.DatacenterConfig) bool { return dc.ID == cfg.Datacenter.ID })
	if local < 0 {
		return dataCenterTopology{}, fmt.Errorf("the installed data center .dataCenter (id %d) is missing in .dataCenters (ids: %v)", cfg.Datacenter.ID, ids)
	}

	if available[local] != cfg.Datacenter {
		return dataCenterTopology{}, fmt.Errorf("the .dataCenters entry with id %d differs from .dataCenter", cfg.Datacenter.ID)
	}

	defaultID := cfg.Datacenter.ID
	if cfg.DefaultDataCenterID != 0 {
		if !slices.Contains(ids, cfg.DefaultDataCenterID) {
			return dataCenterTopology{}, fmt.Errorf(".defaultDataCenterId %d is not one of the configured data centers (ids: %v)", cfg.DefaultDataCenterID, ids)
		}

		defaultID = cfg.DefaultDataCenterID
	}

	return dataCenterTopology{available: available, current: cfg.Datacenter, defaultID: defaultID}, nil
}
