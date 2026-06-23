// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"
	"path/filepath"
	"slices"
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
	// Only these experiments are passed to the installer, setting any experiment not in
	// this list will cause an error during ApplyLTSCompat.
	Experiments []string
	// ClearManagedServices instructs the compat layer to set ManagedServices to nil.
	// Required when the LTS schema expects full provider definitions, not the abbreviated
	// form stored in config.yaml.
	ClearManagedServices bool
	// RequiresJumpboxFiles instructs the bootstrap to generate LTS-versioned compat config files
	// (e.g. config-lts-1_77_2.yaml) instead of using config.yaml directly.
	// This is needed for LTS installers whose schema differs from the current config format.
	RequiresJumpboxFiles bool
	// RequiresOmsBinaryUpdate instructs the bootstrap to build a fresh linux/amd64 OMS binary
	// and deploy it to the jumpbox before running the installer.
	// This is needed when the LTS installer relies on OMS CLI features only present in the
	// version of OMS that bootstraps the environment (not the OMS binary shipped with the LTS).
	RequiresOmsBinaryUpdate bool
	// RequiresCephMasterWatcher instructs the bootstrap to start a background watcher process
	// on the Ceph master node that continuously re-adds the master to the Ceph orchestrator
	// host inventory. This works around a bug in the LTS installer's configureHosts step
	// that removes the master from inventory when applying a declarative host spec.
	RequiresCephMasterWatcher bool
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
		ClearManagedServices:      false,
		RequiresJumpboxFiles:      true,
		RequiresOmsBinaryUpdate:   true,
		RequiresCephMasterWatcher: true,
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
// config-lts-<version>.yaml with compat applied (experiments filtered).
// The codesphere key is omitted from the output — the LTS 1.77.2 installer rejects
// both inline objects and file-path references; only the absent key is accepted.
func GenerateLTSJumpboxFiles(root *files.RootConfig, spec *LTSSpec) (jumpboxConfigBytes []byte, err error) {
	csCopy := root.Codesphere
	if err := ApplyLTSCompat(&csCopy, spec); err != nil {
		return nil, fmt.Errorf("failed to apply LTS compat for %s: %w", spec.InstallVersion, err)
	}

	rootCopy := *root
	rootCopy.Codesphere = csCopy
	rootCopy.CodesphereConfigPath = files.OmitCodesphereSentinel
	rootCopy.GeneratedForVersion = ""

	jumpboxConfigBytes, err = rootCopy.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s jumpbox config: %w", spec.InstallVersion, err)
	}

	return jumpboxConfigBytes, nil
}

// ApplyLTSCompat adjusts a CodesphereConfig to be compatible with the given LTS release:
//  1. Validates that no unsupported experiments are set.
//  2. Only experiments listed in the LTS spec are kept; all others are stripped.
//  3. ManagedServices is cleared when the LTS schema requires full provider definitions.
func ApplyLTSCompat(cfg *files.CodesphereConfig, spec *LTSSpec) error {
	if err := ValidateExperiments(cfg.Experiments, spec.Experiments); err != nil {
		return fmt.Errorf("invalid experiments for %s: %w", spec.InstallVersion, err)
	}
	cfg.Experiments = FilterExperiments(cfg.Experiments, spec.Experiments)
	if spec.ClearManagedServices {
		cfg.ManagedServices = nil
	}
	return nil
}

// ValidateExperiments checks that all experiments in the given slice are present in the
// allowed list. Returns an error listing any unsupported experiments.
func ValidateExperiments(experiments, allowed []string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = struct{}{}
	}

	var unsupported []string
	for _, exp := range experiments {
		if _, ok := allowedSet[exp]; !ok {
			unsupported = append(unsupported, exp)
		}
	}

	if len(unsupported) > 0 {
		return fmt.Errorf("unsupported experiments: %v (supported by this version: %v)", unsupported, allowed)
	}
	return nil
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

// UserSpecifiedExperiments checks whether the given experiments list differs from the
// default experiments. If the user explicitly passed --experiments flags, they
// differ from defaults and we should error on unsupported ones.
func UserSpecifiedExperiments(experiments []string) bool {
	return !slices.Equal(experiments, DefaultExperiments)
}
