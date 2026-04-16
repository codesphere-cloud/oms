// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	stdio "io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	csio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/bootstrap/local"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
	rookcephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

type BootstrapLocalCmd struct {
	cmd             *cobra.Command
	CodesphereEnv   *local.CodesphereEnvironment
	Yes             bool
	FeatureFlagList []string
}

func (c *BootstrapLocalCmd) RunE(_ *cobra.Command, args []string) error {
	err := c.BootstrapLocal()

	if err != nil {
		return fmt.Errorf("failed to bootstrap: %w", err)
	}

	return nil
}

func AddBootstrapLocalCmd(parent *cobra.Command) {
	bootstrapLocalCmd := BootstrapLocalCmd{
		cmd: &cobra.Command{
			Use:   "bootstrap-local",
			Short: "Bootstrap a local Codesphere environment",
			Long: csio.Long(`Bootstraps a local Codesphere environment using a single Linux x86_64 Kubernetes cluster.
				Rook is used to install Ceph, and CNPG is used for the PostgreSQL database.
				For local setups, use Minikube with a virtual machine on Linux.
				Not for production use.`),
		},
		CodesphereEnv: &local.CodesphereEnvironment{},
	}

	flags := bootstrapLocalCmd.cmd.Flags()
	// Installer
	flags.BoolVarP(&bootstrapLocalCmd.Yes, "yes", "y", false, "Auto-approve the local bootstrapping warning prompt")
	flags.StringVar(&bootstrapLocalCmd.CodesphereEnv.InstallVersion, "install-version", "", "Codesphere version to install (downloaded from the OMS portal)")
	flags.StringVar(&bootstrapLocalCmd.CodesphereEnv.InstallHash, "install-hash", "", "Codesphere package hash (required when install-version is set)")
	flags.StringVar(&bootstrapLocalCmd.CodesphereEnv.InstallLocal, "install-local", "", "Path to a local installer package (tar.gz or unpacked directory)")
	// Registry
	flags.StringVar(&bootstrapLocalCmd.CodesphereEnv.RegistryUser, "registry-user", "", "Custom Registry username (optional)")

	// Codesphere Environment
	flags.StringVar(&bootstrapLocalCmd.CodesphereEnv.BaseDomain, "base-domain", "cs.local", "Base domain for Codesphere")
	flags.StringArrayVar(&bootstrapLocalCmd.CodesphereEnv.Experiments, "experiments", gcp.DefaultExperiments, "Experiments to enable in Codesphere installation (optional)")
	flags.StringArrayVar(&bootstrapLocalCmd.FeatureFlagList, "feature-flags", []string{}, "Feature flags to enable in Codesphere installation (optional)")
	flags.StringVar(&bootstrapLocalCmd.CodesphereEnv.Profile, "profile", installer.PROFILE_DEV, "Profile to apply to the install config like resources (supported: dev, minimal, prod)")
	flags.BoolVar(&bootstrapLocalCmd.CodesphereEnv.K0s, "k0s", false, "Use k0s-specific configuration (required to deploy to k0s clusters)")

	flags.StringVar(&bootstrapLocalCmd.CodesphereEnv.ServiceCIDR, "service-cidr", "", "Service CIDR of the Kubernetes cluster. If not specified, OMS will try to determine it.")
	flags.StringVar(&bootstrapLocalCmd.CodesphereEnv.PodCIDR, "pod-cidr", "", "Service CIDR of the Kubernetes cluster. If not specified, OMS will try to determine it.")

	// Config
	flags.StringVar(&bootstrapLocalCmd.CodesphereEnv.InstallDir, "install-dir", ".installer", "Directory for config, secrets, and bundle files")
	flags.StringVar(&bootstrapLocalCmd.CodesphereEnv.InstallConfigPath, "install-config", "", "Path to install config file (default: <install-dir>/config.yaml)")
	flags.StringVar(&bootstrapLocalCmd.CodesphereEnv.SecretsFilePath, "secrets-file", "", "Path to secrets file (default: <install-dir>/prod.vault.yaml)")
	bootstrapLocalCmd.cmd.RunE = bootstrapLocalCmd.RunE

	util.MarkFlagRequired(bootstrapLocalCmd.cmd, "registry-user")

	AddCmd(parent, bootstrapLocalCmd.cmd)
}

func (c *BootstrapLocalCmd) BootstrapLocal() error {
	ctx := c.cmd.Context()
	if err := c.ConfirmLocalBootstrapWarning(); err != nil {
		return err
	}

	if err := c.resolveRegistryPassword(); err != nil {
		return err
	}

	// Resolve install-config and secrets-file defaults from install-dir.
	if c.CodesphereEnv.InstallConfigPath == "" {
		c.CodesphereEnv.InstallConfigPath = filepath.Join(c.CodesphereEnv.InstallDir, "config.yaml")
	}
	if c.CodesphereEnv.SecretsFilePath == "" {
		c.CodesphereEnv.SecretsFilePath = filepath.Join(c.CodesphereEnv.InstallDir, "prod.vault.yaml")
	}

	// Ensure the install directory exists.
	if err := os.MkdirAll(c.CodesphereEnv.InstallDir, 0755); err != nil {
		return fmt.Errorf("failed to create install directory %s: %w", c.CodesphereEnv.InstallDir, err)
	}

	if err := c.ValidatePrerequisites(ctx); err != nil {
		return err
	}

	for _, flag := range c.FeatureFlagList {
		c.CodesphereEnv.FeatureFlags[flag] = true
	}

	stlog := bootstrap.NewStepLogger(false)
	icg := installer.NewInstallConfigManager()
	fw := util.NewFilesystemWriter()
	kubeClient, restConfig, err := c.GetKubeClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize Kubernetes client: %w", err)
	}

	helmClient, err := installer.NewHelmClient("codesphere")
	if err != nil {
		return fmt.Errorf("failed to initialize Helm client: %w", err)
	}

	bs := local.NewLocalBootstrapper(ctx, stlog, kubeClient, restConfig, fw, icg, helmClient, c.CodesphereEnv)
	return bs.Bootstrap()
}

func (c *BootstrapLocalCmd) resolveRegistryPassword() error {
	if pw := os.Getenv("OMS_REGISTRY_PASSWORD"); len(pw) != 0 {
		c.CodesphereEnv.RegistryPassword = pw
		return nil
	}
	fmt.Print("Registry password: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read registry password: %w", err)
	}
	if len(pw) == 0 {
		return fmt.Errorf("registry password is required; set OMS_REGISTRY_PASSWORD or enter it when prompted")
	}
	c.CodesphereEnv.RegistryPassword = string(pw)
	return nil
}

func (c *BootstrapLocalCmd) ConfirmLocalBootstrapWarning() error {
	fmt.Println(csio.Long(`
		############################################################
		# Local Bootstrap Warning                                  #
		############################################################
		#
		# Codesphere local bootstrap is for testing only.
		#
		# Currently supported:
		# - One Kubernetes cluster with Linux x86_64 nodes only
		# - Kubernetes Cluster on Linux with a VM and an extra disk for Rook/Ceph
		#   (use --k0s flag for k0s specific configuration)
		#
		# Not supported:
		# - Minikube on macOS
		#
		# Never run Rook directly on your host system; local disks may be consumed.
		#
		# Recommended command:
		#   minikube start --disk-size=40g --extra-disks=1 --driver kvm2
		############################################################
	`))

	if c.Yes {
		return nil
	}

	fmt.Print("\nType 'yes' to continue: ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, stdio.EOF) {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}

	if strings.TrimSpace(strings.ToLower(input)) != "yes" {
		return fmt.Errorf("aborted: type 'yes' to continue or pass --yes")
	}

	return nil
}

func (c *BootstrapLocalCmd) GetKubeClient(ctx context.Context) (ctrlclient.Client, *rest.Config, error) {
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

func (c *BootstrapLocalCmd) ValidatePrerequisites(ctx context.Context) error {
	if err := c.ValidateKubernetesCluster(ctx); err != nil {
		return err
	}

	if err := c.ValidateHelmVersion(ctx); err != nil {
		return err
	}

	if err := c.ValidateEncryptionTools(); err != nil {
		return err
	}

	return nil
}

func (c *BootstrapLocalCmd) ValidateKubernetesCluster(ctx context.Context) error {
	kubeClient, _, err := c.GetKubeClient(ctx)
	if err != nil {
		return err
	}

	nodeList := &corev1.NodeList{}
	if err := kubeClient.List(ctx, nodeList); err != nil {
		return fmt.Errorf("failed to list Kubernetes nodes: %w", err)
	}

	if len(nodeList.Items) == 0 {
		return fmt.Errorf("connected to Kubernetes cluster but no nodes are available")
	}

	return nil
}

func (c *BootstrapLocalCmd) ValidateHelmVersion(ctx context.Context) error {
	helmPath, err := exec.LookPath("helm")
	if err != nil {
		return fmt.Errorf("helm binary not found in PATH, Helm 3 or newer is required")
	}

	out, err := exec.CommandContext(ctx, helmPath, "version", "--template={{.Version}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get helm version: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	version := strings.TrimSpace(string(out))
	if !semver.IsValid(version) {
		return fmt.Errorf("failed to parse helm version %q: not a valid semantic version", version)
	}

	if semver.Compare(version, "v3.0.0") < 0 {
		return fmt.Errorf("helm version %s is not supported, Helm 3 or newer is required", version)
	}

	return nil
}

func (c *BootstrapLocalCmd) ValidateEncryptionTools() error {
	if _, err := exec.LookPath("sops"); err != nil {
		return fmt.Errorf("sops binary not found in PATH; install it with: brew install sops")
	}

	if _, err := exec.LookPath("age-keygen"); err != nil {
		return fmt.Errorf("age binary not found in PATH (age-keygen missing); install it with: brew install age")
	}

	return nil
}
