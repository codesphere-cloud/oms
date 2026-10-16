// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codespherevalues

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// bomImages are the codesphere component's images from the BOM, by name.
type bomImages map[string]string

// newBOMImages reads the codesphere component's images, moved onto the configured registry when
// the config asks for it.
func newBOMImages(b *bom.Config, registry *files.RegistryConfig) (bomImages, error) {
	images, err := b.GetCodesphereContainerImages()
	if err != nil {
		return nil, fmt.Errorf("failed to read the codesphere images from the BOM: %w", err)
	}

	moveToRegistry := registry != nil && registry.Server != "" && registry.ReplaceImagesInBom

	resolved := make(bomImages, len(images))
	for name, image := range images {
		if !moveToRegistry {
			resolved[name] = image
			continue
		}

		ref, err := parseImageRef(image)
		if err != nil {
			return nil, fmt.Errorf("failed to parse BOM image %s: %w", name, err)
		}

		resolved[name] = ref.withRegistry(registry.Server).String()
	}

	return resolved, nil
}

func (b bomImages) image(name string) (string, error) {
	image, ok := b[name]
	if !ok || image == "" {
		return "", fmt.Errorf("image %s not found in BOM", name)
	}

	return image, nil
}

func (b bomImages) ref(name string) (imageRef, error) {
	image, err := b.image(name)
	if err != nil {
		return imageRef{}, err
	}

	return parseImageRef(image)
}

// workspaceImage resolves a configured workspace image to an image name without tag.
func (b bomImages) workspaceImage(configured files.ImageRef) (string, error) {
	if configured.ImageName != "" {
		return configured.ImageName, nil
	}

	ref, err := b.ref(configured.BomRef)
	if err != nil {
		return "", err
	}

	return ref.image, nil
}
