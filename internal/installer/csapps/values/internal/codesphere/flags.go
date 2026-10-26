// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// WorkspacePVCStorageFlag stores workspaces on PVCs instead of CephFS.
const WorkspacePVCStorageFlag = "workspace-pvc-storage"

// FlagEnv is an environment an experiment or feature can be enabled in.
type FlagEnv string

const (
	// FlagEnvProd enables a flag in production.
	FlagEnvProd FlagEnv = "prod"
	// FlagEnvDev enables a flag in development.
	FlagEnvDev FlagEnv = "dev"
)

// Flags are the environments each experiment or feature is enabled in.
type Flags map[string][]FlagEnv

// EnabledFlags maps each flag name to whether the installer enables it, from the config's
// internal, deprecated experiments, preview and feature flags. Disabled flags are absent.
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

func experimentsValues(conf files.CodesphereConfig, workspacesGatewayEnabled bool) Flags {
	experiments := Flags{
		"hermetic":       []FlagEnv{FlagEnvProd},
		"persistent-nix": []FlagEnv{FlagEnvProd},
	}
	if workspacesGatewayEnabled {
		experiments["gateway-domains"] = []FlagEnv{FlagEnvProd}
	}

	for _, flag := range slices.Concat(conf.Experiments, conf.Internal) { //nolint:staticcheck
		experiments[flag] = []FlagEnv{FlagEnvProd}
	}

	for flag, enabled := range conf.Preview {
		if enabled {
			experiments[flag] = []FlagEnv{FlagEnvProd}
		}
	}

	return experiments
}

func featuresValues(configured map[string]bool) Flags {
	features := Flags{
		"email-signup":     []FlagEnv{FlagEnvProd},
		"email-signin":     []FlagEnv{FlagEnvProd},
		"standalone-teams": []FlagEnv{FlagEnvProd},
		"billing":          []FlagEnv{},
	}

	for feature, enabled := range configured {
		if enabled {
			features[feature] = []FlagEnv{FlagEnvProd}
		} else {
			features[feature] = []FlagEnv{}
		}
	}

	return features
}
