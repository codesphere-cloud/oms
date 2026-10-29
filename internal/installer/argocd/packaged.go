// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"fmt"
	"log"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/distribution/reference"
	"helm.sh/helm/v4/pkg/chart/common/util"
)

func (a *Installer) shouldManagePackagedChart(existing *installer.ReleaseInfo) (bool, error) {
	if existing == nil {
		return true, nil
	}

	if existing.ChartName != "argocd" {
		log.Printf("Preserving existing ArgoCD release using chart %q", existing.ChartName)
		return false, nil
	}

	comparison, err := installer.CompareChartVersions(a.Version, existing.InstalledVersion)
	if err != nil {
		return false, fmt.Errorf("comparing ArgoCD chart versions: %w", err)
	}

	if comparison > 0 {
		return true, nil
	}

	log.Printf("Retaining ArgoCD chart %s; package contains %s", existing.InstalledVersion, a.Version)

	return false, nil
}

func (a *Installer) packagedDefaults(upstream map[string]any) (map[string]any, error) {
	chart, err := a.chartLoader().LoadChart(a.PackageChart)
	if err != nil {
		return nil, fmt.Errorf("loading packaged ArgoCD chart: %w", err)
	}

	if err := chart.Validate("argocd", a.Version, "argo-cd"); err != nil {
		return nil, fmt.Errorf("validating packaged ArgoCD chart: %w", err)
	}

	if a.ImageRegistry != "" {
		images, err := a.packagedImageValues(chart.Values)
		if err != nil {
			return nil, err
		}

		upstream = util.MergeTables(upstream, images)
	}

	return map[string]any{"argo-cd": upstream, "extraSecrets": []any{}}, nil
}

// Resolve image destinations by the same rule used by `oms copy package`.
func (a *Installer) packagedImageValues(chartValues map[string]any) (map[string]any, error) {
	upstream, ok := chartValues["argo-cd"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("packaged ArgoCD chart has no argo-cd values")
	}

	result := map[string]any{}

	for _, component := range []string{"global", "dex", "redis"} {
		vals, _ := upstream[component].(map[string]any)
		image, _ := vals["image"].(map[string]any)

		repository, _ := image["repository"].(string)
		if repository == "" {
			return nil, fmt.Errorf("packaged ArgoCD chart has no %s image repository", component)
		}

		dest, err := installer.PackageImageDestination(repository, a.ImageRegistry)
		if err != nil {
			return nil, fmt.Errorf("mapping ArgoCD %s image: %w", component, err)
		}

		named, err := reference.ParseNormalizedNamed(dest)
		if err != nil {
			return nil, fmt.Errorf("parsing ArgoCD %s image: %w", component, err)
		}

		result[component] = map[string]any{"image": map[string]any{"repository": named.Name()}}
	}

	return result, nil
}
