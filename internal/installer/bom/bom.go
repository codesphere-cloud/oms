// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package bom

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/distribution/reference"
)

const GHCRRegistry = "ghcr.io"

// Config represents the Bill of Materials configuration.
type Config struct {
	Components map[string]ComponentConfig `json:"components"`
	Migrations MigrationsConfig           `json:"migrations"`
}

// ComponentConfig represents a component in the BOM.
type ComponentConfig struct {
	ContainerImages map[string]string  `json:"containerImages,omitempty"`
	Files           map[string]FileRef `json:"files,omitempty"`
}

// FileRef represents a file reference in the BOM.
type FileRef struct {
	SrcPath    string   `json:"srcPath,omitempty"`
	SrcUrl     string   `json:"srcUrl,omitempty"`
	Executable bool     `json:"executable,omitempty"`
	Glob       *GlobRef `json:"glob,omitempty"`
	// OciRef is an OCI image reference for a Helm chart, e.g. ghcr.io/org/charts/my-chart:1.0.0
	OciRef string `json:"ociRef,omitempty"`
}

// GlobRef represents a glob-based file reference.
type GlobRef struct {
	Cwd     string   `json:"cwd"`
	Include string   `json:"include"`
	Exclude []string `json:"exclude,omitempty"`
}

// MigrationsConfig represents the migrations configuration.
type MigrationsConfig struct {
	Db DbMigrationConfig `json:"db"`
}

// DbMigrationConfig represents database migration configuration.
type DbMigrationConfig struct {
	Path string `json:"path"`
	From string `json:"from"`
}

// Parse reads and parses a BOM JSON file.
func Parse(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read BOM file: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse JSON BOM: %w", err)
	}
	return &cfg, nil
}

// GetPCApps returns the pc-applications chart version from the BOM by
// parsing the tag out of the OCI image reference stored at
// components["pc-applications"].files["chart"].ociRef.
// Returns (nil, false) when the component is absent, the chart file entry is
// missing, or the ociRef has no recognisable tag.
func (b *Config) GetPCApps() (reference.Tagged, bool) {
	comp, ok := b.Components["pc-applications"]
	if !ok {
		return nil, false
	}
	chart, ok := comp.Files["chart"]
	if !ok || chart.OciRef == "" {
		return nil, false
	}
	ref, err := reference.ParseNormalizedNamed(chart.OciRef)
	if err != nil {
		return nil, false
	}
	tagged, ok := ref.(reference.Tagged)
	if !ok {
		return nil, false
	}
	return tagged, true
}

// GetCodesphereContainerImages returns all container images from the codesphere component.
func (b *Config) GetCodesphereContainerImages() (map[string]string, error) {
	if b.Components == nil {
		return nil, fmt.Errorf("codesphere component not found in BOM")
	}
	comp, exists := b.Components["codesphere"]
	if !exists {
		return nil, fmt.Errorf("codesphere component not found in BOM")
	}
	return comp.ContainerImages, nil
}

// GHCRImageReferences returns unique GHCR image and OCI chart references from the BOM.
func (b *Config) GHCRImageReferences() []string {
	return b.ImageReferencesForRegistry(GHCRRegistry)
}

// ImageReferencesForRegistry returns unique image and OCI chart references for a registry.
func (b *Config) ImageReferencesForRegistry(registry string) []string {
	if b == nil {
		return nil
	}

	refsSet := map[string]struct{}{}
	for _, component := range b.Components {
		for _, imageRef := range component.ContainerImages {
			addRegistryRef(refsSet, imageRef, registry)
		}

		for _, fileRef := range component.Files {
			addRegistryRef(refsSet, fileRef.OciRef, registry)
		}
	}

	refs := make([]string, 0, len(refsSet))
	for ref := range refsSet {
		refs = append(refs, ref)
	}
	sort.Strings(refs)

	return refs
}

func addRegistryRef(refsSet map[string]struct{}, value string, registry string) {
	ref, ok := NormalizeImageReferenceForRegistry(value, registry)
	if ok {
		refsSet[ref] = struct{}{}
	}
}

// NormalizeImageReferenceForRegistry normalizes a docker image reference and verifies its registry.
func NormalizeImageReferenceForRegistry(value string, registry string) (string, bool) {
	ref := strings.TrimSpace(value)
	ref = strings.TrimPrefix(ref, "docker://")

	if ref == "" || registry == "" {
		return "", false
	}

	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return "", false
	}
	if reference.Domain(named) != registry {
		return "", false
	}
	if !hasTagOrDigest(named) {
		return "", false
	}

	return reference.FamiliarString(named), true
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
