// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"fmt"
	"log"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
)

// ImageCopier copies images from one registry reference to another.
type ImageCopier interface {
	CopyAll(sourceRef string, destinationRef string) error
}

type Mirror struct {
	Copier ImageCopier
	DryRun bool
}

func (m *Mirror) MirrorGHCRImages(config *bom.Config, targetRegistry string) (int, error) {
	if config == nil {
		return 0, fmt.Errorf("bom config must not be nil")
	}

	return m.MirrorImageReferences(config.GHCRImageReferences(), targetRegistry)
}

func (m *Mirror) MirrorImageReferences(sourceRefs []string, targetRegistry string) (int, error) {
	if len(sourceRefs) == 0 {
		return 0, fmt.Errorf("no %s image references found in BOM", bom.GHCRRegistry)
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

		if err := m.Copier.CopyAll(sourceRef, destinationRef); err != nil {
			return 0, err
		}
	}

	log.Printf("Processed %d %s image references", len(sourceRefs), bom.GHCRRegistry)

	return len(sourceRefs), nil
}

func TargetImageRef(sourceRef string, targetRegistry string) (string, error) {
	normalizedSourceRef, ok := bom.NormalizeImageReferenceForRegistry(sourceRef, bom.GHCRRegistry)
	if !ok {
		return "", fmt.Errorf("source reference must point to %s and include a tag or digest: %s", bom.GHCRRegistry, sourceRef)
	}

	normalizedTargetRegistry := normalizeTargetRegistry(targetRegistry)
	if normalizedTargetRegistry == "" {
		return "", fmt.Errorf("target registry must not be empty")
	}
	if strings.Contains(normalizedTargetRegistry, "://") {
		return "", fmt.Errorf("target registry must not include an unsupported scheme: %s", targetRegistry)
	}

	return normalizedTargetRegistry + "/" + strings.TrimPrefix(normalizedSourceRef, bom.GHCRRegistry+"/"), nil
}

func normalizeTargetRegistry(targetRegistry string) string {
	normalized := strings.TrimSpace(targetRegistry)
	normalized = strings.TrimPrefix(normalized, "docker://")
	normalized = strings.TrimPrefix(normalized, "https://")
	normalized = strings.TrimPrefix(normalized, "http://")
	normalized = strings.TrimSuffix(normalized, "/")

	return normalized
}
