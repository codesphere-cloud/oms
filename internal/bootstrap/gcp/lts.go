// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// LTSSpec describes the compatibility requirements for a specific LTS release.
// To add support for a new LTS version, add a new entry to ltsRegistry — no other
// files need to change.
type LTSSpec struct {
	// InstallVersion is the exact install version string that identifies this LTS release.
	InstallVersion string
	// UnsupportedExperiments lists experiments that did not exist at this LTS release
	// and must be stripped from the config before passing it to the LTS installer.
	UnsupportedExperiments []string
	// ClearManagedServices instructs the compat layer to set ManagedServices to nil.
	// Required when the LTS schema expects full provider definitions, not the abbreviated
	// form stored in config.yaml.
	ClearManagedServices bool
	// RequiresJumpboxFiles instructs the bootstrap to generate a separate config-jumpbox.yaml
	// and codesphere.yaml (with compat applied) instead of using the standard config.yaml.
	RequiresJumpboxFiles bool
	// RequiresOmsBinaryUpdate instructs the bootstrap to build a fresh linux/amd64 OMS binary
	// and deploy it to the jumpbox before running the installer.
	RequiresOmsBinaryUpdate bool
}

// ltsRegistry is the single source of truth for all known LTS versions and their quirks.
// Add a new LTSSpec entry here to support an additional LTS release.
var ltsRegistry = []LTSSpec{
	{
		InstallVersion: "codesphere-lts-v1.77.2",
		UnsupportedExperiments: []string{
			"secret-management",
			"sub-path-mount",
		},
		ClearManagedServices:    true,
		RequiresJumpboxFiles:    true,
		RequiresOmsBinaryUpdate: true,
	},
}

// FindLTSSpec returns the LTSSpec for the given installVersion, or nil if it is not a
// known LTS release that requires special handling.
func FindLTSSpec(installVersion string) *LTSSpec {
	for i := range ltsRegistry {
		if ltsRegistry[i].InstallVersion == installVersion {
			return &ltsRegistry[i]
		}
	}
	return nil
}

// LocalCodesphereConfigPath derives the local path for the separate codesphere config file
// from the main config path (same directory, filename "codesphere.yaml").
func LocalCodesphereConfigPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "codesphere.yaml")
}

// LocalJumpboxConfigPath derives the local path for the jumpbox-specific config from the
// main config path (same directory, filename "config-jumpbox.yaml").
func LocalJumpboxConfigPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "config-jumpbox.yaml")
}

// GenerateLTSJumpboxFiles generates the two files needed on the jumpbox for an LTS release
// without modifying the original root config:
//   - jumpboxConfigBytes: config.yaml with inline compat-stripped codesphere object
//   - codesphereBytes: standalone codesphere.yaml with the same compat-stripped codesphere config
func GenerateLTSJumpboxFiles(root *files.RootConfig, spec *LTSSpec) (jumpboxConfigBytes, codesphereBytes []byte, err error) {
	csCopy := root.Codesphere
	ApplyLTSCompat(&csCopy, spec)

	codesphereBytes, err = yaml.Marshal(csCopy)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal %s codesphere config: %w", spec.InstallVersion, err)
	}

	rootCopy := *root
	rootCopy.Codesphere = csCopy

	jumpboxConfigBytes, err = rootCopy.Marshal()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal %s jumpbox config: %w", spec.InstallVersion, err)
	}

	return jumpboxConfigBytes, codesphereBytes, nil
}

// ApplyLTSCompat adjusts a CodesphereConfig to be compatible with the given LTS release:
//  1. Experiments not present in the LTS release are removed.
//  2. ManagedServices is cleared when the LTS schema requires full provider definitions.
func ApplyLTSCompat(cfg *files.CodesphereConfig, spec *LTSSpec) {
	cfg.Experiments = FilterExperiments(cfg.Experiments, spec.UnsupportedExperiments)
	if spec.ClearManagedServices {
		cfg.ManagedServices = nil
	}
}

// FilterExperiments returns a new slice of experiments with the unsupported ones removed.
func FilterExperiments(experiments, unsupported []string) []string {
	unsupportedSet := make(map[string]struct{}, len(unsupported))
	for _, u := range unsupported {
		unsupportedSet[u] = struct{}{}
	}

	filtered := make([]string, 0, len(experiments))
	for _, exp := range experiments {
		if _, remove := unsupportedSet[exp]; !remove {
			filtered = append(filtered, exp)
		}
	}
	return filtered
}
