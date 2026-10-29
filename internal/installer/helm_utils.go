// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	helmchart "helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/cli/values"
	"helm.sh/helm/v4/pkg/getter"
)

// ChartInfo exposes the chart details needed by installers without Helm SDK types.
type ChartInfo struct {
	Name         string
	Version      string
	Values       map[string]any
	Dependencies []string
}

// Validate checks a pinned chart's identity and required bundled dependencies.
func (c ChartInfo) Validate(name, version string, dependencies ...string) error {
	if _, err := semver.StrictNewVersion(version); err != nil {
		return fmt.Errorf("invalid chart version %q: %w", version, err)
	}

	if c.Name != name || c.Version != version {
		return fmt.Errorf("chart %s:%s does not match expected %s:%s", c.Name, c.Version, name, version)
	}

	for _, dependency := range dependencies {
		if !slices.Contains(c.Dependencies, dependency) {
			return fmt.Errorf("chart %s is missing its bundled %s dependency", name, dependency)
		}
	}

	return nil
}

// ChartLoader isolates chart and values file reads from installer orchestration.
//
//mockery:generate: true
type ChartLoader interface {
	LoadChart(path string) (ChartInfo, error)
	LoadValues(paths []string) (map[string]any, error)
}

type chartLoader struct{}

// NewChartLoader creates a loader backed by the Helm SDK.
func NewChartLoader() ChartLoader { return chartLoader{} }

func (chartLoader) LoadChart(path string) (ChartInfo, error) {
	loaded, err := loader.Load(path)
	if err != nil {
		return ChartInfo{}, fmt.Errorf("loading Helm chart: %w", err)
	}

	chart, err := helmchart.NewAccessor(loaded)
	if err != nil {
		return ChartInfo{}, fmt.Errorf("accessing Helm chart: %w", err)
	}

	metadata := chart.MetadataAsMap()
	name, _ := metadata["Name"].(string)
	version, _ := metadata["Version"].(string)

	info := ChartInfo{Name: name, Version: version, Values: chart.Values()}
	for _, dependency := range chart.Dependencies() {
		accessor, err := helmchart.NewAccessor(dependency)
		if err != nil {
			return ChartInfo{}, fmt.Errorf("accessing Helm chart dependency: %w", err)
		}

		info.Dependencies = append(info.Dependencies, accessor.Name())
	}

	return info, nil
}

func (chartLoader) LoadValues(paths []string) (map[string]any, error) {
	result, err := (&values.Options{ValueFiles: paths}).MergeValues(getter.All(cli.New()))
	if err != nil {
		return nil, fmt.Errorf("loading Helm values: %w", err)
	}

	return result, nil
}

// CompareChartVersions returns -1, 0, or 1 when requested is older, equal, or newer.
// Callers decide whether a comparison should reject, skip, or perform an upgrade.
func CompareChartVersions(requestedVersion, installedVersion string) (int, error) {
	installed, err := semver.NewVersion(installedVersion)
	if err != nil {
		return 0, fmt.Errorf("failed to parse installed version %q: %w", installedVersion, err)
	}

	requested, err := semver.NewVersion(requestedVersion)
	if err != nil {
		return 0, fmt.Errorf("failed to parse requested version %q: %w", requestedVersion, err)
	}

	return requested.Compare(installed), nil
}

// ValidateChartRepositoryURL validates the repository schemes supported by Helm.
func ValidateChartRepositoryURL(repoURL string) error {
	if repoURL == "" {
		return nil
	}

	for _, prefix := range []string{"http://", "https://", "oci://"} {
		if strings.HasPrefix(repoURL, prefix) {
			return nil
		}
	}

	return fmt.Errorf("invalid repo URL %q: must start with http://, https://, or oci://", repoURL)
}

// ResolveChartReference expands OCI chart names; HTTP repositories stay separate.
func ResolveChartReference(repoURL, chartName string) (string, string) {
	if strings.HasPrefix(repoURL, "oci://") {
		return strings.TrimRight(repoURL, "/") + "/" + chartName, ""
	}

	return chartName, repoURL
}

// LoginChartRegistry authenticates private OCI chart references when credentials are set.
func LoginChartRegistry(ctx context.Context, helm HelmClient, chartName, username, password string) error {
	if !strings.HasPrefix(chartName, "oci://") || password == "" {
		return nil
	}

	if err := helm.LoginRegistry(ctx, RegistryHost(chartName), username, password); err != nil {
		return fmt.Errorf("logging in to chart registry: %w", err)
	}

	return nil
}
