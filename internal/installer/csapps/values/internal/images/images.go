// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package images resolves the codesphere component's images from the BOM.
package images

import (
	"fmt"
	"strings"

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
	image, tag, digest := splitRef(ref)
	if tag == "" && digest == "" {
		return Ref{}, fmt.Errorf("image %s is missing a tag or digest", ref)
	}

	return Ref{Image: image, Tag: tag, Digest: digest}, nil
}

// splitRef keeps both a tag and a digest when a reference carries both, which the Docker reference
// parsers do not.
func splitRef(ref string) (image, tag, digest string) {
	image = ref
	if at := strings.Index(image, "@"); at != -1 {
		digest = image[at+1:]
		image = image[:at]
	}

	if colon := strings.LastIndex(image, ":"); colon > strings.LastIndex(image, "/") {
		tag = image[colon+1:]
		image = image[:colon]
	}

	return image, tag, digest
}
