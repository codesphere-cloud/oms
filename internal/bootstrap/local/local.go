// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	rookcephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	codesphereSystemNamespace = "codesphere-system"
	codesphereNamespace       = "codesphere"
	workspacesNamespace       = "workspaces"
)

type retryableWaitError struct {
	err error
}

func (e *retryableWaitError) Error() string {
	return e.err.Error()
}

func (e *retryableWaitError) Unwrap() error {
	return e.err
}

func isRetryableWaitError(err error) bool {
	var target *retryableWaitError
	return errors.As(err, &target)
}

type LocalBootstrapper struct {
	ctx        context.Context
	stlog      *bootstrap.StepLogger
	kubeClient client.Client
	restConfig *rest.Config
	fw         util.FileIO
	icg        installer.InstallConfigManager
	helm       installer.HelmClient
	// Environment
	Env *CodesphereEnvironment
	// cephCredentials holds the Ceph auth credentials read after setup.
	cephCredentials *CephCredentials
	// ageRecipient is the age public key used for SOPS vault encryption.
	ageRecipient string
	// ageKeyPath is the filesystem path to the age private key file.
	ageKeyPath string
}

type CodesphereEnvironment struct {
	BaseDomain   string          `json:"base_domain"`
	Experiments  []string        `json:"experiments"`
	FeatureFlags map[string]bool `json:"feature_flags"`
	Profile      string          `json:"profile"`
	// Installer
	InstallVersion string `json:"install_version"`
	InstallHash    string `json:"install_hash"`
	InstallLocal   string `json:"install_local"`
	// Registry
	RegistryUser     string `json:"-"`
	RegistryPassword string `json:"-"`
	// Config
	InstallDir         string              `json:"-"`
	ExistingConfigUsed bool                `json:"-"`
	InstallConfigPath  string              `json:"-"`
	SecretsFilePath    string              `json:"-"`
	InstallConfig      *files.RootConfig   `json:"-"`
	Vault              *files.InstallVault `json:"-"`
	K0s                bool                `json:"-"`
	PodCIDR            string              `json:"pod_cidr"`
	ServiceCIDR        string              `json:"service_cidr"`

	// k0sctl configuration fields
	SSHPublicKey string `json:"ssh_public_key"`
}

func NewLocalBootstrapper(ctx context.Context, stlog *bootstrap.StepLogger, fw util.FileIO, icg installer.InstallConfigManager, helm installer.HelmClient, env *CodesphereEnvironment) *LocalBootstrapper {
	return &LocalBootstrapper{
		ctx:   ctx,
		stlog: stlog,
		fw:    fw,
		icg:   icg,
		helm:  helm,
		Env:   env,
	}
}

func (b *LocalBootstrapper) Bootstrap() error {
	err := b.stlog.Step("Ensure install config", b.EnsureInstallConfig)
	if err != nil {
		return fmt.Errorf("failed to ensure install config: %w", err)
	}

	err = b.stlog.Step("Ensure secrets", b.EnsureSecrets)
	if err != nil {
		return fmt.Errorf("failed to ensure secrets: %w", err)
	}

	err = b.stlog.Step("Resolve age encryption key", b.ResolveAgeKey)
	if err != nil {
		return fmt.Errorf("failed to resolve age encryption key: %w", err)
	}

	err = b.stlog.Step("Ensure k0s", b.EnsureK0s)
	if err != nil {
		return fmt.Errorf("failed to ensure k0s: %w", err)
	}

	err = b.stlog.Step("Ensure namespaces", b.EnsureNamespaces)
	if err != nil {
		return fmt.Errorf("failed to ensure namespaces: %w", err)
	}

	err = b.stlog.Step("Install Rook and test Ceph cluster", func() error {
		err := b.stlog.Substep("Install Rook operator", b.InstallRookHelmChart)
		if err != nil {
			return err
		}

		err = b.stlog.Substep("Deploy test Ceph cluster (single OSD)", b.DeployTestCephCluster)
		if err != nil {
			return err
		}

		err = b.stlog.Substep("Create CephBlockPool and StorageClass", b.DeployCephBlockPoolAndStorageClass)
		if err != nil {
			return err
		}

		err = b.stlog.Substep("Deploy CephFS filesystem", b.DeployCephFilesystem)
		if err != nil {
			return err
		}

		err = b.stlog.Substep("Create CephFS SubVolumeGroup", b.DeployCephFilesystemSubVolumeGroup)
		if err != nil {
			return err
		}

		err = b.stlog.Substep("Bootstrap RGW gateway", b.DeployRGWGateway)
		if err != nil {
			return err
		}

		err = b.stlog.Substep("Ensure Ceph users", func() error {
			creds, err := b.EnsureCephUsers()
			if err != nil {
				return err
			}
			b.cephCredentials = creds
			return nil
		})
		if err != nil {
			return err
		}

		err = b.stlog.Substep("Create Ceph admin credential secrets", b.CreateCephAdminSecrets)
		if err != nil {
			return err
		}

		err = b.stlog.Substep("Sync ceph-mon-endpoints ConfigMap", b.SyncCephMonEndpoints)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to install Rook and deploy test Ceph cluster: %w", err)
	}

	err = b.stlog.Step("Install CloudNativePG and PostgreSQL", func() error {
		err := b.stlog.Substep("Install CloudNativePG operator", b.InstallCloudNativePGHelmChart)
		if err != nil {
			return err
		}

		err = b.stlog.Substep("Deploy PostgreSQL database", b.DeployPostgresDatabase)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to install CloudNativePG and deploy PostgreSQL database: %w", err)
	}

	err = b.stlog.Step("Update install config", b.UpdateInstallConfig)
	if err != nil {
		return fmt.Errorf("failed to update install config: %w", err)
	}

	err = b.stlog.Step("Run Codesphere installer", b.RunInstaller)
	if err != nil {
		return fmt.Errorf("failed to run Codesphere installer: %w", err)
	}

	return nil
}

func (b *LocalBootstrapper) EnsureNamespaces() error {
	for _, ns := range []string{codesphereSystemNamespace, codesphereNamespace, workspacesNamespace} {
		for retries := 0; retries < 5; retries++ {
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: ns},
			}

			// Mark the workspaces namespace as owned by the cluster-config Helm
			// release so that the chart can manage it during install/upgrade.
			if ns == workspacesNamespace {
				namespace.Labels = map[string]string{
					"app.kubernetes.io/managed-by":   "Helm",
					"meta.helm.sh/release-name":      "cluster-config",
					"meta.helm.sh/release-namespace": codesphereNamespace,
				}
				namespace.Annotations = map[string]string{
					"meta.helm.sh/release-name":      "cluster-config",
					"meta.helm.sh/release-namespace": codesphereNamespace,
				}
			}

			err := b.kubeClient.Create(b.ctx, namespace)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create namespace %q: %w", ns, err)
			}

			if err == nil || apierrors.IsAlreadyExists(err) {
				break
			}
			time.Sleep(5 * time.Second)
		}
	}

	// Create a dummy error-page-server Service in the codesphere namespace.
	// The nginx ingress controller references this service as a default backend;
	// without it the controller pods fail to start.
	errorPageSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "error-page-server",
			Namespace: codesphereNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":   "Helm",
				"meta.helm.sh/release-name":      "codesphere",
				"meta.helm.sh/release-namespace": codesphereNamespace,
			},
			Annotations: map[string]string{
				"meta.helm.sh/release-name":      "codesphere",
				"meta.helm.sh/release-namespace": codesphereNamespace,
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{{
				Name: "web",
				Port: 8083,
			}},
			Selector: map[string]string{"app": "error-page-server"},
		},
	}
	if err := b.kubeClient.Create(b.ctx, errorPageSvc); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create dummy error-page-server service: %w", err)
	}

	return nil
}

// CreateCephAdminSecrets creates the ceph-admin-credentials Secret in the
// codesphere and workspaces namespaces containing the CephFS admin credentials.
func (b *LocalBootstrapper) CreateCephAdminSecrets() error {
	if b.cephCredentials == nil {
		return fmt.Errorf("ceph credentials have not been read yet")
	}

	for _, ns := range []string{codesphereNamespace, workspacesNamespace} {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ceph-admin-credentials",
				Namespace: ns,
			},
		}
		_, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, secret, func() error {
			secret.Type = corev1.SecretTypeOpaque
			secret.StringData = map[string]string{
				"ceph-username": b.cephCredentials.CephfsAdmin.Entity,
				"ceph-secret":   b.cephCredentials.CephfsAdmin.Key,
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to create or update ceph-admin-credentials secret in namespace %q: %w", ns, err)
		}
	}

	return nil
}

// SyncCephMonEndpoints copies the rook-ceph-mon-endpoints ConfigMap from the
// rook-ceph namespace into the codesphere and workspaces namespaces so that
// CSI plugins and other consumers can discover the Ceph monitor addresses.
func (b *LocalBootstrapper) SyncCephMonEndpoints() error {
	source := &corev1.ConfigMap{}
	key := client.ObjectKey{Namespace: "rook-ceph", Name: "rook-ceph-mon-endpoints"}
	if err := b.kubeClient.Get(b.ctx, key, source); err != nil {
		return fmt.Errorf("failed to read rook-ceph-mon-endpoints ConfigMap from rook-ceph namespace: %w", err)
	}

	for _, ns := range []string{codesphereNamespace, workspacesNamespace} {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ceph-mon-endpoints",
				Namespace: ns,
			},
		}
		_, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, cm, func() error {
			cm.Data = source.Data
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to sync ceph-mon-endpoints ConfigMap to namespace %q: %w", ns, err)
		}
	}

	return nil
}

// ReadClusterCIDRs reads the pod and service CIDRs.
// If specified, uses CIDRs from the input parameters.
// Else, pod CIDR is read from the first node's spec.podCIDR.
// Service CIDR is read from the kube-apiserver pod's --service-cluster-ip-range flag.
// If that fails, it tries to extract it from a local kube-apiserver process
func (b *LocalBootstrapper) ReadClusterCIDRs() (podCIDR string, serviceCIDR string, err error) {
	podCIDR = b.Env.PodCIDR
	if podCIDR == "" {
		podCIDR, err = b.readPodCIDR()
		if err != nil {
			return "", "", fmt.Errorf("failed to detect pod CIDR: %w", err)
		}
	}

	serviceCIDR = b.Env.ServiceCIDR
	if serviceCIDR == "" {
		serviceCIDR, err = b.readServiceCIDRFromK8s()
		if serviceCIDR != "" {
			return podCIDR, serviceCIDR, nil
		}

		log.Printf("can't read service CIDR from cluster, trying proc filesystem next: %s", err)

		serviceCIDR, err = b.readServiceCIDRFromProc()

		if err != nil {
			err = fmt.Errorf("failed to determine service CIDR: %w", err)
		}
	}
	return
}

// readPodCIDR reads the pod CIDR from the first node.
func (b *LocalBootstrapper) readPodCIDR() (string, error) {
	nodeList := &corev1.NodeList{}
	if err := b.kubeClient.List(b.ctx, nodeList); err != nil {
		return "", fmt.Errorf("failed to list nodes: %w", err)
	}
	if len(nodeList.Items) == 0 {
		return "", fmt.Errorf("no nodes found in cluster")
	}
	podCIDR := nodeList.Items[0].Spec.PodCIDR
	if podCIDR == "" {
		return "", fmt.Errorf("node %q does not have a podCIDR set", nodeList.Items[0].Name)
	}
	return podCIDR, nil
}

// readServiceCIDRFromK8s reads service CIDR from the kube-apiserver pod's --service-cluster-ip-range flag.
func (b *LocalBootstrapper) readServiceCIDRFromK8s() (serviceCIDR string, err error) {
	nodeList := &corev1.NodeList{}
	if err := b.kubeClient.List(b.ctx, nodeList); err != nil {
		return "", fmt.Errorf("failed to list nodes: %w", err)
	}
	if len(nodeList.Items) == 0 {
		return "", fmt.Errorf("no nodes found in cluster")
	}
	apiServerPod := &corev1.Pod{}
	key := client.ObjectKey{Name: "kube-apiserver-" + nodeList.Items[0].Name, Namespace: "kube-system"}
	if err = b.kubeClient.Get(b.ctx, key, apiServerPod); err != nil {
		return "", fmt.Errorf("failed to get kube-apiserver pod: %w", err)
	}

	for _, container := range apiServerPod.Spec.Containers {
		for _, arg := range container.Command {
			if strings.HasPrefix(arg, "--service-cluster-ip-range=") {
				serviceCIDR = strings.TrimPrefix(arg, "--service-cluster-ip-range=")
				break
			}
		}
		if serviceCIDR != "" {
			break
		}
	}

	if serviceCIDR == "" {
		return "", fmt.Errorf("could not determine service CIDR from kube-apiserver pod")
	}

	return serviceCIDR, nil
}

// readServiceCIDRFromProc reads the service CIDR from the api server process on the local machine
// this is necessary for single node k0s installations
func (b *LocalBootstrapper) readServiceCIDRFromProc() (serviceCIDR string, err error) {
	// Look for the kube-apiserver process arguments
	matches, _ := filepath.Glob("/proc/*/cmdline")
	for _, path := range matches {
		var content []byte
		content, err = os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("failed to read cmdline from proc FS: %w", err)
		}
		cmdline := string(content)

		if strings.Contains(cmdline, "kube-apiserver") {
			// Arguments in /proc/PID/cmdline are null-terminated
			args := strings.Split(cmdline, "\x00")
			for _, arg := range args {
				if strings.HasPrefix(arg, "--service-cluster-ip-range=") {
					serviceCIDR = strings.Split(arg, "=")[1]
					return
				}
			}
		}
	}
	return "", errors.New("can't find service CIDR")
}

func (b *LocalBootstrapper) EnsureInstallConfig() error {
	if b.fw.Exists(b.Env.InstallConfigPath) {
		err := b.icg.LoadInstallConfigFromFile(b.Env.InstallConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}

		b.Env.ExistingConfigUsed = true
	}
	err := b.icg.ApplyProfile(b.Env.Profile)
	if err != nil {
		return fmt.Errorf("failed to apply profile: %w", err)
	}

	b.Env.InstallConfig = b.icg.GetInstallConfig()

	return nil
}

func (b *LocalBootstrapper) EnsureSecrets() error {
	if b.fw.Exists(b.Env.SecretsFilePath) {
		err := b.icg.LoadVaultFromFile(b.Env.SecretsFilePath)
		if err != nil {
			return fmt.Errorf("failed to load vault file: %w", err)
		}
		err = b.icg.MergeVaultIntoConfig()
		if err != nil {
			return fmt.Errorf("failed to merge vault into config: %w", err)
		}
	}

	b.Env.Vault = b.icg.GetVault()

	return nil
}

func (b *LocalBootstrapper) ResolveAgeKey() error {
	recipient, keyPath, err := installer.ResolveAgeKey(filepath.Dir(b.Env.SecretsFilePath))
	if err != nil {
		return fmt.Errorf("failed to resolve age key: %w", err)
	}
	b.ageRecipient = recipient
	b.ageKeyPath = keyPath
	if keyPath != "" {
		fmt.Printf("Using age key: %s\n", keyPath)
	}
	return nil
}

func (b *LocalBootstrapper) EnsureK0s() error {

	fw := util.NewFilesystemWriter()
	hw := portal.NewHttpWrapper()
	env := env.NewEnv()
	k0s := installer.NewK0s(hw, env, fw)
	k0sVersion, err := k0s.GetLatestVersion()
	if err != nil {
		return fmt.Errorf("failed to get latest k0s version: %w", err)
	}
	k0sPath, err := k0s.Download(k0sVersion, true, false)
	if err != nil {
		return fmt.Errorf("failed to download k0s: %w", err)
	}

	k0sctl := installer.NewK0sctl(hw, env, fw)
	k0sctlpath, err := k0sctl.Download("", true, false)
	if err != nil {
		return fmt.Errorf("failed to download k0sctl: %w", err)
	}

	k0sctlConfig, err := installer.GenerateK0sctlConfigSingle(b.Env.InstallConfig, k0sVersion, k0sPath)
	if err != nil {
		return fmt.Errorf("failed to generate k0sctl config: %w", err)
	}

	k0sctlConfigData, err := k0sctlConfig.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal k0sctl config: %w", err)
	}

	k0sctlConfigPath := filepath.Join(env.GetOmsWorkdir(), "k0sctl-config.yaml")
	err = fw.WriteFile(k0sctlConfigPath, k0sctlConfigData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write k0sctl config: %w", err)
	}

	err = k0sctl.Apply(k0sctlConfigPath, k0sctlpath, false)
	if err != nil {
		return fmt.Errorf("failed to apply k0sctl config: %w", err)
	}

	kubeconfigPath := filepath.Join(env.GetOmsWorkdir(), "kubeconfig.yaml")
	err = k0sctl.WriteKubeconfig(k0sctlpath, k0sctlConfigPath, kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	b.kubeClient, b.restConfig, err = b.GetKubeClient(context.Background(), kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to initialize Kubernetes client: %w", err)
	}

	return nil
}

func (b *LocalBootstrapper) UpdateInstallConfig() (err error) {
	b.Env.InstallConfig.Secrets.BaseDir = filepath.Join(b.Env.InstallDir, "secrets")
	if err := os.MkdirAll(b.Env.InstallConfig.Secrets.BaseDir, 0700); err != nil {
		return fmt.Errorf("failed to create secrets base directory: %w", err)
	}
	if err := b.EnsureGitHubAccessConfigured(); err != nil {
		return fmt.Errorf("failed to ensure GitHub access is configured: %w", err)
	}

	b.Env.InstallConfig.Postgres.Mode = "external"
	b.Env.InstallConfig.Postgres.Database = cnpgDatabaseName
	b.Env.InstallConfig.Postgres.CACertPem, err = b.ReadPostgresCA()
	if err != nil {
		return fmt.Errorf("failed to read PostgreSQL CA: %w", err)
	}

	b.Env.InstallConfig.Postgres.ServerAddress = "masterdata-rw.codesphere.svc.cluster.local"
	b.Env.InstallConfig.Postgres.Port = 5432
	b.Env.InstallConfig.Postgres.Primary = nil
	b.Env.InstallConfig.Postgres.Replica = nil
	pgPassword, err := b.ReadPostgresSuperuserPassword()
	if err != nil {
		return fmt.Errorf("failed to read PostgreSQL superuser password: %w", err)
	}
	b.Env.Vault.SetSecret(files.SecretEntry{
		Name: "postgresPassword",
		Fields: &files.SecretFields{
			Password: pgPassword,
		},
	})

	// Store the active kubeconfig in the vault so that install-components.js
	// can retrieve it via SecretManagerSops when deploying Helm charts.
	kubeConfigContent, err := b.getKubeConfig()
	if err != nil {
		return fmt.Errorf("failed to read kubeconfig: %w", err)
	}
	b.Env.Vault.SetSecret(files.SecretEntry{
		Name: "kubeConfig",
		File: &files.SecretFile{
			Name:    "kubeConfig",
			Content: kubeConfigContent,
		},
	})

	b.Env.InstallConfig.Cluster.RookExternalCluster = &files.RookExternalClusterConfig{
		Enabled: false,
	}
	b.Env.InstallConfig.Cluster.PgOperator = &files.PgOperatorConfig{
		Enabled: false,
	}
	b.Env.InstallConfig.Cluster.BarmanCloudPlugin = &files.BarmanCloudPluginConfig{
		Enabled: false,
	}
	b.Env.InstallConfig.Cluster.RgwLoadBalancer = &files.RgwLoadBalancerConfig{
		Enabled: true,
	}
	cephMonHosts, err := b.ReadCephMonHosts()
	if err != nil {
		return fmt.Errorf("failed to read Ceph monitor hosts: %w", err)
	}
	b.Env.InstallConfig.Ceph = files.CephConfig{
		Hosts: cephMonHosts,
	}
	if b.cephCredentials != nil {
		b.addCephSecretsToVault(b.Env.Vault)
	}

	b.Env.InstallConfig.Kubernetes = files.KubernetesConfig{
		ManagedByCodesphere: false,
	}

	podCIDR, serviceCIDR, err := b.ReadClusterCIDRs()
	if err != nil {
		return fmt.Errorf("failed to read cluster CIDRs: %w. Use --service-cidr and --pod-cidr to specify them", err)
	}
	b.Env.InstallConfig.Kubernetes.PodCIDR = podCIDR
	b.Env.InstallConfig.Kubernetes.ServiceCIDR = serviceCIDR
	b.Env.InstallConfig.Cluster.Gateway.ServiceType = "LoadBalancer"
	b.Env.InstallConfig.Cluster.PublicGateway.ServiceType = "LoadBalancer"

	// TODO: certificates
	b.Env.InstallConfig.Codesphere.CertIssuer = files.CertIssuerConfig{
		Type: "self-signed",
	}

	b.Env.InstallConfig.Codesphere.Domain = b.Env.BaseDomain
	b.Env.InstallConfig.Codesphere.WorkspaceHostingBaseDomain = "ws." + b.Env.BaseDomain
	// TODO: set public IP or configure DNS for local setup
	// b.Env.InstallConfig.Codesphere.PublicIP = b.Env.ControlPlaneNodes[1].GetExternalIP()
	b.Env.InstallConfig.Codesphere.CustomDomains = files.CustomDomainsConfig{
		CNameBaseDomain: "ws." + b.Env.BaseDomain,
	}
	b.Env.InstallConfig.Codesphere.DNSServers = []string{"8.8.8.8"}

	defaultImages := bootstrap.DefaultCodesphereDeployConfig().Images
	if b.Env.InstallConfig.Codesphere.DeployConfig.Images == nil {
		b.Env.InstallConfig.Codesphere.DeployConfig.Images = defaultImages
	} else {
		for imageName, defaultImage := range defaultImages {
			if _, exists := b.Env.InstallConfig.Codesphere.DeployConfig.Images[imageName]; !exists {
				b.Env.InstallConfig.Codesphere.DeployConfig.Images[imageName] = defaultImage
			}
		}
	}
	b.Env.InstallConfig.Codesphere.Plans = bootstrap.DefaultCodespherePlans()

	b.Env.InstallConfig.Codesphere.Experiments = b.Env.Experiments
	b.Env.InstallConfig.Codesphere.Features = b.Env.FeatureFlags

	if !b.Env.ExistingConfigUsed {
		err := b.icg.GenerateSecrets()
		if err != nil {
			return fmt.Errorf("failed to generate secrets: %w", err)
		}
	}

	if err := b.icg.WriteInstallConfig(b.Env.InstallConfigPath, true); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if err := b.icg.WriteVault(b.Env.SecretsFilePath, true); err != nil {
		return fmt.Errorf("failed to write vault file: %w", err)
	}
	if err := installer.EncryptFileWithSOPS(b.Env.SecretsFilePath, filepath.Join(b.Env.InstallConfig.Secrets.BaseDir, "prod.vault.yaml"), b.ageRecipient); err != nil {
		return fmt.Errorf("failed to encrypt vault file: %w", err)
	}

	return nil
}

func (b *LocalBootstrapper) EnsureGitHubAccessConfigured() error {
	if b.Env.RegistryPassword == "" {
		return fmt.Errorf("registry password is not set")
	}
	b.Env.InstallConfig.Registry.Server = "ghcr.io"
	b.Env.InstallConfig.Registry.Username = b.Env.RegistryUser
	b.Env.InstallConfig.Registry.Password = b.Env.RegistryPassword
	b.Env.InstallConfig.Registry.ReplaceImagesInBom = false
	b.Env.InstallConfig.Registry.LoadContainerImages = false
	return nil
}

// addCephSecretsToVault appends Ceph credentials to the vault as SecretEntry items.
// These mirror the secrets that the JS installer stores via SecretManagerSops:
//   - cephFsId (password = FSID)
//   - cephfsAdmin, cephfsAdminCodesphere (password = auth key)
//   - rgwAdminAccessKey, rgwAdminSecretKey (password = S3 access/secret keys)
//   - csiRbdNode, csiRbdProvisioner, csiCephfsNode, csiCephfsProvisioner, csiOperator (password = auth key)
func (b *LocalBootstrapper) addCephSecretsToVault(vault *files.InstallVault) {
	creds := b.cephCredentials

	vault.SetSecret(files.SecretEntry{Name: "cephFsId", Fields: &files.SecretFields{Password: creds.FSID}})
	vault.SetSecret(files.SecretEntry{Name: "cephfsAdmin", Fields: &files.SecretFields{Username: creds.CephfsAdmin.Entity, Password: creds.CephfsAdmin.Key}})
	vault.SetSecret(files.SecretEntry{Name: "cephfsAdminCodesphere", Fields: &files.SecretFields{Username: creds.CephfsAdminCodesphere.Entity, Password: creds.CephfsAdminCodesphere.Key}})
	vault.SetSecret(files.SecretEntry{Name: "rgwAdminAccessKey", Fields: &files.SecretFields{Password: creds.RGWAdmin.AccessKey}})
	vault.SetSecret(files.SecretEntry{Name: "rgwAdminSecretKey", Fields: &files.SecretFields{Password: creds.RGWAdmin.SecretKey}})
	vault.SetSecret(files.SecretEntry{Name: "csiRbdNode", Fields: &files.SecretFields{Username: creds.CSIRBDNode.Entity, Password: creds.CSIRBDNode.Key}})
	vault.SetSecret(files.SecretEntry{Name: "csiRbdProvisioner", Fields: &files.SecretFields{Username: creds.CSIRBDProvisioner.Entity, Password: creds.CSIRBDProvisioner.Key}})
	vault.SetSecret(files.SecretEntry{Name: "csiCephfsNode", Fields: &files.SecretFields{Username: creds.CSICephFSNode.Entity, Password: creds.CSICephFSNode.Key}})
	vault.SetSecret(files.SecretEntry{Name: "csiCephfsProvisioner", Fields: &files.SecretFields{Username: creds.CSICephFSProvisioner.Entity, Password: creds.CSICephFSProvisioner.Key}})
	// csiOperator is managed by Rook internally; provide a dummy value for vault compatibility.
	vault.SetSecret(files.SecretEntry{Name: "csiOperator", Fields: &files.SecretFields{Username: "client.csi-rbd-provisioner", Password: "dummy"}})
}

// getKubeConfig builds a kubeconfig YAML from the in-memory rest.Config so
// that install-components.js can use it to talk to the cluster.
func (b *LocalBootstrapper) getKubeConfig() (string, error) {
	cfg := b.restConfig

	cluster := clientcmdapi.NewCluster()
	cluster.Server = cfg.Host
	cluster.CertificateAuthorityData = cfg.CAData
	if cfg.CAFile != "" && len(cluster.CertificateAuthorityData) == 0 {
		cluster.CertificateAuthority = cfg.CAFile
	}
	cluster.InsecureSkipTLSVerify = cfg.Insecure

	authInfo := clientcmdapi.NewAuthInfo()
	authInfo.ClientCertificateData = cfg.CertData
	if cfg.CertFile != "" && len(authInfo.ClientCertificateData) == 0 {
		authInfo.ClientCertificate = cfg.CertFile
	}
	authInfo.ClientKeyData = cfg.KeyData
	if cfg.KeyFile != "" && len(authInfo.ClientKeyData) == 0 {
		authInfo.ClientKey = cfg.KeyFile
	}
	authInfo.Token = cfg.BearerToken
	if cfg.BearerTokenFile != "" && authInfo.Token == "" {
		authInfo.TokenFile = cfg.BearerTokenFile
	}
	if cfg.Username != "" {
		authInfo.Username = cfg.Username
		authInfo.Password = cfg.Password
	}

	kubeConfig := clientcmdapi.NewConfig()
	kubeConfig.Clusters["default"] = cluster
	kubeConfig.AuthInfos["default"] = authInfo
	kubeConfig.Contexts["default"] = &clientcmdapi.Context{
		Cluster:  "default",
		AuthInfo: "default",
	}
	kubeConfig.CurrentContext = "default"

	data, err := clientcmd.Write(*kubeConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal kubeconfig from rest.Config: %w", err)
	}

	return string(data), nil
}

func (b *LocalBootstrapper) GetKubeClient(ctx context.Context, kubeconfigPath string) (ctrlclient.Client, *rest.Config, error) {
	err := os.Setenv("KUBECONFIG", kubeconfigPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to set KUBECONFIG environment variable: %w", err)
	}
	kubeConfig, err := ctrlconfig.GetConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load Kubernetes config: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, nil, fmt.Errorf("failed to add Kubernetes core scheme: %w", err)
	}

	if err := cnpgv1.AddToScheme(scheme); err != nil {
		return nil, nil, fmt.Errorf("failed to add CloudNativePG scheme: %w", err)
	}

	if err := rookcephv1.AddToScheme(scheme); err != nil {
		return nil, nil, fmt.Errorf("failed to add Rook Ceph scheme: %w", err)
	}

	kubeClient, err := ctrlclient.New(kubeConfig, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize Kubernetes client: %w", err)
	}
	return kubeClient, kubeConfig, nil
}
