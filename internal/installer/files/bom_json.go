// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files

import (
	"encoding/json"
	"fmt"
	"os"
)

// BomConfig represents the Bill of Materials configuration
type BomConfig struct {
	Components map[string]ComponentConfig `json:"components"`
	Migrations MigrationsConfig           `json:"migrations"`
}

// ComponentConfig represents a component in the BOM
type ComponentConfig struct {
	ContainerImages map[string]string  `json:"containerImages,omitempty"`
	Files           map[string]FileRef `json:"files,omitempty"`
}

// FileRef represents a file reference in the BOM
type FileRef struct {
	SrcPath    string   `json:"srcPath,omitempty"`
	SrcUrl     string   `json:"srcUrl,omitempty"`
	Executable bool     `json:"executable,omitempty"`
	Glob       *GlobRef `json:"glob,omitempty"`
}

// GlobRef represents a glob-based file reference
type GlobRef struct {
	Cwd     string   `json:"cwd"`
	Include string   `json:"include"`
	Exclude []string `json:"exclude,omitempty"`
}

// MigrationsConfig represents the migrations configuration
type MigrationsConfig struct {
	Db DbMigrationConfig `json:"db"`
}

// DbMigrationConfig represents database migration configuration
type DbMigrationConfig struct {
	Path string `json:"path"`
	From string `json:"from"`
}

// CodesphereComponent represents the codesphere-specific component
type CodesphereComponent struct {
	ContainerImages map[string]string  `json:"containerImages"`
	Files           map[string]FileRef `json:"files"`
}

// ParseBomConfig reads and parses a BOM JSON file
func (b *BomConfig) ParseBomConfig(filePath string) error {
	bomData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read BOM file: %w", err)
	}

	err = json.Unmarshal(bomData, b)
	if err != nil {
		return fmt.Errorf("failed to parse JSON BOM: %w", err)
	}

	return nil
}

// GetCodesphereContainerImages returns all container images from the codesphere component
func (b *BomConfig) GetCodesphereContainerImages() (map[string]string, error) {
	if b.Components == nil {
		return nil, fmt.Errorf("codesphere component not found in BOM")
	}

	codesphereComp, exists := b.Components["codesphere"]
	if !exists {
		return nil, fmt.Errorf("codesphere component not found in BOM")
	}

	return codesphereComp.ContainerImages, nil
}
