// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/release"
	"helm.sh/helm/v4/pkg/storage/driver"
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

type UpgradeChartOptions struct {
	InstallIfNotExist bool // if true, perform an install if the release does not already exist
}

// HelmClient is the seam that makes the Helm SDK mockable.
// Every method receives only plain Go types so that test doubles never need to
// import any helm.sh package.
//
//mockery:generate: true
type HelmClient interface {
	// FindRelease returns info about an existing release, or nil if none exists.
	FindRelease(releaseName string) (*ReleaseInfo, error)

	// InstallChart performs a fresh Helm install and returns an error on failure.
	InstallChart(ctx context.Context, cfg ChartConfig) error

	// UpgradeChart upgrades an existing Helm release and returns an error on failure.
	UpgradeChart(ctx context.Context, cfg ChartConfig, opts UpgradeChartOptions) error
}

// ---------------------------------------------------------------------------
// Concrete implementation backed by the Helm Go SDK v4
// ---------------------------------------------------------------------------

type helmClient struct {
	settings         *cli.EnvSettings
	defaultNamespace string
	driver           string
}

func NewHelmClient(namespace string) (HelmClient, error) {
	return &helmClient{
		settings:         cli.New(),
		defaultNamespace: namespace,
		driver:           os.Getenv("HELM_DRIVER"),
	}, nil
}

func (h *helmClient) newActionConfig(namespace string) (*action.Configuration, error) {
	if namespace == "" {
		namespace = h.defaultNamespace
	}
	if namespace == "" {
		return nil, fmt.Errorf("helm namespace is required")
	}

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(
		h.settings.RESTClientGetter(),
		namespace,
		h.driver,
	); err != nil {
		return nil, fmt.Errorf("helm action config init failed: %w", err)
	}

	return actionConfig, nil
}

func (h *helmClient) FindRelease(releaseName string) (*ReleaseInfo, error) {
	actionConfig, err := h.newActionConfig("")
	if err != nil {
		return nil, err
	}

	listClient := action.NewList(actionConfig)
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
	actionConfig, err := h.newActionConfig(cfg.Namespace)
	if err != nil {
		return err
	}

	installClient := action.NewInstall(actionConfig)
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

func (h *helmClient) UpgradeChart(ctx context.Context, cfg ChartConfig, opts UpgradeChartOptions) error {
	actionConfig, err := h.newActionConfig(cfg.Namespace)
	if err != nil {
		return err
	}

	if opts.InstallIfNotExist {
		// If a release does not exist, install it.
		if _, err := h.FindRelease(cfg.ReleaseName); errors.Is(err, driver.ErrReleaseNotFound) {
			return h.InstallChart(ctx, cfg)
		}
	}

	upgradeClient := action.NewUpgrade(actionConfig)
	upgradeClient.Namespace = cfg.Namespace
	upgradeClient.WaitStrategy = "watcher"
	upgradeClient.Version = cfg.Version
	upgradeClient.RepoURL = cfg.RepoURL
	upgradeClient.Timeout = 5 * time.Minute
	upgradeClient.Install = opts.InstallIfNotExist

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
