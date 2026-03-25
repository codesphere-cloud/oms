// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package bootstrap

import "github.com/codesphere-cloud/oms/internal/installer/files"

// DefaultCodesphereDeployConfig returns a fresh copy of the default Codesphere
// deployConfig used by the bootstrap flows.
func DefaultCodesphereDeployConfig() files.DeployConfig {
	return files.DeployConfig{
		Images: map[string]files.ImageConfig{
			"ubuntu-24.04": {
				Name:           "Ubuntu 24.04",
				SupportedUntil: "2028-05-31",
				Flavors: map[string]files.FlavorConfig{
					"default": {
						Image: files.ImageRef{
							BomRef: "workspace-agent-24.04",
						},
						Pool: map[int]int{
							1: 0,
							2: 0,
							3: 0,
						},
					},
				},
			},
		},
	}
}

// DefaultCodespherePlans returns a fresh copy of the default Codesphere plans
// used by the bootstrap flows.
func DefaultCodespherePlans() files.PlansConfig {
	return files.PlansConfig{
		HostingPlans: map[int]files.HostingPlan{
			1: {
				CPUTenth:      20,
				GPUParts:      0,
				MemoryMb:      4096,
				StorageMb:     20480,
				TempStorageMb: 1024,
			},
			2: {
				CPUTenth:      40,
				GPUParts:      0,
				MemoryMb:      8192,
				StorageMb:     40960,
				TempStorageMb: 1024,
			},
			3: {
				CPUTenth:      80,
				GPUParts:      0,
				MemoryMb:      16384,
				StorageMb:     40960,
				TempStorageMb: 1024,
			},
		},
		WorkspacePlans: map[int]files.WorkspacePlan{
			1: {
				Name:          "Standard",
				HostingPlanID: 1,
				MaxReplicas:   3,
				OnDemand:      true,
			},
			2: {
				Name:          "Big",
				HostingPlanID: 2,
				MaxReplicas:   3,
				OnDemand:      true,
			},
			3: {
				Name:          "Pro",
				HostingPlanID: 3,
				MaxReplicas:   3,
				OnDemand:      true,
			},
		},
	}
}
