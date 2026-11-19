// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import "github.com/codesphere-cloud/oms/internal/installer/files"

// KyvernoPolicies is the key of the kyverno-policies chart under `applications` in the
// pc-applications values.
const KyvernoPolicies = "kyverno-policies"

// KyvernoPoliciesPcAppsValues passes cluster.kyverno.policiesOverride to the kyverno-policies
// application, which lets the single-replica profiles keep the PDB policy in Audit mode.
// They are the base of the pc-apps values: an explicit `pcApps` block in config.yaml and any
// --pc-apps-values file still override them.
// Returns nil when no override is configured, leaving the chart defaults untouched.
func KyvernoPoliciesPcAppsValues(config *files.RootConfig) files.ChartValues {
	kyverno := config.Cluster.Kyverno
	if kyverno == nil || len(kyverno.PoliciesOverride) == 0 {
		return nil
	}

	return files.ChartValues{
		"applications": files.ChartValues{
			KyvernoPolicies: files.ChartValues{
				"valuesObject": kyverno.PoliciesOverride,
			},
		},
	}
}
