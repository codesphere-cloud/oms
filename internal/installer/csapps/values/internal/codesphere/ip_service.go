// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const metalLBKind = "metallb"

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
	if metalLB == nil || !metalLB.Enabled {
		if ipService == nil {
			return nil, nil
		}

		return &IPService{LoadBalancerKind: ipService.LoadBalancerKind, AddressPools: ipService.AddressPools}, nil
	}

	poolNames := metalLBPoolNames(metalLB)

	if ipService == nil {
		return &IPService{LoadBalancerKind: metalLBKind, AddressPools: poolNames}, nil
	}

	if ipService.LoadBalancerKind != metalLBKind {
		return nil, fmt.Errorf(
			"if metallb is enabled in the cluster config, codesphere.ipService must also use metallb",
		)
	}

	for _, pool := range ipService.AddressPools {
		if !slices.Contains(poolNames, pool) {
			return nil, fmt.Errorf("pool %s is not defined in the metallb config (%v)", pool, poolNames)
		}
	}

	return &IPService{LoadBalancerKind: metalLBKind, AddressPools: ipService.AddressPools}, nil
}

func metalLBPoolNames(metalLB *files.MetalLBConfig) []string {
	poolNames := make([]string, len(metalLB.Pools))
	for i, pool := range metalLB.Pools {
		poolNames[i] = pool.Name
	}

	return poolNames
}
