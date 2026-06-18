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
	"helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/cli/values"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/kube"
	"k8s.io/client-go/rest"
)

const (
	DefaultRepoURL   = "https://argoproj.github.io/argo-helm"
	DefaultNamespace = "argocd"
)

// InstallerConfig holds all user-facing parameters for an ArgoCD install/upgrade.
type InstallerConfig struct {
	Version        string
	DatacenterId   string
	OciPassword    string
	OciRegistryURL string
	GitPassword    string
	FullInstall    bool
	ForceConflicts bool
	RepoURL        string
	ValueFiles     []string
	RESTConfig     *rest.Config
}

// Installer holds the resolved configuration and initialized clients.
type Installer struct {
	InstallerConfig
	Helm      installer.HelmClient
	Resources ArgoCDResources
}

func NewInstaller(cfg InstallerConfig) (*Installer, error) {
	helm, err := installer.NewHelmClientWithRESTConfig("argocd", cfg.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("init helm client failed: %w", err)
	}

	resources, err := NewArgoCDResourcesWithRESTConfig(cfg.DatacenterId, cfg.OciPassword, cfg.OciRegistryURL, cfg.GitPassword, cfg.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("init argocd resources client failed: %w", err)
	}

	return &Installer{
		InstallerConfig: cfg,
		Helm:            helm,
		Resources:       resources,
	}, nil
}

// Install is the top-level orchestrator. It delegates every Helm interaction
// to the HelmClient interface, keeping this function short and testable.
func (a *Installer) Install() error {
	if err := a.validateRepoURL(); err != nil {
		return err
	}

	if a.Version != "" {
		log.Printf("Installing/Upgrading ArgoCD helm chart version %s\n", a.Version)
	} else {
		log.Println("Installing/Upgrading ArgoCD helm chart (latest version)")
	}

	ctx := context.Background()

	vals, err := (&values.Options{
		ValueFiles: a.ValueFiles,
	}).MergeValues(getter.Getters())
	if err != nil {
		return fmt.Errorf("loading values files: %w", err)
	}

	defaults := map[string]any{
		"dex": map[string]any{"enabled": false},
	}
	vals = util.MergeTables(vals, defaults)

	chartName, repoURL := a.resolveChartRef("argo-cd")
	cfg := installer.ChartConfig{
		ReleaseName:     "argocd",
		ChartName:       chartName,
		RepoURL:         repoURL,
		Namespace:       DefaultNamespace,
		Version:         a.Version,
		CreateNamespace: true,
		Values:          vals,
		WaitStrategy:    kube.LegacyStrategy, // StatusWatcherStrategy causes issues with ArgoCD's post-install hooks
	}

	existing, err := a.Helm.FindRelease(cfg.Namespace, cfg.ReleaseName)
	if err != nil {
		return err
	}

	if existing != nil {
		if err := a.upgrade(ctx, cfg, existing); err != nil {
			return err
		}
	} else {
		if err := a.install(ctx, cfg); err != nil {
			return err
		}
	}

	if a.FullInstall {
		err = a.Resources.ApplyAll(ctx)
		if err != nil {
			return fmt.Errorf("failed apply post chart install resources: %v", err)
		}
	}

	a.showPostInstallHints()

	return nil
}

func (a *Installer) install(ctx context.Context, cfg installer.ChartConfig) error {
	log.Println("No existing ArgoCD release found, performing fresh install")

	if err := a.Helm.InstallChart(ctx, cfg, installer.InstallChartOptions{ForceConflicts: a.ForceConflicts}); err != nil {
		return err
	}

	if cfg.Version != "" {
		fmt.Printf("Successfully installed Argo CD (chart version: %s)\n", cfg.Version)
	} else {
		fmt.Println("Successfully installed Argo CD (latest chart version)")
	}
	return nil
}

// upgrade validates the version constraint and then performs a Helm upgrade.
func (a *Installer) upgrade(ctx context.Context, cfg installer.ChartConfig, existing *installer.ReleaseInfo) error {
	log.Printf("Found existing ArgoCD release with chart version %s\n", existing.InstalledVersion)

	if a.Version != "" {
		installedSemver, err := semver.NewVersion(existing.InstalledVersion)
		if err != nil {
			return fmt.Errorf("failed to parse installed version %q: %w", existing.InstalledVersion, err)
		}
		requestedSemver, err := semver.NewVersion(a.Version)
		if err != nil {
			return fmt.Errorf("failed to parse requested version %q: %w", a.Version, err)
		}

		if requestedSemver.LessThan(installedSemver) {
			return fmt.Errorf(
				"requested version %s is older than installed version %s; downgrade is not allowed",
				a.Version, existing.InstalledVersion,
			)
		}
		log.Printf("Upgrading ArgoCD from %s to %s\n", existing.InstalledVersion, a.Version)
	} else {
		log.Printf("Upgrading ArgoCD from %s to latest\n", existing.InstalledVersion)
	}

	if err := a.Helm.UpgradeChart(ctx, cfg, installer.UpgradeChartOptions{ForceConflicts: a.ForceConflicts}); err != nil {
		return err
	}

	if cfg.Version != "" {
		fmt.Printf("Successfully upgraded Argo CD to chart version %s\n", cfg.Version)
	} else {
		fmt.Println("Successfully upgraded Argo CD to the latest chart version")
	}
	return nil
}

func (a *Installer) validateRepoURL() error {
	if a.RepoURL == "" {
		return nil
	}
	for _, prefix := range []string{"http://", "https://", "oci://"} {
		if strings.HasPrefix(a.RepoURL, prefix) {
			return nil
		}
	}
	return fmt.Errorf("invalid repo URL %q: must start with http://, https://, or oci://", a.RepoURL)
}

func (a *Installer) resolveChartRef(chartName string) (string, string) {
	repoURL := a.RepoURL
	if repoURL == "" {
		repoURL = DefaultRepoURL
	}
	if strings.HasPrefix(repoURL, "oci://") {
		return strings.TrimRight(repoURL, "/") + "/" + chartName, ""
	}
	return chartName, repoURL
}

func (a *Installer) showPostInstallHints() {
	log.Println(`To get ArgoCD admin password:`)
	log.Println(`  kubectl get secrets/argocd-initial-admin-secret -nargocd -ojson | jq -r ".data.password" | base64 -d`)
	log.Println(`To port-forward ArgoCD UI to localhost:8080:`)
	log.Println(`  kubectl port-forward svc/argocd-server 8080:80 -nargocd`)
}
