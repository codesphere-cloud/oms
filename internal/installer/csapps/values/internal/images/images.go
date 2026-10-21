// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package images resolves the codesphere component's images from the BOM.
package images

import (
	"fmt"
	"path"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// Images are the codesphere component's images from the BOM, by name.
type Images map[string]string

// New reads the codesphere component's images, moved onto the configured registry when
// the config asks for it.
func New(b *bom.Config, registry *files.RegistryConfig) (Images, error) {
	images, err := b.GetCodesphereContainerImages()
	if err != nil {
		return nil, fmt.Errorf("failed to read the codesphere images from the BOM: %w", err)
	}

	moveToRegistry := registry != nil && registry.Server != "" && registry.ReplaceImagesInBom

	resolved := make(Images, len(images))
	for name, image := range images {
		if !moveToRegistry {
			resolved[name] = image
			continue
		}

		ref, err := parseRef(image)
		if err != nil {
			return nil, fmt.Errorf("failed to parse BOM image %s: %w", name, err)
		}

		resolved[name] = ref.withRegistry(registry.Server).String()
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

// WorkspaceImage resolves a configured workspace image to an image name without tag.
func (b Images) WorkspaceImage(configured files.ImageRef) (string, error) {
	if configured.ImageName != "" {
		return configured.ImageName, nil
	}

	ref, err := b.Ref(configured.BomRef)
	if err != nil {
		return "", err
	}

	return ref.Image, nil
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
	parsed := Ref{}

	if name, digest, ok := strings.Cut(ref, "@"); ok {
		ref = name
		parsed.Digest = digest
	}

	// A registry port also contains a colon: with two or three parts, the last one is the tag.
	parts := strings.Split(ref, ":")
	if len(parts) == 2 || len(parts) == 3 {
		parsed.Tag = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}

	parsed.Image = strings.Join(parts, ":")

	if parsed.Tag == "" && parsed.Digest == "" {
		return Ref{}, fmt.Errorf("image %s is missing a tag or digest", ref)
	}

	return parsed, nil
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

// withRegistry moves the image onto registryServer and keeps its original registry in the path, so
// images from different registries cannot clash.
func (i Ref) withRegistry(registryServer string) Ref {
	sourceHost, sourcePath, _ := strings.Cut(i.Image, "/")
	escapedHost := strings.NewReplacer(".", "_", ":", "_").Replace(sourceHost)
	targetHost, targetPath, _ := strings.Cut(registryServer, "/")

	moved := i
	targetPath = path.Join("/", targetPath, escapedHost, sourcePath)
	moved.Image = strings.TrimSuffix(targetHost+targetPath, "/")

	return moved
}

const authServiceImage = "auth-service"

// ReleaseTag is the tag of all Codesphere service images in the BOM. The chart pins every
// service to it; like the installer, it is read from the auth-service image.
func (b Images) ReleaseTag() (string, error) {
	authService, err := b.Ref(authServiceImage)
	if err != nil {
		return "", err
	}

	if authService.Tag == "" {
		return "", fmt.Errorf("image %s has no tag to deploy Codesphere with", authServiceImage)
	}

	return authService.Tag, nil
}
