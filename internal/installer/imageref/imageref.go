// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package imageref splits image references and names them on a mirror registry.
package imageref

import (
	"path"
	"strings"
)

// registrySegmentReplacer flattens a registry into a single path segment.
var registrySegmentReplacer = strings.NewReplacer(".", "_", ":", "_")

// Split separates a reference into its name, tag and digest:
//
//	ghcr.io/codesphere-cloud/api:1.2.3@sha256:abc
//	\_____/ \__________________/ \___/ \________/
//	registry     repository       tag    digest
//	\__________________________/
//	            name
//
// A reference can carry both a tag and a digest, which the old pc-installer keeps and the Docker
// reference parsers do not.
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

// MirroredName returns the name an image gets on mirrorRegistry: ghcr.io/codesphere-cloud/api on
// registry.example.com:5000 becomes registry.example.com:5000/ghcr_io/codesphere-cloud/api. The
// source registry stays as a path segment, so images from different registries cannot clash.
// Copying images and the values pointing at them must name them identically. A name without a
// registry keeps its path: alpine/kubectl becomes <mirror>/alpine/kubectl, not
// <mirror>/docker_io/alpine/kubectl as Docker's implicit docker.io registry would give.
func MirroredName(name, mirrorRegistry string) string {
	registry, repository, found := strings.Cut(name, "/")

	registry = registrySegmentReplacer.Replace(registry)
	if !found {
		return path.Join(mirrorRegistry, registry)
	}

	return path.Join(mirrorRegistry, registry, repository)
}
