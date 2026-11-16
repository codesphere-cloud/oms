// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// FalcoPcAppsValues points the falco-deployment application at the configured registry.
// The falco-operator pulls the plugin OCI artifacts itself, so the image registry policy does
// not rewrite them and the chart's registry.name must name the mirror instead.
// The operator matches pull credentials against registry.name, so it only receives the host
// and optional port. Registries with a repository path prefix also need the plugin repositories
// overridden through the pcApps block or --pc-apps-values.
// They are the base of the pc-apps values: an explicit `pcApps` block in config.yaml and any
// --pc-apps-values file still override them.
// Returns nil when no registry server is set, leaving the chart defaults untouched.
func FalcoPcAppsValues(config *files.RootConfig) (files.ChartValues, error) {
	if config.Registry == nil || config.Registry.Server == "" {
		return nil, nil
	}

	registry, err := installer.NormalizeRegistry(config.Registry.Server)
	if err != nil {
		return nil, fmt.Errorf("invalid registry server for falco-deployment: %w", err)
	}

	return files.ChartValues{
		"applications": files.ChartValues{
			"falco-deployment": files.ChartValues{
				"valuesObject": files.ChartValues{
					"registry": files.ChartValues{
						"name": installer.RegistryHost(registry),
					},
				},
			},
		},
	}, nil
}
