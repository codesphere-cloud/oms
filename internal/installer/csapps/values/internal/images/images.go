// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package images resolves the codesphere component's images from the BOM.
package images

import (
	// Registers SHA-256 so reference.Parse accepts sha256 digests.
	_ "crypto/sha256"
	"fmt"

	"github.com/distribution/reference"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
)

// Images are the codesphere component's images from the BOM, by name.
type Images map[string]string

// New reads the codesphere component's image references.
func New(b *bom.Config) (Images, error) {
	images, err := b.GetCodesphereContainerImages()
	if err != nil {
		return nil, fmt.Errorf("failed to read the codesphere images from the BOM: %w", err)
	}

	return images, nil
}

// Image returns the full reference of the named image.
func (b Images) Image(name string) (string, error) {
	image, ok := b[name]
	if !ok || image == "" {
		return "", fmt.Errorf("image %s not found in BOM", name)
	}

	return image, nil
}

// Named returns the named image parsed, with a tag or a digest. The parts of a reference:
//
//	     name (repository)
//	┌────────────┴─────────────┐
//	ghcr.io/codesphere-cloud/api:1.2.3@sha256:0123…
//	└──┬──┘ └────────┬─────────┘ └─┬─┘ └────┬─────┘
//	domain         path           tag    digest
func (b Images) Named(name string) (reference.Named, error) {
	image, err := b.Image(name)
	if err != nil {
		return nil, err
	}

	parsed, err := reference.Parse(image)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image %s: %w", image, err)
	}

	named, ok := parsed.(reference.Named)
	if !ok {
		return nil, fmt.Errorf("image %s has no name", image)
	}

	_, tagged := named.(reference.Tagged)

	_, digested := named.(reference.Digested)
	if !tagged && !digested {
		return nil, fmt.Errorf("image %s is missing a tag or digest", image)
	}

	return named, nil
}

// Tagged returns the named image parsed, for values that need its tag.
func (b Images) Tagged(name string) (reference.NamedTagged, error) {
	named, err := b.Named(name)
	if err != nil {
		return nil, err
	}

	tagged, ok := named.(reference.NamedTagged)
	if !ok {
		return nil, fmt.Errorf("image %s is missing a tag", named)
	}

	return tagged, nil
}
