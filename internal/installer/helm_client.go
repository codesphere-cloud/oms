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
	"helm.sh/helm/v4/pkg/registry"
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
	InstallIfNotExist bool
}

// HelmClient is the seam that makes the Helm SDK mockable.
// Every method receives only plain Go types so that test doubles never need to
// import any helm.sh package.
//
//mockery:generate: true
type HelmClient interface {
	// FindRelease returns info about an existing release, or nil if none exists.
	FindRelease(namespace, releaseName string) (*ReleaseInfo, error)

	// InstallChart performs a fresh Helm install and returns an error on failure.
	InstallChart(ctx context.Context, cfg ChartConfig) error

	// UpgradeChart upgrades an existing Helm release and returns an error on failure.
	UpgradeChart(ctx context.Context, cfg ChartConfig, opts UpgradeChartOptions) error

	// LoginRegistry authenticates against an OCI registry for private chart pulls.
	LoginRegistry(ctx context.Context, host, username, password string) error
}

// ---------------------------------------------------------------------------
// Concrete implementation backed by the Helm Go SDK v4
// ---------------------------------------------------------------------------

type helmClient struct {
	defaultNamespace string
	driver           string
	registryClient   *registry.Client
}

func NewHelmClient(namespace string) (HelmClient, error) {
	return &helmClient{
		defaultNamespace: namespace,
		driver:           os.Getenv("HELM_DRIVER"),
	}, nil
}

// LoginRegistry authenticates against an OCI registry. The context parameter
// is accepted for interface consistency but is not used by the underlying
// Helm registry client.
func (h *helmClient) LoginRegistry(_ context.Context, host, username, password string) error {
	registryClient, err := registry.NewClient()
	if err != nil {
		return fmt.Errorf("creating registry client: %w", err)
	}

	if err := registryClient.Login(host, registry.LoginOptBasicAuth(username, password)); err != nil {
		return fmt.Errorf("registry login to %q failed: %w", host, err)
	}

	h.registryClient = registryClient
	return nil
}

// helmEnv holds the per-call Helm action configuration and CLI settings.
// Both are scoped to the same namespace so that the RESTClientGetter, Helm
// storage driver, and LocateChart all operate in the intended namespace.
type helmEnv struct {
	actionConfig *action.Configuration
	settings     *cli.EnvSettings
}

func (h *helmClient) newHelmEnv(namespace string) (*helmEnv, error) {
	if namespace == "" {
		namespace = h.defaultNamespace
	}
	if namespace == "" {
		return nil, fmt.Errorf("helm namespace is required")
	}

	// Create per-call settings so the RESTClientGetter uses the correct
	// namespace for Kubernetes operations (resource deployment), not just
	// Helm storage. Without this, resources land in the kubeconfig context's
	// default namespace instead of the requested one.
	settings := cli.New()
	settings.SetNamespace(namespace)

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(
		settings.RESTClientGetter(),
		namespace,
		h.driver,
	); err != nil {
		return nil, fmt.Errorf("helm action config init failed: %w", err)
	}

	if h.registryClient != nil {
		// Reuse the registry client that was authenticated during LoginRegistry.
		actionConfig.RegistryClient = h.registryClient
	} else {
		registryClient, err := registry.NewClient()
		if err != nil {
			return nil, fmt.Errorf("helm registry client init failed: %w", err)
		}
		actionConfig.RegistryClient = registryClient
	}

	return &helmEnv{actionConfig: actionConfig, settings: settings}, nil
}

func (h *helmClient) FindRelease(namespace, releaseName string) (*ReleaseInfo, error) {
	env, err := h.newHelmEnv(namespace)
	if err != nil {
		return nil, err
	}

	listClient := action.NewList(env.actionConfig)
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
	env, err := h.newHelmEnv(cfg.Namespace)
	if err != nil {
		return err
	}

	installClient := action.NewInstall(env.actionConfig)
	installClient.ReleaseName = cfg.ReleaseName
	installClient.Namespace = cfg.Namespace
	installClient.CreateNamespace = cfg.CreateNamespace
	installClient.DryRunStrategy = "none"
	installClient.WaitStrategy = "watcher"
	installClient.Version = cfg.Version
	installClient.RepoURL = cfg.RepoURL
	installClient.Timeout = 5 * time.Minute

	chartPath, err := installClient.LocateChart(cfg.ChartName, env.settings)
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
	if opts.InstallIfNotExist {
		rel, err := h.FindRelease(cfg.Namespace, cfg.ReleaseName)
		if err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
			return err
		}
		if rel == nil {
			return h.InstallChart(ctx, cfg)
		}
	}

	env, err := h.newHelmEnv(cfg.Namespace)
	if err != nil {
		return err
	}

	upgradeClient := action.NewUpgrade(env.actionConfig)
	upgradeClient.Namespace = cfg.Namespace
	upgradeClient.WaitStrategy = "watcher"
	upgradeClient.Version = cfg.Version
	upgradeClient.RepoURL = cfg.RepoURL
	upgradeClient.Timeout = 5 * time.Minute

	chartPath, err := upgradeClient.LocateChart(cfg.ChartName, env.settings)
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
