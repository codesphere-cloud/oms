package installer

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Masterminds/semver/v3"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/cli"
)

// ArgoCD holds the user-facing configuration for the install/upgrade command.
type ArgoCD struct {
	Version      string
	DatacenterId string
	OciPassword  string
	GitPassword  string
	FullInstall  bool
	Helm         HelmClient // inject a real or mock client
	Resources    ArgoCDResources
}

func NewArgoCD(version string, dcId string, passwordOCI string, passwordGit string, fullInstall bool) (*ArgoCD, error) {
	settings := cli.New()
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), "argocd", os.Getenv("HELM_DRIVER")); err != nil {
		return nil, fmt.Errorf("init helm client failed: %w", err)
	}
	helm, err := NewHelmClient("argocd")
	if err != nil {
		log.Fatal(err)
	}

	resources, err := NewArgoCDResources(dcId, passwordOCI, passwordGit)
	if err != nil {
		return nil, fmt.Errorf("init argocd resources client failed: %v", err)
	}
	return &ArgoCD{
		Version:      version,
		DatacenterId: dcId,
		OciPassword:  passwordOCI,
		GitPassword:  passwordGit,
		FullInstall:  fullInstall,
		Helm:         helm,
		Resources:    resources,
	}, nil
}

// Install is the top-level orchestrator. It delegates every Helm interaction
// to the HelmClient interface, keeping this function short and testable.
func (a *ArgoCD) Install() error {
	if a.Version != "" {
		log.Printf("Installing/Upgrading ArgoCD helm chart version %s\n", a.Version)
	} else {
		log.Println("Installing/Upgrading ArgoCD helm chart (latest version)")
	}

	ctx := context.Background()

	cfg := ChartConfig{
		ReleaseName:     "argocd",
		ChartName:       "argo-cd",
		RepoURL:         "https://argoproj.github.io/argo-helm",
		Namespace:       "argocd",
		Version:         a.Version,
		CreateNamespace: true,
		Values: map[string]interface{}{
			"dex": map[string]interface{}{
				"enabled": false,
			},
		},
	}

	// 1. Find existing release
	existing, err := a.Helm.FindRelease(cfg.ReleaseName)
	if err != nil {
		return err
	}

	if existing != nil {
		// 2a. Upgrade path
		if err := a.upgrade(ctx, cfg, existing); err != nil {
			return err
		}
	} else {
		// 2b. Fresh install path
		if err := a.install(ctx, cfg); err != nil {
			return err
		}
	}

	// 3. Optional post-install resources
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

	if err := a.Helm.InstallChart(ctx, cfg); err != nil {
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

	if err := a.Helm.UpgradeChart(ctx, cfg); err != nil {
		return err
	}

	if cfg.Version != "" {
		fmt.Printf("Successfully upgraded Argo CD to chart version %s\n", cfg.Version)
	} else {
		fmt.Println("Successfully upgraded Argo CD to the latest chart version")
	}
	return nil
}

func (a *ArgoCD) showPostInstallHints() {
	log.Println(`To get ArgoCD admin password:`)
	log.Println(`  kubectl get secrets/argocd-initial-admin-secret -nargocd -ojson | jq -r ".data.password" | base64 -d`)
	log.Println(`To port-forward ArgoCD UI to localhost:8080:`)
	log.Println(`  kubectl port-forward svc/argocd-server 8080:80 -nargocd`)
}
