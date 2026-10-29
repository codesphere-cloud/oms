// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"fmt"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
)

// CodesphereChartConfig selects the bundled chart by default. An explicit version
// selects the upstream installer, whose chart version and repo have their own semantics.
func CodesphereChartConfig(config *bom.Config, chartPath, version, repoURL string) (InstallerConfig, error) {
	if version != "" {
		if repoURL == "" {
			repoURL = DefaultRepoURL
		}

		return InstallerConfig{Version: version, RepoURL: repoURL, UseUpstream: true}, nil
	}

	if repoURL != "" && repoURL != DefaultRepoURL {
		return InstallerConfig{}, fmt.Errorf("--argo-repo requires --argo-version to select the upstream ArgoCD chart")
	}

	chart, ok := config.GetChart("argocd")
	if !ok {
		return InstallerConfig{}, fmt.Errorf("installer BOM has no tagged argocd chart reference")
	}

	return InstallerConfig{Version: chart.Tag(), PackageChart: chartPath}, nil
}
