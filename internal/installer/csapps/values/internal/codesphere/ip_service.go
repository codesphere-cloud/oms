// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// IPService is the load balancer workspaces get public IPs from.
type IPService struct {
	LoadBalancerKind string   `yaml:"loadBalancerKind"`
	AddressPools     []string `yaml:"addressPools,omitempty"`
}

// ipServiceValues returns the configured IP service, or one using all MetalLB pools when MetalLB is
// enabled and nothing is configured.
func ipServiceValues(
	ipService *files.IPServiceConfig,
	metalLB *files.MetalLBConfig,
) (*IPService, error) {
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

		return &IPService{LoadBalancerKind: "metallb", AddressPools: poolNames}, nil
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

	return &IPService{LoadBalancerKind: ipService.LoadBalancerKind, AddressPools: ipService.AddressPools}, nil
}
