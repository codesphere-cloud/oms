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

// mirrorsImages reports whether the bootstrap pulls from a registry other than ghcr.io, such as
// a mirror in an air-gapped installation. Images are then resolved onto that registry and pulled
// with its credentials.
func (e *CodesphereEnvironment) mirrorsImages() (bool, error) {
	registry, err := e.registryURL()
	if err != nil {
		return false, err
	}

	return registry != DefaultRegistryURL, nil
}

// resolveImage maps an upstream image reference onto the configured registry. A mirror stores
// images in the layout 'oms copy package' writes, so the same mapping finds them there. Without
// a mirror the reference is returned unchanged.
func (e *CodesphereEnvironment) resolveImage(ref string) (string, error) {
	registry, err := e.registryURL()
	if err != nil {
		return "", err
	}

	if registry == DefaultRegistryURL {
		return ref, nil
	}

	image, err := installer.PackageImageDestination(ref, registry)
	if err != nil {
		return "", fmt.Errorf("failed to resolve image %q on registry %q: %w", ref, registry, err)
	}

	return image, nil
}
