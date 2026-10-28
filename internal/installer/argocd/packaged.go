// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"context"
	"fmt"
	"log"

	"github.com/Masterminds/semver/v3"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/distribution/reference"
	"helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/kube"
)

// installPackaged only manages releases from the Codesphere chart family. Upstream
// chart versions and wrapper chart versions are independent version sequences.
func (a *Installer) installPackaged() error {
	ctx := context.Background()

	existing, err := a.Helm.FindRelease(DefaultNamespace, "argocd")
	if err != nil {
		return fmt.Errorf("finding existing ArgoCD release: %w", err)
	}

	if err := a.reconcilePackagedChart(ctx, existing); err != nil {
		return err
	}
	// Retaining an existing chart must not skip credentials or subsequent app setup.
	if a.FullInstall {
		if err := a.Resources.ApplyAll(ctx); err != nil {
			return fmt.Errorf("applying ArgoCD resources: %w", err)
		}
	}

	a.showPostInstallHints()

	return nil
}

func (a *Installer) reconcilePackagedChart(ctx context.Context, existing *installer.ReleaseInfo) error {
	manage, err := a.shouldManagePackagedChart(existing)
	if err != nil {
		return err
	}

	if !manage {
		return nil
	}

	cfg, err := a.packagedChartConfig()
	if err != nil {
		return err
	}

	if existing == nil {
		if err := a.Helm.InstallChart(ctx, cfg, installer.InstallChartOptions{ForceConflicts: a.ForceConflicts}); err != nil {
			return fmt.Errorf("installing packaged ArgoCD chart: %w", err)
		}

		return nil
	}

	if err := a.Helm.UpgradeChart(ctx, cfg, installer.UpgradeChartOptions{ForceConflicts: a.ForceConflicts}); err != nil {
		return fmt.Errorf("upgrading packaged ArgoCD chart: %w", err)
	}

	return nil
}

func (a *Installer) shouldManagePackagedChart(existing *installer.ReleaseInfo) (bool, error) {
	if existing == nil {
		return true, nil
	}

	if existing.ChartName != "argocd" {
		log.Printf("Preserving existing ArgoCD release using chart %q", existing.ChartName)
		return false, nil
	}

	return a.isNewerPackagedChart(existing.InstalledVersion)
}

func (a *Installer) isNewerPackagedChart(installedVersion string) (bool, error) {
	installed, err := semver.StrictNewVersion(installedVersion)
	if err != nil {
		return false, fmt.Errorf("invalid installed Codesphere ArgoCD chart version: %w", err)
	}

	requested, err := semver.StrictNewVersion(a.Version)
	if err != nil {
		return false, fmt.Errorf("invalid packaged ArgoCD chart version: %w", err)
	}

	if requested.GreaterThan(installed) {
		return true, nil
	}

	log.Printf("Retaining ArgoCD chart %s; package contains %s", installedVersion, a.Version)

	return false, nil
}

func (a *Installer) packagedChartConfig() (installer.ChartConfig, error) {
	cfg := installer.ChartConfig{ReleaseName: "argocd", Namespace: DefaultNamespace, ChartName: a.PackageChart, Version: a.Version, CreateNamespace: true, WaitStrategy: kube.LegacyStrategy}

	chartValues, err := a.PackageLoader.LoadChart(a.PackageChart, a.Version)
	if err != nil {
		return cfg, fmt.Errorf("loading packaged ArgoCD chart: %w", err)
	}

	trust, err := a.registryTrustValues()
	if err != nil {
		return cfg, err
	}

	upstream := util.MergeTables(map[string]any{"dex": map[string]any{"enabled": false}}, trust)

	if a.ImageRegistry != "" {
		images, err := a.packagedImageValues(chartValues)
		if err != nil {
			return cfg, err
		}

		upstream = util.MergeTables(upstream, images)
	}

	defaults := map[string]any{"argo-cd": upstream, "extraSecrets": []any{}}

	overrides, err := a.PackageLoader.LoadValues(a.ValueFiles)
	if err != nil {
		return cfg, fmt.Errorf("loading packaged ArgoCD values: %w", err)
	}

	cfg.Values = util.MergeTables(overrides, defaults)

	return cfg, nil
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

		dest, err := installer.PackageImageDestination(repository+":mapping", a.ImageRegistry)
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
