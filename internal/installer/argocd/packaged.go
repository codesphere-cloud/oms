// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/distribution/reference"
	helmchart "helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli/values"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// installPackaged only manages releases from the Codesphere chart family. Upstream
// chart versions and wrapper chart versions are independent version sequences.
func (a *Installer) installPackaged() error {
	ctx := context.Background()

	existing, err := a.Helm.FindRelease(DefaultNamespace, "argocd")
	if err != nil {
		return fmt.Errorf("finding existing ArgoCD release: %w", err)
	}

	manage := true

	if existing != nil {
		if existing.ChartName != "argocd" {
			log.Printf("Preserving existing ArgoCD release using chart %q", existing.ChartName)

			manage = false
		} else {
			installed, err := semver.StrictNewVersion(existing.InstalledVersion)
			if err != nil {
				return fmt.Errorf("invalid installed Codesphere ArgoCD chart version: %w", err)
			}

			requested, err := semver.StrictNewVersion(a.Version)
			if err != nil {
				return fmt.Errorf("invalid packaged ArgoCD chart version: %w", err)
			}

			if !requested.GreaterThan(installed) {
				log.Printf("Retaining ArgoCD chart %s; package contains %s", existing.InstalledVersion, a.Version)

				manage = false
			}
		}
	} else {
		found, err := a.hasArgoCDWorkloads(ctx)
		if err != nil {
			return err
		}

		if found {
			log.Print("Preserving existing ArgoCD workloads without the expected Helm release")

			manage = false
		}
	}

	if manage {
		cfg, err := a.packagedChartConfig()
		if err != nil {
			return err
		}

		if existing == nil {
			err = a.Helm.InstallChart(ctx, cfg, installer.InstallChartOptions{ForceConflicts: a.ForceConflicts})
		} else {
			err = a.Helm.UpgradeChart(ctx, cfg, installer.UpgradeChartOptions{ForceConflicts: a.ForceConflicts})
		}

		if err != nil {
			return fmt.Errorf("installing packaged ArgoCD chart: %w", err)
		}
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

func (a *Installer) hasArgoCDWorkloads(ctx context.Context) (bool, error) {
	if a.Clientset == nil {
		return false, fmt.Errorf("kubernetes client is required to detect existing ArgoCD installations")
	}

	deployments, err := a.Clientset.AppsV1().Deployments(DefaultNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("checking existing ArgoCD deployments: %w", err)
	}

	for _, d := range deployments.Items {
		if d.Name == "argocd-server" || d.Labels["app.kubernetes.io/part-of"] == "argocd" || strings.HasPrefix(d.Labels["app.kubernetes.io/name"], "argocd-") {
			return true, nil
		}
	}

	statefulsets, err := a.Clientset.AppsV1().StatefulSets(DefaultNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("checking existing ArgoCD statefulsets: %w", err)
	}

	for _, s := range statefulsets.Items {
		if s.Name == "argocd-application-controller" || s.Labels["app.kubernetes.io/part-of"] == "argocd" || strings.HasPrefix(s.Labels["app.kubernetes.io/name"], "argocd-") {
			return true, nil
		}
	}

	return false, nil
}

func (a *Installer) packagedChartConfig() (installer.ChartConfig, error) {
	cfg := installer.ChartConfig{ReleaseName: "argocd", Namespace: DefaultNamespace, ChartName: a.PackageChart, Version: a.Version, CreateNamespace: true, WaitStrategy: kube.LegacyStrategy}

	loaded, err := loader.Load(a.PackageChart)
	if err != nil {
		return cfg, fmt.Errorf("loading packaged ArgoCD chart: %w", err)
	}

	chart, err := helmchart.NewAccessor(loaded)
	if err != nil {
		return cfg, fmt.Errorf("accessing packaged ArgoCD chart: %w", err)
	}

	metadata := chart.MetadataAsMap()
	if metadata["Name"] != "argocd" || metadata["Version"] != a.Version {
		return cfg, fmt.Errorf("packaged ArgoCD chart %v:%v does not match BOM argocd:%s", metadata["Name"], metadata["Version"], a.Version)
	}

	if _, err := semver.StrictNewVersion(a.Version); err != nil {
		return cfg, fmt.Errorf("invalid packaged ArgoCD chart version: %w", err)
	}

	dependencyFound := false

	for _, dep := range chart.Dependencies() {
		accessor, err := helmchart.NewAccessor(dep)
		if err != nil {
			return cfg, fmt.Errorf("accessing packaged ArgoCD dependency: %w", err)
		}

		if accessor.Name() == "argo-cd" {
			dependencyFound = true
		}
	}

	if !dependencyFound {
		return cfg, fmt.Errorf("packaged ArgoCD chart is missing its bundled argo-cd dependency")
	}

	trust, err := a.registryTrustValues()
	if err != nil {
		return cfg, err
	}

	upstream := util.MergeTables(map[string]any{"dex": map[string]any{"enabled": false}}, trust)

	if a.ImageRegistry != "" {
		images, err := a.packagedImageValues(chart.Values())
		if err != nil {
			return cfg, err
		}

		upstream = util.MergeTables(upstream, images)
	}

	defaults := map[string]any{"argo-cd": upstream, "extraSecrets": []any{}}

	overrides, err := (&values.Options{ValueFiles: a.ValueFiles}).MergeValues(getter.Getters())
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
