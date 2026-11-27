// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// applicationValues returns the pc-applications values of one chart of the PC installer:
// `enabled: false` when the run does not install it, otherwise its values under `valuesObject`
// with `enabled` left to the pc-applications default. A chart that is installed and has no values
// gets no entry.
func applicationValues(name string, installed bool, values files.ChartValues) files.ChartValues {
	switch {
	case !installed:
		return files.ChartValues{"applications": files.ChartValues{name: files.ChartValues{"enabled": false}}}
	case len(values) > 0:
		return files.ChartValues{"applications": files.ChartValues{name: files.ChartValues{"valuesObject": values}}}
	default:
		return files.ChartValues{"applications": files.ChartValues{}}
	}
}

// imagePullSecrets names the registry pull secret when the config has a registry, and no secret
// otherwise, as nothing creates it then.
func imagePullSecrets(config *files.RootConfig) []any {
	if config.Registry == nil || config.Registry.Server == "" {
		return []any{}
	}

	return []any{map[string]any{"name": installer.RegistryPullSecretName}}
}
