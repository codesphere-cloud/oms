// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	helmchart "helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli/values"
	"helm.sh/helm/v4/pkg/getter"
)

// CodesphereChartConfig selects the bundled chart by default. An explicit version
// selects the upstream installer, whose chart version and repo have their own semantics.
func CodesphereChartConfig(config *bom.Config, chartPath, version, repoURL string) (InstallerConfig, error) {
	if version != "" {
		return InstallerConfig{Version: version, RepoURL: repoURL}, nil
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

// PackagedChartLoader isolates chart and values file reads from orchestration.
//
//mockery:generate: true
type PackagedChartLoader interface {
	LoadChart(path, version string) (map[string]any, error)
	LoadValues(paths []string) (map[string]any, error)
}

type packagedChartLoader struct{}

func (packagedChartLoader) LoadChart(path, version string) (map[string]any, error) {
	loaded, err := loader.Load(path)
	if err != nil {
		return nil, fmt.Errorf("loading packaged ArgoCD chart: %w", err)
	}

	chart, err := helmchart.NewAccessor(loaded)
	if err != nil {
		return nil, fmt.Errorf("accessing packaged ArgoCD chart: %w", err)
	}

	metadata := chart.MetadataAsMap()
	if metadata["Name"] != "argocd" || metadata["Version"] != version {
		return nil, fmt.Errorf("packaged ArgoCD chart %v:%v does not match BOM argocd:%s", metadata["Name"], metadata["Version"], version)
	}

	if _, err := semver.StrictNewVersion(version); err != nil {
		return nil, fmt.Errorf("invalid packaged ArgoCD chart version: %w", err)
	}

	if err := validateBundledArgoCD(chart.Dependencies()); err != nil {
		return nil, err
	}

	return chart.Values(), nil
}

func validateBundledArgoCD(dependencies []helmchart.Charter) error {
	for _, dep := range dependencies {
		accessor, err := helmchart.NewAccessor(dep)
		if err != nil {
			return fmt.Errorf("accessing packaged ArgoCD dependency: %w", err)
		}

		if accessor.Name() == "argo-cd" {
			return nil
		}
	}

	return fmt.Errorf("packaged ArgoCD chart is missing its bundled argo-cd dependency")
}

func (packagedChartLoader) LoadValues(paths []string) (map[string]any, error) {
	result, err := (&values.Options{ValueFiles: paths}).MergeValues(getter.Getters())
	if err != nil {
		return nil, fmt.Errorf("loading packaged ArgoCD values: %w", err)
	}

	return result, nil
}
