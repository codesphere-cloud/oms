// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"context"
	"crypto/rand"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/csapps"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InstallCodespherePlatformCmd runs only the Codesphere platform step (Phase 3).
type InstallCodespherePlatformCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

// Run installs the Codesphere platform phase; cancelling ctx stops the installation.
func (c *InstallCodespherePlatformCmd) Run(ctx context.Context) error {
	if err := validateInstallCodesphereVault(c.Opts); err != nil {
		return err
	}

	if c.Opts.LocalComponents && c.Opts.LocalConfigDir == "" {
		return fmt.Errorf("--local-config-dir is required with --local-components")
	}

	effectiveOpts, cfg, cleanup, err := prepareInstallConfig(c.Opts, installer.NewConfig())
	if err != nil {
		return err
	}
	defer cleanup()

	if err := ensureCodespherePackageForVersion(ctx, effectiveOpts, cfg, c.Env); err != nil {
		return err
	}

	return installCodespherePlatform(ctx, effectiveOpts, cfg, c.Env)
}

func installCodespherePlatform(ctx context.Context, opts *InstallCodesphereOpts, cfg files.RootConfig, env env.Env) error {
	if err := installer.EnsureClusterAdminSecret(ctx, opts.Vault, opts.PrivKey, opts.VaultType, cfg); err != nil {
		return fmt.Errorf("failed to set cluster admin email: %w", err)
	}

	// If the codesphere step is skipped we don't want to update the secret that is the source for ESO.
	// We use ESO with `refreshPolicy: OnChange` which would result in a long lived drift between source and target secret which might be confusing for debugging and could cause problems if we ever switch the refreshPolicy or trigger a manual sync.
	if !installer.IsStepSkipped(cfg, opts.SkipSteps, installer.CodesphereStep) {
		if err := ensureVaultSecret(ctx, opts, cfg); err != nil {
			return err
		}

		if err := vault.WriteVaultValuesToConfig(opts.ConfigPath, opts.VaultSecretName, rand.Text()); err != nil {
			return fmt.Errorf("failed to pass the vault Secret to the chart: %w", err)
		}
	}

	viaArgoCD := opts.CodesphereViaArgoCD && !installer.IsStepSkipped(cfg, opts.SkipSteps, installer.CodesphereStep)

	workdir := env.GetOmsWorkdir()

	pm := installer.NewPackage(workdir, opts.Package)
	if opts.LocalComponents {
		pm = installer.NewPackage(filepath.Dir(opts.Package), opts.Package)
	}

	var (
		kubeClient       client.Client
		installerBOM     *bom.Config
		installedVersion string
	)

	if viaArgoCD {
		var err error

		// ci.Install migrates the database before the ArgoCD deploy, so a BOM the deploy
		// cannot use must stop the install here.
		installerBOM, err = readArgoCDBOM(pm, opts)
		if err != nil {
			return err
		}

		kubeClient, err = newKubeClient(opts, cfg)
		if err != nil {
			return err
		}

		installedVersion, err = csapps.InstalledVersion(ctx, kubeClient, &cfg)
		if err != nil {
			return fmt.Errorf("failed to read the installed Codesphere version: %w", err)
		}
	}

	cm := installer.NewConfig()
	im := system.NewImage(ctx)

	ci := &installer.CodesphereInstaller{
		ConfigPath:        opts.ConfigPath,
		VaultPath:         opts.Vault,
		PrivKey:           opts.PrivKey,
		VaultType:         opts.VaultType,
		Force:             opts.Force,
		Verbose:           opts.Verbose,
		SkipSteps:         opts.SkipSteps,
		AllowedSteps:      installer.PlatformSteps,
		CodesphereOnly:    true,
		DirectConnection:  opts.DirectConnection,
		AutoApprove:       opts.AutoApprove,
		LocalComponents:   opts.LocalComponents,
		LocalConfigDir:    opts.LocalConfigDir,
		SkipImageBuilding: opts.LocalComponents,
		SkipChartDeploy:   viaArgoCD,
		// The legacy installer picks the migrations to run from it. With ArgoCD there is no Helm
		// release it could read the version from.
		InstalledVersion: installedVersion,
	}
	if err := ci.Install(ctx, pm, cm, im, runtime.GOOS, runtime.GOARCH); err != nil {
		return fmt.Errorf("failed to install platform: %w", err)
	}

	if viaArgoCD {
		if err := csapps.Install(ctx, kubeClient, &cfg, installerBOM, opts.VaultSecretName); err != nil {
			return fmt.Errorf("failed to deploy Codesphere through ArgoCD: %w", err)
		}
	}

	return nil
}

// newKubeClient connects to the cluster the config and vault describe.
func newKubeClient(opts *InstallCodesphereOpts, cfg files.RootConfig) (client.Client, error) {
	_, restConfig, err := installer.VaultAndRESTConfig(opts.Vault, opts.PrivKey, opts.VaultType, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load the Kubernetes config: %w", err)
	}

	kubeClient, err := installer.NewKubeClient(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return kubeClient, nil
}

// readArgoCDBOM reads the package BOM and checks it has the charts the ArgoCD deploy needs.
func readArgoCDBOM(pm installer.PackageManager, opts *InstallCodesphereOpts) (*bom.Config, error) {
	if err := pm.ExtractDependency(bom.Filename, opts.Force, opts.Verbose); err != nil {
		return nil, fmt.Errorf("failed to extract the package BOM: %w", err)
	}

	installerBOM, err := bom.Parse(pm.GetDependencyPath(bom.Filename))
	if err != nil {
		return nil, fmt.Errorf("failed to read the package BOM: %w", err)
	}

	if _, ok := installerBOM.GetChart(csapps.CsAppsComponent); !ok {
		return nil, fmt.Errorf("%s chart not found in the package BOM", csapps.CsAppsComponent)
	}

	if _, ok := installerBOM.GetChart(csapps.CodesphereComponent); !ok {
		return nil, fmt.Errorf("%s chart not found in the package BOM", csapps.CodesphereComponent)
	}

	return installerBOM, nil
}

// ensureVaultSecret writes the vault Secret even though the dependencies phase already wrote
// cs-vault: The platform phase can pass another name (via --vault-secret-name), e.g. for dev
// preview deployments where we have multiple Codesphere installations on one cluster.
func ensureVaultSecret(ctx context.Context, opts *InstallCodesphereOpts, cfg files.RootConfig) error {
	installVault, restConfig, err := installer.VaultAndRESTConfig(opts.Vault, opts.PrivKey, opts.VaultType, cfg)
	if err != nil {
		return fmt.Errorf("failed to load vault and Kubernetes config: %w", err)
	}

	kubeClient, err := installer.NewKubeClient(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	creator := vault.NewVaultSecretCreator(kubeClient)
	if err := creator.CreateSecretFromVault(ctx, installVault, vault.VaultSecretNamespace, opts.VaultSecretName); err != nil {
		return fmt.Errorf("failed to write vault secret %q: %w", opts.VaultSecretName, err)
	}

	return nil
}

func AddInstallCodespherePlatformCmd(codesphere *cobra.Command, opts *InstallCodesphereOpts) {
	platform := InstallCodespherePlatformCmd{
		cmd: &cobra.Command{
			Use:   "platform",
			Short: "Install the Codesphere platform (Phase 3)",
			Long: io.Long(`Install the Codesphere platform (Phase 3).
			Runs step: codesphere.
			Requires the infrastructure and dependencies phases to have completed successfully.`),
			Example: util.FormatExamples("install codesphere platform", []io.Example{
				{
					Cmd:  "-p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml",
					Desc: "Install Codesphere platform only",
				},
			}),
		},
		Opts: opts,
		Env:  env.NewEnv(),
	}

	addCodesphereViaArgoCDFlag(platform.cmd, opts)

	util.AddCmd(codesphere, platform.cmd)
	platform.cmd.RunE = util.CancelOnSignal(platform.Run)
}

// addCodesphereViaArgoCDFlag adds the flag only to the commands that deploy Codesphere.
func addCodesphereViaArgoCDFlag(cmd *cobra.Command, opts *InstallCodesphereOpts) {
	cmd.Flags().BoolVar(&opts.CodesphereViaArgoCD, "codesphere-via-argocd", false, "Deploy Codesphere through the cs-applications ArgoCD app of apps instead of Helm")
}
