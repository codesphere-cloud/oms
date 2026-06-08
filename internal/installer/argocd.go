// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Masterminds/semver/v3"
	"helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/cli/values"
	"helm.sh/helm/v4/pkg/getter"
)

const argoCDDefaultRepoURL = "https://argoproj.github.io/argo-helm"

// ArgoCD holds the user-facing configuration for the install/upgrade command.
type ArgoCD struct {
	Version        string
	DatacenterId   string
	OciPassword    string
	OciRegistryURL string
	GitPassword    string
	FullInstall    bool
	ForceConflicts bool
	RepoURL        string // defaults to argoCDDefaultRepoURL if empty
	ValueFiles     []string
	Helm           HelmClient // inject a real or mock client
	Resources      ArgoCDResources
}

func NewArgoCD(version string, dcId string, passwordOCI string, ociRegistryURL string, passwordGit string, fullInstall bool, forceConflicts bool, repoURL string, valueFiles []string) (*ArgoCD, error) {
	helm, err := NewHelmClient("argocd")
	if err != nil {
		return nil, fmt.Errorf("init helm client failed: %w", err)
	}

	resources, err := NewArgoCDResources(dcId, passwordOCI, ociRegistryURL, passwordGit)
	if err != nil {
		return nil, fmt.Errorf("init argocd resources client failed: %w", err)
	}
	return &ArgoCD{
		Version:        version,
		DatacenterId:   dcId,
		OciPassword:    passwordOCI,
		OciRegistryURL: ociRegistryURL,
		GitPassword:    passwordGit,
		FullInstall:    fullInstall,
		ForceConflicts: forceConflicts,
		RepoURL:        repoURL,
		ValueFiles:     valueFiles,
		Helm:           helm,
		Resources:      resources,
	}, nil
}

// Install is the top-level orchestrator. It delegates every Helm interaction
// to the HelmClient interface, keeping this function short and testable.
func (a *ArgoCD) Install() error {
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
	}).MergeValues(getter.All(cli.New()))
	if err != nil {
		return fmt.Errorf("loading values files: %w", err)
	}

	// Apply our defaults underneath the user-provided values so value files can
	// override them. MergeTables gives precedence to dst (the user values).
	defaults := map[string]any{
		"dex": map[string]any{"enabled": false},
	}
	vals = util.MergeTables(vals, defaults)

	chartName, repoURL := a.resolveChartRef("argo-cd")
	cfg := ChartConfig{
		ReleaseName:     "argocd",
		ChartName:       chartName,
		RepoURL:         repoURL,
		Namespace:       "argocd",
		Version:         a.Version,
		CreateNamespace: true,
		Values:          vals,
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

// install performs a fresh Helm install.
func (a *ArgoCD) install(ctx context.Context, cfg ChartConfig) error {
	log.Println("No existing ArgoCD release found, performing fresh install")

	if err := a.Helm.InstallChart(ctx, cfg, InstallChartOptions{ForceConflicts: a.ForceConflicts}); err != nil {
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
func (a *ArgoCD) upgrade(ctx context.Context, cfg ChartConfig, existing *ReleaseInfo) error {
	log.Printf("Found existing ArgoCD release with chart version %s\n", existing.InstalledVersion)

	// Prevent downgrades when a specific version is requested
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

	if err := a.Helm.UpgradeChart(ctx, cfg, UpgradeChartOptions{ForceConflicts: a.ForceConflicts}); err != nil {
		return err
	}

	if cfg.Version != "" {
		fmt.Printf("Successfully upgraded Argo CD to chart version %s\n", cfg.Version)
	} else {
		fmt.Println("Successfully upgraded Argo CD to the latest chart version")
	}
	return nil
}

// validateRepoURL ensures a non-empty RepoURL uses a supported scheme.
func (a *ArgoCD) validateRepoURL() error {
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

// resolveChartRef returns the (chartName, repoURL) pair to use in ChartConfig.
// For OCI repos the full reference is passed as chartName and repoURL is empty,
// because helm's LocateChart expects "oci://<registry>/<repo>/<chart>" as the
// chart name with no separate RepoURL.
func (a *ArgoCD) resolveChartRef(chartName string) (string, string) {
	repoURL := a.RepoURL
	if repoURL == "" {
		repoURL = argoCDDefaultRepoURL
	}
	if strings.HasPrefix(repoURL, "oci://") {
		return strings.TrimRight(repoURL, "/") + "/" + chartName, ""
	}
	return chartName, repoURL
}

func (a *ArgoCD) showPostInstallHints() {
	log.Println(`To get ArgoCD admin password:`)
	log.Println(`  kubectl get secrets/argocd-initial-admin-secret -nargocd -ojson | jq -r ".data.password" | base64 -d`)
	log.Println(`To port-forward ArgoCD UI to localhost:8080:`)
	log.Println(`  kubectl port-forward svc/argocd-server 8080:80 -nargocd`)
}
