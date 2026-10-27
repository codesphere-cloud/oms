// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"net/url"
	"strings"
)

// DefaultRegistryURL is the public registry base for Codesphere artifacts.
const DefaultRegistryURL = "ghcr.io"

// NormalizeRegistry resolves the default and validates a scheme-free registry base.
// Registry bases may include a port and repository path.
func NormalizeRegistry(base string) (string, error) {
	if base == "" {
		return DefaultRegistryURL, nil
	}

	if strings.Contains(base, "://") || strings.HasPrefix(base, "//") {
		return "", fmt.Errorf("registry URL must not include a scheme or leading //")
	}

	registry, err := url.Parse("//" + base)
	if err != nil {
		return "", fmt.Errorf("parse chart registry URL: %w", err)
	}

	if registry.Host == "" || registry.User != nil || registry.RawQuery != "" || registry.Fragment != "" {
		return "", fmt.Errorf("registry must contain a host and optional port and repository path")
	}

	return strings.TrimRight(strings.TrimPrefix(registry.JoinPath().String(), "//"), "/"), nil
}

// ChartsRegistryURL appends the Codesphere chart repository to a registry base.
func ChartsRegistryURL(base string) (string, error) {
	registry, err := NormalizeRegistry(base)
	if err != nil {
		return "", err
	}

	return registry + ChartsRepositoryPath, nil
}
