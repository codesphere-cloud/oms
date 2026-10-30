// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package bom

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// IndexFilename is the BOM index filename within an installer package's dependencies.
const IndexFilename = "bom-index.json"

// IndexV3 is a v3 BOM index containing v2 component manifests.
type IndexV3 struct {
	Version    string        `json:"version"`
	Components []ComponentV2 `json:"components"`
}

// ComponentV2 is a v2 component manifest embedded in a BOM index.
type ComponentV2 struct {
	Version         string                      `json:"version"`
	Name            string                      `json:"name"`
	ContainerImages map[string]ContainerImageV2 `json:"containerImages,omitempty"`
	HelmCharts      map[string]HelmChartV2      `json:"helmCharts,omitempty"`
	Files           map[string]FileRef          `json:"files,omitempty"`
}

// ContainerImageV2 describes a container image and its discovery metadata.
type ContainerImageV2 struct {
	Ref     string   `json:"ref"`
	Digest  string   `json:"digest,omitempty"`
	Sources []string `json:"sources,omitempty"`
}

// HelmChartV2 describes an OCI Helm chart referenced by a v2 component.
type HelmChartV2 struct {
	Ref string `json:"ref"`
}

// GetCodesphereContainerImages returns named image refs for workspace image builds.
// Keep the declared tag: workspace builds use it to name the derived image.
func (b *IndexV3) GetCodesphereContainerImages() (map[string]string, error) {
	for _, component := range b.Components {
		if component.Name != "codesphere" {
			continue
		}

		images := make(map[string]string, len(component.ContainerImages))
		for name, image := range component.ContainerImages {
			images[name] = image.Ref
		}

		return images, nil
	}

	return nil, fmt.Errorf("codesphere component not found in BOM index")
}

// ParseIndex parses a BOM index, accepting only v3 indexes with v2 components.
func ParseIndex(data []byte) (*IndexV3, error) {
	var index IndexV3
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to parse JSON BOM index: %w", err)
	}

	if index.Version != "v3" {
		return nil, fmt.Errorf("unsupported BOM index version %q: expected v3", index.Version)
	}

	for _, component := range index.Components {
		if component.Version != "2" {
			return nil, fmt.Errorf("unsupported BOM component %q version %q: expected 2", component.Name, component.Version)
		}
	}

	return &index, nil
}

// GetContainerImages returns sorted, unique image references from the index,
// pinning each reference to its declared digest unless it already includes one.
func (b *IndexV3) GetContainerImages() []string {
	images := map[string]struct{}{}

	for _, component := range b.Components {
		for _, image := range component.ContainerImages {
			if image.Ref == "" {
				continue
			}

			ref := image.Ref
			if image.Digest != "" && !strings.Contains(ref, "@") {
				ref += "@" + image.Digest
			}

			images[ref] = struct{}{}
		}
	}

	return slices.Sorted(maps.Keys(images))
}

// GetChartRefs returns sorted, unique OCI chart references from both chart sources.
func (b *IndexV3) GetChartRefs() []string {
	charts := map[string]struct{}{}

	// Normalize refs with and without oci:// so older and newer index layouts deduplicate together.
	for _, component := range b.Components {
		for _, chart := range component.HelmCharts {
			if chart.Ref != "" {
				charts[strings.TrimPrefix(chart.Ref, "oci://")] = struct{}{}
			}
		}

		for _, file := range component.Files {
			if file.OciRef != "" {
				charts[strings.TrimPrefix(file.OciRef, "oci://")] = struct{}{}
			}
		}
	}

	return slices.Sorted(maps.Keys(charts))
}
