// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer"
)

// DefaultRegistryURL is the registry base used by local bootstrap.
const DefaultRegistryURL = installer.DefaultRegistryURL

func (e *CodesphereEnvironment) registryURL() (string, error) {
	registry, err := installer.NormalizeRegistry(e.RegistryURL)
	if err != nil {
		return "", fmt.Errorf("invalid local image registry: %w", err)
	}

	return registry, nil
}

func (e *CodesphereEnvironment) chartsRegistryURL() (string, error) {
	base, err := e.registryURL()
	if err != nil {
		return "", err
	}

	chartsRegistry, err := installer.ChartRegistryURL(base)
	if err != nil {
		return "", fmt.Errorf("resolving chart registry URL: %w", err)
	}

	return chartsRegistry, nil
}
