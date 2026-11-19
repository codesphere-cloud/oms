// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"fmt"

	"github.com/distribution/reference"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// registryPullSecret is the Secret the installer creates for the registry the images are served from.
const registryPullSecret = "docker-regcred"

// bomImage is a container image of the package BOM, split the way the charts take it.
type bomImage struct {
	// Registry is the host, Repository the path below it, Name both.
	Registry   string
	Repository string
	Name       string
	Tag        string
}

// bomImageOf returns the image the package BOM names for a component, placed on the configured
// registry exactly where `oms copy package` puts it. It reports false when the BOM has no such
// image, so a package that predates the component leaves the chart defaults alone.
func bomImageOf(packageBom *bom.Config, config *files.RootConfig, component, key string) (bomImage, bool, error) {
	if packageBom == nil {
		return bomImage{}, false, nil
	}

	source := packageBom.Components[component].ContainerImages[key]
	if source == "" {
		return bomImage{}, false, nil
	}

	named, err := parseTagged(source, component, key)
	if err != nil {
		return bomImage{}, false, err
	}

	if registry := replacementRegistry(config); registry != "" {
		mapped, err := installer.PackageImageDestination(source, registry)
		if err != nil {
			return bomImage{}, false, fmt.Errorf("mapping BOM image %s.%s: %w", component, key, err)
		}

		if named, err = parseTagged(mapped, component, key); err != nil {
			return bomImage{}, false, err
		}
	}

	return bomImage{
		Registry:   reference.Domain(named),
		Repository: reference.Path(named),
		Name:       named.Name(),
		Tag:        named.Tag(),
	}, true, nil
}

// parseTagged parses an image reference that must carry a tag. The check precedes the mapping onto
// a registry, which would turn a missing tag into latest.
func parseTagged(source, component, key string) (reference.NamedTagged, error) {
	named, err := reference.ParseNormalizedNamed(source)
	if err != nil {
		return nil, fmt.Errorf("parsing BOM image %s.%s: %w", component, key, err)
	}

	tagged, ok := named.(reference.NamedTagged)
	if !ok {
		return nil, fmt.Errorf("BOM image %s.%s has no tag: %s", component, key, source)
	}

	return tagged, nil
}

// replacementRegistry is the registry the installation's images are rewritten onto, empty when the
// config does not replace the images of the BOM.
func replacementRegistry(config *files.RootConfig) string {
	if config.Registry == nil || !config.Registry.ReplaceImagesInBom {
		return ""
	}

	return config.Registry.Server
}

// imagePullSecrets names the registry Secret exactly when the config has a registry, whether or not
// it replaces the images of the BOM.
func imagePullSecrets(config *files.RootConfig) []any {
	if config.Registry == nil || config.Registry.Server == "" {
		return []any{}
	}

	return []any{map[string]any{"name": registryPullSecret}}
}
