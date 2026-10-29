// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package argocd installs and upgrades ArgoCD via Helm and sets up the
// resources it needs in the cluster, such as registry and repository credentials.
package argocd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	k8s "github.com/codesphere-cloud/oms/internal/util"
	"helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DefaultNamespace is the namespace ArgoCD is installed into.
const DefaultNamespace = "argocd"

// DefaultRepoURL is used only for an explicit upstream version override.
const DefaultRepoURL = "https://argoproj.github.io/argo-helm"

const imagePullSecretName = "docker-regcred"

// PackagedChartName identifies the Codesphere wrapper chart and its BOM component.
const PackagedChartName = "argocd"

// UpstreamChartName identifies the upstream chart and its wrapper values key.
const UpstreamChartName = "argo-cd"

// InstallerConfig holds all user-facing parameters for an ArgoCD install/upgrade.
type InstallerConfig struct {
	PackageChart   string
	ImageRegistry  string
	UseUpstream    bool
	ChartName      string
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

// ConfigFromBOM resolves the ArgoCD chart name, repository, and version from the
// installer BOM, using the configured chart registry for mirrored installations.
// The chart name and version always come from the BOM.
func ConfigFromBOM(bomConfig *bom.Config, cfg InstallerConfig) (InstallerConfig, error) {
	if bomConfig == nil {
		return InstallerConfig{}, fmt.Errorf("installer BOM is required")
	}

	chart, ok := bomConfig.GetChart(PackagedChartName)
	if !ok {
		return InstallerConfig{}, fmt.Errorf("argocd chart with a valid tagged OCI reference not found in installer BOM")
	}

	cfg.ChartName = path.Base(chart.Name())

	if cfg.OciRegistryURL == "" {
		cfg.OciRegistryURL = path.Dir(chart.Name())
	}

	cfg.RepoURL = "oci://" + strings.TrimPrefix(cfg.OciRegistryURL, "oci://")

	cfg.Version = chart.Tag()

	return cfg, nil
}

// Installer holds the resolved configuration and initialized clients.
type Installer struct {
	InstallerConfig
	Helm        installer.HelmClient
	Resources   Resources
	Client      client.Client
	ChartLoader installer.ChartLoader
}

// NewInstaller resolves the required ArgoCD chart from the BOM before creating clients.
// It uses cfg.RESTConfig when set and falls back to the default kubeconfig otherwise.
func NewInstaller(cfg InstallerConfig, bomConfig *bom.Config) (*Installer, error) {
	var err error
	if !cfg.UseUpstream {
		cfg, err = ConfigFromBOM(bomConfig, cfg)
		if err != nil {
			return nil, err
		}
	}

	restConfig := cfg.RESTConfig
	if restConfig == nil {
		restConfig, err = ctrlconfig.GetConfig()
		if err != nil {
			return nil, fmt.Errorf("loading kubernetes config: %w", err)
		}
	}

	helm, err := installer.NewHelmClientWithRESTConfig(DefaultNamespace, restConfig)
	if err != nil {
		return nil, fmt.Errorf("init helm client failed: %w", err)
	}

	clientset, _, err := k8s.NewClientsFromRESTConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes clients: %w", err)
	}

	kubeClient, err := installer.NewKubeClient(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	resources, err := NewResources(clientset, cfg.DatacenterID, cfg.OciUsername, cfg.OciPassword, cfg.OciRegistryURL, cfg.GitPassword)
	if err != nil {
		return nil, fmt.Errorf("init argocd resources client failed: %w", err)
	}

	return &Installer{
		InstallerConfig: cfg,
		Helm:            helm,
		Resources:       resources,
		Client:          kubeClient,
		ChartLoader:     installer.NewChartLoader(),
	}, nil
}

// Install is the top-level orchestrator. It delegates every Helm interaction
// to the HelmClient interface, keeping this function short and testable.
func (a *Installer) Install() error {
	if err := installer.ValidateChartRepositoryURL(a.RepoURL); err != nil {
		return fmt.Errorf("validating ArgoCD chart repository: %w", err)
	}

	if a.Version != "" {
		log.Printf("Installing/Upgrading ArgoCD helm chart version %s\n", a.Version)
	} else {
		log.Println("Installing/Upgrading ArgoCD helm chart (latest version)")
	}

	ctx := context.Background()

	vals, err := a.chartLoader().LoadValues(a.ValueFiles)
	if err != nil {
		return fmt.Errorf("loading values files: %w", err)
	}

	defaults := map[string]any{
		"dex": map[string]any{"enabled": false},
	}

	trust, err := a.configureRegistryCA()
	if err != nil {
		return err
	}

	defaults = util.MergeTables(defaults, trust)

	pullSecret, err := a.imagePullSecret()
	if err != nil {
		return err
	}

	if pullSecret != nil {
		defaults = util.MergeTables(defaults, map[string]any{
			"global": map[string]any{"imagePullSecrets": []any{map[string]any{"name": pullSecret.Name}}},
		})
	}

	defaults, err = a.chartDefaults(defaults)
	if err != nil {
		return err
	}

	vals = util.MergeTables(vals, defaults)

	chartName := a.ChartName
	if chartName == "" {
		chartName = UpstreamChartName
	}

	chartName, repoURL := installer.ResolveChartReference(a.RepoURL, chartName)
	if a.PackageChart != "" {
		chartName, repoURL = a.PackageChart, ""
	}

	if err := installer.LoginChartRegistry(ctx, a.Helm, chartName, ociUsernameOrDefault(a.OciUsername), a.OciPassword); err != nil {
		return fmt.Errorf("logging in to ArgoCD chart registry: %w", err)
	}
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
		return fmt.Errorf("finding existing ArgoCD release: %w", err)
	}

	if err := a.reconcileChart(ctx, cfg, existing, pullSecret); err != nil {
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

func (a *Installer) chartLoader() installer.ChartLoader {
	if a.ChartLoader != nil {
		return a.ChartLoader
	}

	return installer.NewChartLoader()
}

func (a *Installer) chartDefaults(defaults map[string]any) (map[string]any, error) {
	if a.PackageChart != "" {
		return a.packagedDefaults(defaults)
	}

	if a.UseUpstream {
		return defaults, nil
	}

	return map[string]any{UpstreamChartName: defaults}, nil
}

func (a *Installer) reconcileChart(ctx context.Context, cfg installer.ChartConfig, existing *installer.ReleaseInfo, pullSecret *corev1.Secret) error {
	if a.PackageChart != "" {
		manage, err := a.shouldManagePackagedChart(existing)
		if err != nil {
			return err
		}

		if !manage {
			return a.refreshRetainedPullSecret(ctx, existing, pullSecret)
		}
	}

	if existing != nil {
		if err := a.validateUpgradeVersion(cfg, existing); err != nil {
			return err
		}
	}

	if pullSecret != nil {
		if err := a.provisionImagePullSecret(ctx, pullSecret); err != nil {
			return fmt.Errorf("provisioning ArgoCD image pull secret: %w", err)
		}
	}

	if existing == nil {
		return a.install(ctx, cfg)
	}

	return a.upgrade(ctx, cfg, existing)
}

// Retained OMS-managed releases still need current image credentials. Externally
// managed releases keep their own pull-secret configuration and ownership.
func (a *Installer) refreshRetainedPullSecret(ctx context.Context, existing *installer.ReleaseInfo, pullSecret *corev1.Secret) error {
	if existing.ChartName != PackagedChartName || pullSecret == nil {
		return nil
	}

	if err := a.provisionImagePullSecret(ctx, pullSecret); err != nil {
		return fmt.Errorf("refreshing retained ArgoCD image pull secret: %w", err)
	}
	return nil
}

// provisionImagePullSecret ensures credentials exist before Helm hooks need them.
func (a *Installer) provisionImagePullSecret(ctx context.Context, desired *corev1.Secret) error {
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: desired.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, a.Client, namespace, func() error { return nil }); err != nil {
		return fmt.Errorf("ensuring ArgoCD namespace: %w", err)
	}

	secret := &corev1.Secret{ObjectMeta: desired.ObjectMeta}

	_, err := controllerutil.CreateOrUpdate(ctx, a.Client, secret, func() error {
		secret.Type = desired.Type
		secret.Data = desired.Data
		// Retire the old Helm hook while preserving other metadata.
		delete(secret.Annotations, "helm.sh/hook")
		delete(secret.Annotations, "helm.sh/hook-weight")
		delete(secret.Annotations, "helm.sh/hook-delete-policy")

		return nil
	})
	if err != nil {
		return fmt.Errorf("applying image pull secret: %w", err)
	}

	return nil
}

// imagePullSecret supplies credentials before the chart's Redis pre-install job runs.
func (a *Installer) imagePullSecret() (*corev1.Secret, error) {
	if a.OciPassword == "" {
		return nil, nil
	}

	host := installer.RegistryHost(a.OciRegistryURL)
	if host == "" {
		host = "ghcr.io"
	}

	username := ociUsernameOrDefault(a.OciUsername)

	config, err := json.Marshal(map[string]any{
		"auths": map[string]any{
			host: map[string]string{
				"username": username,
				"password": a.OciPassword,
				"auth":     base64.StdEncoding.EncodeToString([]byte(username + ":" + a.OciPassword)),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encoding ArgoCD image pull credentials: %w", err)
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: imagePullSecretName, Namespace: DefaultNamespace},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data:       map[string][]byte{corev1.DockerConfigJsonKey: config},
	}, nil
}

// configureRegistryCA adds the OCI registry's certificate authority to Helm's registry client
// and returns values for ArgoCD's certificate store. The repo server runs in a container and
// reads that store instead of the trust store of the node it runs on. The store is keyed by
// host name without its port.
func (a *Installer) configureRegistryCA() (map[string]any, error) {
	if a.OciRegistryCAFile == "" {
		return nil, nil
	}

	authority, err := os.ReadFile(a.OciRegistryCAFile)
	if err != nil {
		return nil, fmt.Errorf("reading registry certificate authority: %w", err)
	}

	if err := a.Helm.ConfigureRegistryCA(authority); err != nil {
		return nil, fmt.Errorf("configuring Helm registry trust: %w", err)
	}

	// A registry URL may name its scheme, which is neither part of the host nor recognised as
	// one: splitting "oci:" into host and port succeeds and yields the scheme itself.
	address := installer.RegistryHost(a.OciRegistryURL)

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

func ociUsernameOrDefault(username string) string {
	if username != "" {
		return username
	}

	return defaultOciUsername
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

// upgrade performs a Helm upgrade after the requested version has been validated.
func (a *Installer) upgrade(ctx context.Context, cfg installer.ChartConfig, existing *installer.ReleaseInfo) error {
	log.Printf("Found existing ArgoCD release with chart version %s\n", existing.InstalledVersion)
	if a.Version != "" {
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

func (a *Installer) validateUpgradeVersion(cfg installer.ChartConfig, existing *installer.ReleaseInfo) error {
	// Helm records the chart name, but not its original repository reference.
	// Different charts have independent versions, so only compare matching names.
	if a.Version != "" && existing.ChartName == path.Base(cfg.ChartName) {
		comparison, err := installer.CompareChartVersions(a.Version, existing.InstalledVersion)
		if err != nil {
			return fmt.Errorf("comparing ArgoCD chart versions: %w", err)
		}

		if comparison < 0 {
			return fmt.Errorf(
				"requested version %s is older than installed version %s; downgrade is not allowed",
				a.Version, existing.InstalledVersion,
			)
		}
	}

	return nil
}

func (a *Installer) showPostInstallHints() {
	log.Println(`To get ArgoCD admin password:`)
	log.Println(`  kubectl get secrets/argocd-initial-admin-secret -nargocd -ojson | jq -r ".data.password" | base64 -d`)
	log.Println(`To port-forward ArgoCD UI to localhost:8080:`)
	log.Println(`  kubectl port-forward svc/argocd-server 8080:80 -nargocd`)
}
