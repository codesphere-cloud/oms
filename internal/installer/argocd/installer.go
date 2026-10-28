// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package argocd installs and upgrades ArgoCD via Helm and sets up the
// resources it needs in the cluster, such as registry and repository credentials.
package argocd

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/codesphere-cloud/oms/internal/installer"
	k8s "github.com/codesphere-cloud/oms/internal/util"
	"helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/kube"
	"k8s.io/client-go/rest"
)

// Defaults for the ArgoCD Helm chart repository and the namespace ArgoCD is installed into.
const (
	DefaultRepoURL   = "https://argoproj.github.io/argo-helm"
	DefaultNamespace = "argocd"
)

// InstallerConfig holds all user-facing parameters for an ArgoCD install/upgrade.
type InstallerConfig struct {
	// PackageChart selects the extracted Codesphere chart instead of upstream.
	PackageChart   string
	ImageRegistry  string
	Version        string
	DatacenterID   string
	OciUsername    string
	OciPassword    string
	OciRegistryURL string
	// OciRegistryCAFile is the certificate authority of the OCI registry, needed when its
	// certificate is not signed by a public authority.
	OciRegistryCAFile string
	GitPassword       string
	FullInstall       bool
	ForceConflicts    bool
	RepoURL           string
	ValueFiles        []string
	RESTConfig        *rest.Config
}

// Installer holds the resolved configuration and initialized clients.
type Installer struct {
	InstallerConfig
	Helm          installer.HelmClient
	Resources     Resources
	PackageLoader PackagedChartLoader
}

// NewInstaller creates an Installer with Helm and Kubernetes clients. It uses cfg.RESTConfig
// when set and falls back to the default kubeconfig otherwise.
func NewInstaller(cfg InstallerConfig) (*Installer, error) {
	if cfg.RESTConfig != nil {
		helm, err := installer.NewHelmClientWithRESTConfig(DefaultNamespace, cfg.RESTConfig)
		if err != nil {
			return nil, fmt.Errorf("init helm client failed: %w", err)
		}

		clientset, _, err := k8s.NewClientsFromRESTConfig(cfg.RESTConfig)
		if err != nil {
			return nil, fmt.Errorf("creating kubernetes clients: %w", err)
		}

		resources, err := NewResources(clientset, cfg.DatacenterID, cfg.OciUsername, cfg.OciPassword, cfg.OciRegistryURL, cfg.GitPassword)
		if err != nil {
			return nil, fmt.Errorf("init argocd resources client failed: %w", err)
		}

		return &Installer{
			InstallerConfig: cfg,
			Helm:            helm,
			Resources:       resources,
			PackageLoader:   packagedChartLoader{},
		}, nil
	}

	helm, err := installer.NewHelmClient(DefaultNamespace)
	if err != nil {
		return nil, fmt.Errorf("init helm client failed: %w", err)
	}

	clientset, _, err := k8s.NewClients()
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes clients: %w", err)
	}

	resources, err := NewResources(clientset, cfg.DatacenterID, cfg.OciUsername, cfg.OciPassword, cfg.OciRegistryURL, cfg.GitPassword)
	if err != nil {
		return nil, fmt.Errorf("init argocd resources client failed: %w", err)
	}

	return &Installer{
		InstallerConfig: cfg,
		Helm:            helm,
		Resources:       resources,
		PackageLoader:   packagedChartLoader{},
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

	trust, err := a.registryTrustValues()
	if err != nil {
		return err
	}

	defaults := util.MergeTables(map[string]any{"dex": map[string]any{"enabled": false}}, trust)

	existing, err := a.Helm.FindRelease(DefaultNamespace, "argocd")
	if err != nil {
		return fmt.Errorf("finding existing ArgoCD release: %w", err)
	}

	if err := a.reconcileChart(ctx, existing, defaults); err != nil {
		return err
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

// reconcileChart shares Helm operations between upstream and packaged charts.
func (a *Installer) reconcileChart(ctx context.Context, existing *installer.ReleaseInfo, defaults map[string]any) error {
	if a.PackageChart != "" {
		manage, err := a.shouldManagePackagedChart(existing)
		if err != nil {
			return err
		}

		if !manage {
			return nil
		}
	}

	cfg, err := a.chartConfig(defaults)
	if err != nil {
		return err
	}

	if existing == nil {
		return a.install(ctx, cfg)
	}

	return a.upgrade(ctx, cfg, existing)
}

func (a *Installer) chartLoader() PackagedChartLoader {
	if a.PackageLoader != nil {
		return a.PackageLoader
	}

	return packagedChartLoader{}
}

func (a *Installer) chartConfig(defaults map[string]any) (installer.ChartConfig, error) {
	chartName, repoURL := a.resolveChartRef("argo-cd")

	cfg := installer.ChartConfig{
		ReleaseName: "argocd", ChartName: chartName, RepoURL: repoURL,
		Namespace: DefaultNamespace, Version: a.Version, CreateNamespace: true,
		WaitStrategy: kube.LegacyStrategy,
	}
	if a.PackageChart != "" {
		var err error

		defaults, err = a.packagedDefaults(defaults)
		if err != nil {
			return cfg, err
		}

		cfg.ChartName, cfg.RepoURL = a.PackageChart, ""
	}

	overrides, err := a.chartLoader().LoadValues(a.ValueFiles)
	if err != nil {
		return cfg, fmt.Errorf("loading ArgoCD values: %w", err)
	}

	cfg.Values = util.MergeTables(overrides, defaults)

	return cfg, nil
}

// registryTrustValues puts the OCI registry's certificate authority into ArgoCD's certificate
// store. The repo server runs in a container and reads that store instead of the trust store of
// the node it runs on, so it rejects a registry with a self-signed certificate until the
// authority is configured here. The store is keyed by host name without its port.
func (a *Installer) registryTrustValues() (map[string]any, error) {
	if a.OciRegistryCAFile == "" {
		return nil, nil
	}

	authority, err := os.ReadFile(a.OciRegistryCAFile)
	if err != nil {
		return nil, fmt.Errorf("reading registry certificate authority: %w", err)
	}

	// A registry URL may name its scheme, which is neither part of the host nor recognised as
	// one: splitting "oci:" into host and port succeeds and yields the scheme itself.
	registryURL := a.OciRegistryURL
	if _, rest, found := strings.Cut(registryURL, "://"); found {
		registryURL = rest
	}

	address, _, _ := strings.Cut(registryURL, "/")

	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}

	if host == "" {
		return nil, fmt.Errorf("registry URL %q has no host to trust the certificate authority for", a.OciRegistryURL)
	}

	return map[string]any{
		"configs": map[string]any{
			"tls": map[string]any{
				"certificates": map[string]any{host: string(authority)},
			},
		},
	}, nil
}

func (a *Installer) install(ctx context.Context, cfg installer.ChartConfig) error {
	log.Println("No existing ArgoCD release found, performing fresh install")

	if err := a.Helm.InstallChart(ctx, cfg, installer.InstallChartOptions{ForceConflicts: a.ForceConflicts}); err != nil {
		return fmt.Errorf("installing ArgoCD chart: %w", err)
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
		comparison, err := a.compareChartVersion(existing.InstalledVersion)
		if err != nil {
			return err
		}

		if comparison < 0 {
			return fmt.Errorf("requested version %s is older than installed version %s; downgrade is not allowed", a.Version, existing.InstalledVersion)
		}

		log.Printf("Upgrading ArgoCD from %s to %s\n", existing.InstalledVersion, a.Version)
	} else {
		log.Printf("Upgrading ArgoCD from %s to latest\n", existing.InstalledVersion)
	}

	if err := a.Helm.UpgradeChart(ctx, cfg, installer.UpgradeChartOptions{ForceConflicts: a.ForceConflicts}); err != nil {
		return fmt.Errorf("upgrading ArgoCD chart: %w", err)
	}

	if cfg.Version != "" {
		fmt.Printf("Successfully upgraded Argo CD to chart version %s\n", cfg.Version)
	} else {
		fmt.Println("Successfully upgraded Argo CD to the latest chart version")
	}

	return nil
}

func (a *Installer) compareChartVersion(installedVersion string) (int, error) {
	installed, err := semver.NewVersion(installedVersion)
	if err != nil {
		return 0, fmt.Errorf("failed to parse installed version %q: %w", installedVersion, err)
	}

	requested, err := semver.NewVersion(a.Version)
	if err != nil {
		return 0, fmt.Errorf("failed to parse requested version %q: %w", a.Version, err)
	}

	return requested.Compare(installed), nil
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
