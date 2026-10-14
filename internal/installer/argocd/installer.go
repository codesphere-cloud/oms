// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	k8s "github.com/codesphere-cloud/oms/internal/util"
	"github.com/distribution/reference"
	"helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/cli/values"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/kube"
	"k8s.io/client-go/rest"
)

const (
	DefaultRepoURL   = "https://argoproj.github.io/argo-helm"
	DefaultNamespace = "argocd"

	// mirrorChartName is the Codesphere argocd chart, a wrapper around the upstream argo-cd chart,
	// and the BOM component that pins it and lists its images.
	mirrorChartName = "argocd"
	// mirrorPullSecretName is the image pull secret ArgoCD's pods use with a mirrored registry.
	mirrorPullSecretName = "argocd-registry-pull"
)

// InstallerConfig holds all user-facing parameters for an ArgoCD install/upgrade.
type InstallerConfig struct {
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
	BOM               *bom.Config
	ValueFiles        []string
	RESTConfig        *rest.Config
}

// Installer holds the resolved configuration and initialized clients.
type Installer struct {
	InstallerConfig
	Helm      installer.HelmClient
	Resources ArgoCDResources
}

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

		resources, err := NewArgoCDResources(clientset, cfg.DatacenterID, cfg.OciUsername, cfg.OciPassword, cfg.OciRegistryURL, cfg.GitPassword)
		if err != nil {
			return nil, fmt.Errorf("init argocd resources client failed: %w", err)
		}
		return &Installer{
			InstallerConfig: cfg,
			Helm:            helm,
			Resources:       resources,
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

	resources, err := NewArgoCDResources(clientset, cfg.DatacenterID, cfg.OciUsername, cfg.OciPassword, cfg.OciRegistryURL, cfg.GitPassword)
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

	// A repository naming the registry the package was mirrored into serves the Codesphere chart
	// and its images instead of the upstream chart.
	chartName := "argo-cd"
	version := a.Version

	var mirrorValues map[string]any

	mirror := strings.TrimRight(strings.TrimPrefix(a.OciRegistryURL, "oci://"), "/")
	if a.BOM != nil && mirror != "" && strings.TrimRight(a.RepoURL, "/") == "oci://"+mirror {
		var err error

		chartName = mirrorChartName

		version, mirrorValues, err = a.mirrorChart()
		if err != nil {
			return err
		}

		log.Printf("Installing the Codesphere ArgoCD chart and its images from %s\n", a.RepoURL)
	}

	if version != "" {
		log.Printf("Installing/Upgrading ArgoCD helm chart version %s\n", version)
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

	trust, err := a.registryTrustValues()
	if err != nil {
		return err
	}

	defaults = util.MergeTables(defaults, trust)
	if mirrorValues != nil {
		// The Codesphere chart passes the values of the upstream chart on through its argo-cd
		// dependency.
		defaults = util.MergeTables(map[string]any{"argo-cd": defaults}, mirrorValues)
	}
	vals = util.MergeTables(vals, defaults)

	chartName, repoURL := a.resolveChartRef(chartName)
	cfg := installer.ChartConfig{
		ReleaseName:     "argocd",
		ChartName:       chartName,
		RepoURL:         repoURL,
		Namespace:       DefaultNamespace,
		Version:         version,
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

// mirrorChart returns the version of the Codesphere chart to install from the mirror in RepoURL,
// the one the BOM pins unless a version is given, and the values that point the chart at the
// mirror. The mirror requires a login for every pull, so the values also deliver the OCI
// credentials as an image pull secret with the chart.
func (a *Installer) mirrorChart() (string, map[string]any, error) {
	chart, ok := a.BOM.GetChart(mirrorChartName)
	if !ok {
		return "", nil, fmt.Errorf("BOM has no tagged %s chart to install from %s", mirrorChartName, a.RepoURL)
	}

	version := a.Version
	if version == "" {
		version = chart.Tag()
	}

	host, _, _ := strings.Cut(strings.TrimPrefix(a.RepoURL, "oci://"), "/")

	repositories, err := a.mirrorImageRepositories(host)
	if err != nil {
		return "", nil, err
	}

	dockerConfig, err := json.Marshal(map[string]any{
		"auths": map[string]any{
			host: map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte(a.OciUsername + ":" + a.OciPassword))},
		},
	})
	if err != nil {
		return "", nil, fmt.Errorf("encoding registry credentials: %w", err)
	}

	// The upstream chart renders its extra objects as templates, which the encoded credentials
	// pass through unchanged.
	pullSecret := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name": mirrorPullSecretName,
			// The chart's own pre-install hooks already run pods that pull from the mirror, so the
			// secret is created as a hook ahead of them and kept for the pods installed after.
			"annotations": map[string]any{
				"helm.sh/hook":               "pre-install,pre-upgrade",
				"helm.sh/hook-weight":        "-10",
				"helm.sh/hook-delete-policy": "before-hook-creation",
			},
		},
		"type": "kubernetes.io/dockerconfigjson",
		"data": map[string]any{".dockerconfigjson": base64.StdEncoding.EncodeToString(dockerConfig)},
	}

	argoCD := map[string]any{
		"global": map[string]any{
			"image":            map[string]any{"repository": repositories["argoproj/argocd"]},
			"imagePullSecrets": []any{map[string]any{"name": mirrorPullSecretName}},
		},
		"redis":        map[string]any{"image": map[string]any{"repository": repositories["redis"]}},
		"extraObjects": []any{pullSecret},
	}

	return version, map[string]any{
		// The Codesphere chart syncs the Dex client secret through an ExternalSecret, which needs
		// the external-secrets operator ArgoCD is installed before. Dex is disabled.
		"extraSecrets": []any{},
		"argo-cd":      argoCD,
	}, nil
}

// mirrorImageRepositories returns the mirror repositories of the images the Codesphere chart
// runs, keyed by the end of their upstream name, which the BOM keeps below whichever path the
// chart mirrors them to. They are named the way 'oms copy package' copied them below host. Both
// images are required.
func (a *Installer) mirrorImageRepositories(host string) (map[string]string, error) {
	names := []string{"argoproj/argocd", "redis"}
	repositories := map[string]string{}

	for _, image := range a.BOM.Components[mirrorChartName].ContainerImages {
		named, err := reference.ParseNormalizedNamed(image)
		if err != nil {
			return nil, fmt.Errorf("parsing %s image %q from the BOM: %w", mirrorChartName, image, err)
		}

		for _, name := range names {
			if !strings.HasSuffix("/"+reference.Path(named), "/"+name) {
				continue
			}

			destination, err := installer.PackageImageDestination(image, host)
			if err != nil {
				return nil, fmt.Errorf("mapping %s image %q onto %s: %w", mirrorChartName, image, host, err)
			}

			mirrored, err := reference.ParseNormalizedNamed(destination)
			if err != nil {
				return nil, fmt.Errorf("parsing mirrored image %q: %w", destination, err)
			}

			repositories[name] = mirrored.Name()
		}
	}

	for _, name := range names {
		if _, ok := repositories[name]; !ok {
			return nil, fmt.Errorf("BOM lists no %s image for the %s chart", name, mirrorChartName)
		}
	}

	return repositories, nil
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

	if cfg.Version != "" {
		installedSemver, err := semver.NewVersion(existing.InstalledVersion)
		if err != nil {
			return fmt.Errorf("failed to parse installed version %q: %w", existing.InstalledVersion, err)
		}

		requestedSemver, err := semver.NewVersion(cfg.Version)
		if err != nil {
			return fmt.Errorf("failed to parse requested version %q: %w", cfg.Version, err)
		}

		if requestedSemver.LessThan(installedSemver) {
			return fmt.Errorf(
				"requested version %s is older than installed version %s; downgrade is not allowed",
				cfg.Version, existing.InstalledVersion,
			)
		}

		log.Printf("Upgrading ArgoCD from %s to %s\n", existing.InstalledVersion, cfg.Version)
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
