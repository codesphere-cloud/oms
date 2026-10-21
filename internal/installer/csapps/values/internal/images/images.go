// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package images resolves the codesphere component's images from the BOM.
package images

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/imageref"
)

// Images are the codesphere component's images from the BOM, by name.
type Images map[string]string

// New reads the codesphere component's image references, rewritten to point at the configured
// registry when the config asks for it.
func New(b *bom.Config, registry *files.RegistryConfig) (Images, error) {
	images, err := b.GetCodesphereContainerImages()
	if err != nil {
		return nil, fmt.Errorf("failed to read the codesphere images from the BOM: %w", err)
	}

	pointAtRegistry := registry != nil && registry.Server != "" && registry.ReplaceImagesInBom

	resolved := make(Images, len(images))
	for name, image := range images {
		if pointAtRegistry {
			imageName, tag, digest := imageref.Split(image)
			mirrored := Ref{Image: imageref.MirroredName(imageName, registry.Server), Tag: tag, Digest: digest}
			image = mirrored.String()
		}

		resolved[name] = image
	}

	return resolved, nil
}

// Image returns the full reference of the named image.
func (b Images) Image(name string) (string, error) {
	image, ok := b[name]
	if !ok || image == "" {
		return "", fmt.Errorf("image %s not found in BOM", name)
	}

	return image, nil
}

// Ref returns the named image split into image, tag and digest.
func (b Images) Ref(name string) (Ref, error) {
	image, err := b.Image(name)
	if err != nil {
		return Ref{}, err
	}

	return parseRef(image)
}

// Ref is an image reference split like the Codesphere installer's OciImage, so the values name
// images exactly as it does.
type Ref struct {
	// Image is the reference without tag and digest, e.g. ghcr.io/org/app.
	Image  string
	Tag    string
	Digest string
}

func parseRef(ref string) (Ref, error) {
	image, tag, digest := imageref.Split(ref)
	if tag == "" && digest == "" {
		return Ref{}, fmt.Errorf("image %s is missing a tag or digest", ref)
	}

	return Ref{Image: image, Tag: tag, Digest: digest}, nil
}

func (i Ref) String() string {
	ref := i.Image
	if i.Tag != "" {
		ref += ":" + i.Tag
	}

	if i.Digest != "" {
		ref += "@" + i.Digest
	}

	return ref
}
