// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"os"
	"time"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/release"
)

// ReleaseInfo holds the details of an existing Helm release that the rest of
// the application cares about — completely decoupled from the Helm SDK types.
type ReleaseInfo struct {
	Name             string
	InstalledVersion string // chart version currently deployed
}

// ChartConfig describes *what* to install/upgrade and *where*.
type ChartConfig struct {
	ReleaseName     string
	ChartName       string
	RepoURL         string
	Namespace       string
	Version         string // "" means latest
	Values          map[string]interface{}
	CreateNamespace bool
}

// HelmClient is the seam that makes the Helm SDK mockable.
// Every method receives only plain Go types so that test doubles never need to
// import any helm.sh package.
type HelmClient interface {
	// FindRelease returns info about an existing release, or nil if none exists.
	FindRelease(releaseName string) (*ReleaseInfo, error)

	// InstallChart performs a fresh Helm install and returns an error on failure.
	InstallChart(ctx context.Context, cfg ChartConfig) error

	// UpgradeChart upgrades an existing Helm release and returns an error on failure.
	UpgradeChart(ctx context.Context, cfg ChartConfig) error
}

// ---------------------------------------------------------------------------
// Concrete implementation backed by the Helm Go SDK v4
// ---------------------------------------------------------------------------

type helmClient struct {
	settings     *cli.EnvSettings
	actionConfig *action.Configuration
}

func NewHelmClient(namespace string) (HelmClient, error) {
	settings := cli.New()
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(
		settings.RESTClientGetter(),
		namespace,
		os.Getenv("HELM_DRIVER"),
	); err != nil {
		return nil, fmt.Errorf("helm action config init failed: %w", err)
	}
	return &helmClient{
		settings:     settings,
		actionConfig: actionConfig,
	}, nil
}

func (h *helmClient) FindRelease(releaseName string) (*ReleaseInfo, error) {
	listClient := action.NewList(h.actionConfig)
	listClient.Filter = "^" + releaseName + "$"
	listClient.Deployed = true
	listClient.SetStateMask()

	releases, err := listClient.Run()
	if err != nil {
		return nil, fmt.Errorf("list releases failed: %w", err)
	}

	for _, r := range releases {
		acc, err := release.NewAccessor(r)
		if err != nil {
			continue
		}
		if acc.Name() != releaseName {
			continue
		}

		chartAcc, err := chart.NewAccessor(acc.Chart())
		if err != nil {
			return nil, fmt.Errorf("failed to access chart metadata: %w", err)
		}
		metadata := chartAcc.MetadataAsMap()
		version, _ := metadata["Version"].(string)

		return &ReleaseInfo{
			Name:             acc.Name(),
			InstalledVersion: version,
		}, nil
	}

	return nil, nil // no release found
}

func (h *helmClient) InstallChart(ctx context.Context, cfg ChartConfig) error {
	installClient := action.NewInstall(h.actionConfig)
	installClient.ReleaseName = cfg.ReleaseName
	installClient.Namespace = cfg.Namespace
	installClient.CreateNamespace = cfg.CreateNamespace
	installClient.DryRunStrategy = "none"
	installClient.WaitStrategy = "watcher"
	installClient.Version = cfg.Version
	installClient.RepoURL = cfg.RepoURL
	installClient.Timeout = 5 * time.Minute

	chartPath, err := installClient.LocateChart(cfg.ChartName, h.settings)
	if err != nil {
		return fmt.Errorf("LocateChart failed: %w", err)
	}

	chartRequested, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("load chart failed: %w", err)
	}

	_, err = installClient.RunWithContext(ctx, chartRequested, cfg.Values)
	if err != nil {
		return fmt.Errorf("install failed: %w", err)
	}

	return nil
}

func (h *helmClient) UpgradeChart(ctx context.Context, cfg ChartConfig) error {
	upgradeClient := action.NewUpgrade(h.actionConfig)
	upgradeClient.Namespace = cfg.Namespace
	upgradeClient.WaitStrategy = "watcher"
	upgradeClient.Version = cfg.Version
	upgradeClient.RepoURL = cfg.RepoURL

	chartPath, err := upgradeClient.LocateChart(cfg.ChartName, h.settings)
	if err != nil {
		return fmt.Errorf("LocateChart failed: %w", err)
	}

	chartRequested, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("load chart failed: %w", err)
	}

	_, err = upgradeClient.RunWithContext(ctx, cfg.ReleaseName, chartRequested, cfg.Values)
	if err != nil {
		return fmt.Errorf("upgrade failed: %w", err)
	}

	return nil
}
