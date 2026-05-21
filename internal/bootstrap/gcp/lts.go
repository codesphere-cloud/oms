// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// LTSSpec describes the compatibility requirements for a specific LTS release.
// To add support for a new LTS version, add a new entry to ltsRegistry — no other
// files need to change.
type LTSSpec struct {
	// InstallVersion is the exact install version string that identifies this LTS release.
	InstallVersion string
	// Experiments lists all experiments supported by this LTS release.
	// Only these experiments are passed to the installer, any others are stripped.
	Experiments []string
	// ClearManagedServices instructs the compat layer to set ManagedServices to nil.
	// Required when the LTS schema expects full provider definitions, not the abbreviated
	// form stored in config.yaml.
	ClearManagedServices bool
	// RequiresJumpboxFiles instructs the bootstrap to generate LTS-versioned compat config files
	// (e.g. config-lts-1_77_2.yaml and codesphere-lts-1_77_2.yaml) instead of using config.yaml directly.
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
		Experiments: []string{
			"managed-services",
			"custom-service-image",
			"ms-in-ls",
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

// LTSConfigFileSuffix derives a filesystem-safe suffix from an LTS installVersion string.
// For example, "codesphere-lts-v1.77.2" -> "lts-1_77_2".
func LTSConfigFileSuffix(installVersion string) string {
	s := strings.TrimPrefix(installVersion, "codesphere-")
	s = strings.ReplaceAll(s, "v", "")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}

// LocalLTSConfigPath derives the local path for the LTS-specific jumpbox config from the
// main config path. For example, with installVersion "codesphere-lts-v1.77.2" and
// configPath "config.yaml" it returns "config-lts-1_77_2.yaml".
func LocalLTSConfigPath(configPath string, spec *LTSSpec) string {
	return filepath.Join(filepath.Dir(configPath), "config-"+LTSConfigFileSuffix(spec.InstallVersion)+".yaml")
}

// GenerateLTSJumpboxFiles generates the LTS-versioned config file needed on the jumpbox
// without modifying the original root config. It returns the bytes for
// config-lts-<version>.yaml with the compat-stripped codesphere section inlined.
func GenerateLTSJumpboxFiles(root *files.RootConfig, spec *LTSSpec) (jumpboxConfigBytes []byte, err error) {
	csCopy := root.Codesphere
	ApplyLTSCompat(&csCopy, spec)

	rootCopy := *root
	rootCopy.Codesphere = csCopy
	// Clear the version annotation so the old LTS installer does not encounter an unknown field.
	rootCopy.GeneratedForVersion = ""

	jumpboxConfigBytes, err = rootCopy.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s jumpbox config: %w", spec.InstallVersion, err)
	}

	return jumpboxConfigBytes, nil
}

// ApplyLTSCompat adjusts a CodesphereConfig to be compatible with the given LTS release:
//  1. Only experiments listed in the LTS spec are kept; all others are stripped.
//  2. ManagedServices is cleared when the LTS schema requires full provider definitions.
func ApplyLTSCompat(cfg *files.CodesphereConfig, spec *LTSSpec) {
	cfg.Experiments = FilterExperiments(cfg.Experiments, spec.Experiments)
	if spec.ClearManagedServices {
		cfg.ManagedServices = nil
	}
}

// FilterExperiments returns a new slice containing only the experiments present in the allowed list.
func FilterExperiments(experiments, allowed []string) []string {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = struct{}{}
	}

	filtered := make([]string, 0, len(experiments))
	for _, exp := range experiments {
		if _, ok := allowedSet[exp]; ok {
			filtered = append(filtered, exp)
		}
	}
	return filtered
}
