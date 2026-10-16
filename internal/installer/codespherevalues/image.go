// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codespherevalues

import (
	"fmt"
	"path"
	"strings"
)

// imageRef is an image reference split like the Codesphere installer's OciImage, so the values name
// images exactly as it does.
type imageRef struct {
	// image is the reference without tag and digest, e.g. ghcr.io/org/app.
	image  string
	tag    string
	digest string
}

func parseImageRef(ref string) (imageRef, error) {
	parsed := imageRef{}

	if name, digest, ok := strings.Cut(ref, "@"); ok {
		ref = name
		parsed.digest = digest
	}

	// A registry port also contains a colon: with two or three parts, the last one is the tag.
	parts := strings.Split(ref, ":")
	if len(parts) == 2 || len(parts) == 3 {
		parsed.tag = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}

	parsed.image = strings.Join(parts, ":")

	if parsed.tag == "" && parsed.digest == "" {
		return imageRef{}, fmt.Errorf("image %s is missing a tag or digest", ref)
	}

	return parsed, nil
}

func (i imageRef) String() string {
	ref := i.image
	if i.tag != "" {
		ref += ":" + i.tag
	}

	if i.digest != "" {
		ref += "@" + i.digest
	}

	return ref
}

// withRegistry moves the image onto registryServer and keeps its original registry in the path, so
// images from different registries cannot clash.
func (i imageRef) withRegistry(registryServer string) imageRef {
	sourceHost, sourcePath, _ := strings.Cut(i.image, "/")
	escapedHost := strings.NewReplacer(".", "_", ":", "_").Replace(sourceHost)
	targetHost, targetPath, _ := strings.Cut(registryServer, "/")

	moved := i
	moved.image = strings.TrimSuffix(targetHost+path.Join("/", targetPath, escapedHost, sourcePath), "/")

	return moved
}
