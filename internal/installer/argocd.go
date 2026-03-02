// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Masterminds/semver/v3"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/release"
)

type ArgoCDManager interface {
	Install() error
}

type ArgoCD struct {
	Version string
}

func NewArgoCD(version string) ArgoCDManager {
	return &ArgoCD{
		Version: version,
	}
}

// Install the ArgoCD chart
func (a *ArgoCD) Install() error {
	if a.Version == "" {
		a.Version = "9.1.4"
	}
	log.Printf("Installing/Upgrading ArgoCD helm chart version %s\n", a.Version)

	settings := cli.New()
	ctx := context.Background()

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), "argocd", os.Getenv("HELM_DRIVER")); err != nil {
		return fmt.Errorf("init failed: %w", err)
	}

	// Check if a release already exists
	listClient := action.NewList(actionConfig)
	listClient.Filter = "^argocd$"
	listClient.Deployed = true
	listClient.SetStateMask()

	releases, err := listClient.Run()
	if err != nil {
		return fmt.Errorf("list releases failed: %w", err)
	}

	// Find existing "argocd" release using the Accessor interface
	var existingAccessor release.Accessor

	for _, r := range releases {
		acc, err := release.NewAccessor(r)
		if err != nil {
			continue
		}
		if acc.Name() == "argocd" {
			existingAccessor = acc
			break
		}
	}

	chartName := "argo-cd"
	repoURL := "https://argoproj.github.io/argo-helm"
	vals := map[string]interface{}{
		"dex": map[string]interface{}{
			"enabled": false,
		},
	}

	if existingAccessor != nil {
		// A release already exists — compare versions using the chart accessor
		chartAcc, err := chart.NewAccessor(existingAccessor.Chart())
		if err != nil {
			return fmt.Errorf("failed to access chart metadata: %w", err)
		}
		metadata := chartAcc.MetadataAsMap()
		installedVersion, _ := metadata["Version"].(string)
		log.Printf("Found existing ArgoCD release with chart version %s\n", installedVersion)

		installedSemver, err := semver.NewVersion(installedVersion)
		if err != nil {
			return fmt.Errorf("failed to parse installed version %q: %w", installedVersion, err)
		}
		requestedSemver, err := semver.NewVersion(a.Version)
		if err != nil {
			return fmt.Errorf("failed to parse requested version %q: %w", a.Version, err)
		}

		if requestedSemver.LessThan(installedSemver) {
			return fmt.Errorf(
				"requested version %s is older than installed version %s; downgrade is not allowed",
				a.Version, installedVersion,
			)
		}

		// Version is equal or larger — perform an upgrade
		log.Printf("Upgrading ArgoCD from %s to %s\n", installedVersion, a.Version)

		upgradeClient := action.NewUpgrade(actionConfig)
		upgradeClient.Namespace = "argocd"
		upgradeClient.WaitStrategy = "watcher"
		upgradeClient.Version = a.Version
		upgradeClient.ChartPathOptions.RepoURL = repoURL

		chartPath, err := upgradeClient.ChartPathOptions.LocateChart(chartName, settings)
		if err != nil {
			return fmt.Errorf("LocateChart failed: %w", err)
		}

		chartRequested, err := loader.Load(chartPath)
		if err != nil {
			return fmt.Errorf("load failed: %w", err)
		}

		_, err = upgradeClient.RunWithContext(ctx, existingAccessor.Name(), chartRequested, vals)
		if err != nil {
			return fmt.Errorf("upgrade failed: %w", err)
		}

		fmt.Printf("Successfully upgraded Argo CD to chart version %s\n", a.Version)
	} else {
		// No existing release — perform a fresh install
		log.Println("No existing ArgoCD release found, performing fresh install")

		installClient := action.NewInstall(actionConfig)
		installClient.ReleaseName = "argocd"
		installClient.Namespace = "argocd"
		installClient.CreateNamespace = true
		installClient.DryRunStrategy = "none"
		installClient.WaitStrategy = "watcher"
		installClient.Version = a.Version
		installClient.ChartPathOptions.RepoURL = repoURL

		chartPath, err := installClient.ChartPathOptions.LocateChart(chartName, settings)
		if err != nil {
			return fmt.Errorf("LocateChart failed: %w", err)
		}

		chartRequested, err := loader.Load(chartPath)
		if err != nil {
			return fmt.Errorf("load failed: %w", err)
		}

		_, err = installClient.RunWithContext(ctx, chartRequested, vals)
		if err != nil {
			return fmt.Errorf("install failed: %w", err)
		}

		fmt.Printf("Successfully installed Argo CD (chart version: %s)\n", a.Version)
	}

	return nil
}
