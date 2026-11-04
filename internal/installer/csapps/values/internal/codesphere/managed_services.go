// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const (
	defaultProviderScope = "global"

	// Landscape managed services run their workspaces on this plan, and 20 (CS Micro) is the only
	// plan ID every environment has.
	// TODO(CU-869cwbjnx): use a plan read from the database instead.
	defaultManagedServicePlanID = 20
)

// managedServicesValues passes the configured providers on as they are, with the installer's defaults.
func managedServicesValues(services []files.ManagedServiceConfig) []files.ManagedServiceConfig {
	if services == nil {
		return nil
	}

	values := make([]files.ManagedServiceConfig, 0, len(services))
	for _, service := range services {
		if service.Scope == "" {
			service.Scope = defaultProviderScope
		}

		// Only landscape providers have versions in the installer's schema; for REST providers, which
		// a provider without a landscape backend is, it drops them.
		if _, landscape := service.Backend["landscape"]; !landscape {
			service.Versions = nil
		}

		values = append(values, service)
	}

	return values
}
