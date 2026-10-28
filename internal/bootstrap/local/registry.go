// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"fmt"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
)

// DefaultRegistryURL is the registry base used by local bootstrap.
const DefaultRegistryURL = "ghcr.io"

func (e *CodesphereEnvironment) registryURL() (string, error) {
	if e.RegistryURL == "" {
		return DefaultRegistryURL, nil
	}

	if strings.Contains(e.RegistryURL, "://") || strings.HasPrefix(e.RegistryURL, "//") {
		return "", fmt.Errorf("registry URL must not include a scheme or leading //")
	}

	return strings.TrimRight(e.RegistryURL, "/"), nil
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
