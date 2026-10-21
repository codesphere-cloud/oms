// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// ipServiceValues returns the configured IP service, or one using all MetalLB pools when MetalLB is
// enabled and nothing is configured.
func ipServiceValues(
	ipService *files.IPServiceConfig,
	metalLB *files.MetalLBConfig,
) (map[string]any, error) {
	metalLBEnabled := metalLB != nil && metalLB.Enabled
	poolNames := []string{}

	if metalLBEnabled {
		for _, pool := range metalLB.Pools {
			poolNames = append(poolNames, pool.Name)
		}
	}

	if ipService == nil {
		if !metalLBEnabled {
			return nil, nil
		}

		return map[string]any{"loadBalancerKind": "metallb", "addressPools": poolNames}, nil
	}

	if metalLBEnabled && ipService.LoadBalancerKind != "metallb" {
		return nil, fmt.Errorf(
			"if metallb is enabled in the cluster config, codesphere.ipService must also use metallb",
		)
	}

	if metalLBEnabled {
		for _, pool := range ipService.AddressPools {
			if !slices.Contains(poolNames, pool) {
				return nil, fmt.Errorf(
					"pool %s is not defined in the metallb config (%v)",
					pool,
					poolNames,
				)
			}
		}
	}

	values := map[string]any{"loadBalancerKind": ipService.LoadBalancerKind}
	if ipService.AddressPools != nil {
		values["addressPools"] = ipService.AddressPools
	}

	return values, nil
}
