// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package imageref splits and mirrors container image references as the old pc-installer does. It
// works on plain strings, because the Docker reference parsers normalize references the old
// pc-installer keeps as written.
package imageref

import "strings"

// registrySegmentReplacer flattens a registry into a single path segment.
var registrySegmentReplacer = strings.NewReplacer(".", "_", ":", "_")

// Split separates a reference into its repository, tag and digest. A reference can carry both a
// tag and a digest, which the old pc-installer keeps and the reference parsers do not.
func Split(reference string) (repository, tag, digest string) {
	repository = reference
	if at := strings.Index(repository, "@"); at != -1 {
		digest = repository[at+1:]
		repository = repository[:at]
	}

	if colon := strings.LastIndex(repository, ":"); colon > strings.LastIndex(repository, "/") {
		tag = repository[colon+1:]
		repository = repository[:colon]
	}

	return repository, tag, digest
}

// MirrorRepository returns the repository path a reference keeps below a mirror registry. The old
// pc-installer moves the source registry into the first path segment with its dots and colons
// replaced by underscores, so that ghcr.io/codesphere-cloud/api ends up as
// <server>/ghcr_io/codesphere-cloud/api. Copying images and the values pointing at them must name
// them identically. The leading segment of a reference without a registry counts as one, keeping
// alpine/kubectl below alpine instead of below docker.io.
func MirrorRepository(repository string) string {
	registry, path, found := strings.Cut(repository, "/")

	registry = registrySegmentReplacer.Replace(registry)
	if !found {
		return registry
	}

	return registry + "/" + path
}
