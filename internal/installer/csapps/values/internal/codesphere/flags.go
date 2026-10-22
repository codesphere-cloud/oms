// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// WorkspacePVCStorageFlag stores workspaces on PVCs instead of CephFS.
const WorkspacePVCStorageFlag = "workspace-pvc-storage"

const prodEnv = "prod"

// EnabledFlags are the flags the installer enables from the config's internal, deprecated
// experiments, preview and feature flags.
func EnabledFlags(conf files.CodesphereConfig) map[string]bool {
	flags := map[string]bool{}
	// The installer still reads the deprecated experiments.
	for _, flag := range slices.Concat(conf.Internal, conf.Experiments) { //nolint:staticcheck
		flags[flag] = true
	}

	for _, toggles := range []map[string]bool{conf.Preview, conf.Features} {
		for flag, enabled := range toggles {
			if enabled {
				flags[flag] = true
			}
		}
	}

	return flags
}

func experimentsValues(conf files.CodesphereConfig, workspacesGatewayEnabled bool) map[string]any {
	experiments := map[string]any{
		"hermetic":       []string{prodEnv},
		"persistent-nix": []string{prodEnv},
	}
	if workspacesGatewayEnabled {
		experiments["gateway-domains"] = []string{prodEnv}
	}

	for _, flag := range slices.Concat(conf.Experiments, conf.Internal) { //nolint:staticcheck
		experiments[flag] = []string{prodEnv}
	}

	for flag, enabled := range conf.Preview {
		if enabled {
			experiments[flag] = []string{prodEnv}
		}
	}

	return experiments
}

func featuresValues(configured map[string]bool) map[string]any {
	features := map[string]any{
		"email-signup":     []string{prodEnv},
		"email-signin":     []string{prodEnv},
		"standalone-teams": []string{prodEnv},
		"billing":          []string{},
	}

	for feature, enabled := range configured {
		if enabled {
			features[feature] = []string{prodEnv}
		} else {
			features[feature] = []string{}
		}
	}

	return features
}
