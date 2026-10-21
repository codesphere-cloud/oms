// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package imageref splits container image references and names them on a mirror registry as the
// old pc-installer does. It works on plain strings, because the Docker reference parsers normalize
// references the old pc-installer keeps as written, e.g. alpine/kubectl to docker.io/alpine/kubectl.
//
// The parts of a reference:
//
//	ghcr.io/codesphere-cloud/api:1.2.3@sha256:abc
//	└────────── name ──────────┘ └tag┘ └─digest─┘
//	└─────┘ └─── repository ───┘
//	registry
package imageref

import (
	"path"
	"strings"
)

// registrySegmentReplacer flattens a registry into a single path segment.
var registrySegmentReplacer = strings.NewReplacer(".", "_", ":", "_")

// Split separates a reference into its name, tag and digest. A reference can carry both a tag and
// a digest, which the old pc-installer keeps and the reference parsers do not.
func Split(reference string) (name, tag, digest string) {
	name = reference
	if at := strings.Index(name, "@"); at != -1 {
		digest = name[at+1:]
		name = name[:at]
	}

	if colon := strings.LastIndex(name, ":"); colon > strings.LastIndex(name, "/") {
		tag = name[colon+1:]
		name = name[:colon]
	}

	return name, tag, digest
}

// MirroredName returns the name an image gets on mirrorRegistry. The source registry becomes the
// first path segment with its dots and colons replaced by underscores, so images from different
// registries cannot clash: ghcr.io/codesphere-cloud/api on registry.example.com:5000 becomes
// registry.example.com:5000/ghcr_io/codesphere-cloud/api. Copying images and the values pointing
// at them must name them identically. The first segment of a name without a registry counts as
// one, keeping alpine/kubectl below alpine instead of below docker.io.
func MirroredName(name, mirrorRegistry string) string {
	registry, repository, found := strings.Cut(name, "/")

	registry = registrySegmentReplacer.Replace(registry)
	if !found {
		return path.Join(mirrorRegistry, registry)
	}

	return path.Join(mirrorRegistry, registry, repository)
}
