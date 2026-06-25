// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"fmt"
	"log"
	"strings"

	"github.com/distribution/reference"
)

const CodesphereRegistry = "ghcr.io"

type Mirror struct {
	Copier ImageCopier
	DryRun bool
}

func (m *Mirror) MirrorImages(sourceRefs []string, targetRegistry string) (int, error) {
	if len(sourceRefs) == 0 {
		return 0, fmt.Errorf("no %s image references found in BOM", CodesphereRegistry)
	}
	if !m.DryRun && m.Copier == nil {
		return 0, fmt.Errorf("image copier is required")
	}

	for _, sourceRef := range sourceRefs {
		destinationRef, err := TargetImageRef(sourceRef, targetRegistry)
		if err != nil {
			return 0, err
		}

		log.Printf("Mirroring %s -> %s", sourceRef, destinationRef)
		if m.DryRun {
			continue
		}

		if err := m.Copier.Copy(sourceRef, destinationRef); err != nil {
			return 0, err
		}
	}

	log.Printf("Processed %d %s image references", len(sourceRefs), CodesphereRegistry)

	return len(sourceRefs), nil
}

func TargetImageRef(sourceRef string, targetRegistry string) (string, error) {
	if targetRegistry == "" {
		return "", fmt.Errorf("target registry must not be empty")
	}
	ref, err := reference.ParseNormalizedNamed(sourceRef)
	if err != nil {
		return "", fmt.Errorf("invalid source reference %s: %w", sourceRef, err)
	}
	if reference.Domain(ref) != CodesphereRegistry {
		return "", fmt.Errorf("source reference %s is not a Codesphere image reference", sourceRef)
	}
	return targetRegistry + "/" + strings.TrimPrefix(sourceRef, CodesphereRegistry+"/"), nil
}

// IsCodesphereImageReference returns true if the given value is a valid Codesphere image reference.
func IsCodesphereImageReference(value string) bool {
	if value == "" {
		return false
	}

	named, err := reference.ParseNormalizedNamed(value)
	if err != nil {
		return false
	}
	if reference.Domain(named) != CodesphereRegistry {
		return false
	}
	if !hasTagOrDigest(named) {
		return false
	}

	return true
}

func hasTagOrDigest(named reference.Named) bool {
	if _, ok := named.(reference.Tagged); ok {
		return true
	}
	if _, ok := named.(reference.Digested); ok {
		return true
	}
	return false
}
