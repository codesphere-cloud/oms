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

// Images are the codesphere component's container images from the BOM. The key is the
// image's name in the BOM, the value its full reference:
//
//	"auth-service": "ghcr.io/codesphere-cloud/codesphere-monorepo/auth-service:v1.111.0"
//
// Reference, Name, Tag, NameAndTag and Image return the reference or one of its parts, named as OCI and
// distribution/reference name them:
//
//	                       6
//	┌──────────────────────┴──────────────────────┐
//	ghcr.io/codesphere-cloud/api:1.2.3@sha256:0123…
//	└──┬──┘ └────────┬─────────┘ └─┬─┘ └────┬─────┘
//	   1             2             3        4
//	└────────────┬─────────────┘
//	             5
//
//	   OCI         distribution/reference   Images
//	1  registry    reference.Domain
//	2  repository  reference.Path
//	3  tag         Tagged.Tag               Tag
//	4  digest      Digested.Digest
//	5              Named.Name               Name
//	6  reference   Reference.String         Reference
type Images map[string]string

// Image is an image reference split into name and tag, as the charts take it.
type Image struct {
	Name string `yaml:"name"`
	Tag  string `yaml:"tag"`
}

// New reads the codesphere component's image references.
func New(b *bom.Config) (Images, error) {
	images, err := b.GetCodesphereContainerImages()
	if err != nil {
		return nil, fmt.Errorf("failed to read the codesphere images from the BOM: %w", err)
	}

	return images, nil
}

// Has reports whether the BOM has the named image.
func (images Images) Has(name string) bool {
	return images[name] != ""
}

// Reference returns the named image's reference as the BOM has it.
func (images Images) Reference(name string) (string, error) {
	named, err := images.parse(name)
	if err != nil {
		return "", err
	}

	return named.String(), nil
}

// Name returns the registry and repository of the named image, without tag and digest.
func (images Images) Name(name string) (string, error) {
	named, err := images.parse(name)
	if err != nil {
		return "", err
	}

	return named.Name(), nil
}

// Tag returns the tag of the named image.
func (images Images) Tag(name string) (string, error) {
	named, err := images.parse(name)
	if err != nil {
		return "", err
	}

	return tag(named)
}

// NameAndTag returns the name and tag of the named image.
func (images Images) NameAndTag(name string) (string, string, error) {
	named, err := images.parse(name)
	if err != nil {
		return "", "", err
	}

	imageTag, err := tag(named)
	if err != nil {
		return "", "", err
	}

	return named.Name(), imageTag, nil
}

// Image returns the named image split into name and tag.
func (images Images) Image(name string) (Image, error) {
	imageName, imageTag, err := images.NameAndTag(name)
	if err != nil {
		return Image{}, err
	}

	return Image{Name: imageName, Tag: imageTag}, nil
}

func tag(named reference.Named) (string, error) {
	tagged, ok := named.(reference.Tagged)
	if !ok {
		return "", fmt.Errorf("image %s is missing a tag", named)
	}

	return tagged.Tag(), nil
}

func (images Images) parse(name string) (reference.Named, error) {
	image, ok := images[name]
	if !ok || image == "" {
		return nil, fmt.Errorf("image %s not found in BOM", name)
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
