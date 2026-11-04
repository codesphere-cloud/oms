// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
)

// ChartSelection contains only the selected chart source and version.
type ChartSelection struct {
	PackageChart string
	Version      string
	RepoURL      string
	UseUpstream  bool
}

// SelectChart selects the bundled chart by default. An explicit version
// selects the upstream installer, whose chart version and repo have their own semantics.
func SelectChart(config *bom.Config, chartPath, version, repoURL string) (ChartSelection, error) {
	if version != "" {
		if repoURL == "" {
			repoURL = DefaultRepoURL
		}

		return ChartSelection{Version: version, RepoURL: repoURL, UseUpstream: true}, nil
	}

	if repoURL != "" && repoURL != DefaultRepoURL {
		return ChartSelection{}, fmt.Errorf("--argo-repo requires --argo-version to select the upstream ArgoCD chart")
	}

	chart, ok := config.GetChart(PackagedChartName)
	if !ok {
		return ChartSelection{}, fmt.Errorf("installer BOM has no tagged argocd chart reference")
	}

	return ChartSelection{Version: chart.Tag(), PackageChart: chartPath}, nil
}
