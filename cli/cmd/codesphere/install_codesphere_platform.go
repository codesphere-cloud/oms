// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
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
)

const codesphereStep = "codesphere"

// InstallCodespherePlatformCmd runs only the Codesphere platform step (Phase 3).
type InstallCodespherePlatformCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

func (c *InstallCodespherePlatformCmd) RunE(cmd *cobra.Command, _ []string) error {
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

	if err := ensureCodespherePackageForVersion(effectiveOpts, cfg, c.Env); err != nil {
		return err
	}

	return installCodespherePlatform(cmd.Context(), effectiveOpts, cfg, c.Env)
}

func installCodespherePlatform(ctx context.Context, opts *InstallCodesphereOpts, cfg files.RootConfig, env env.Env) error {
	if err := installer.EnsureClusterAdminSecret(ctx, opts.Vault, opts.PrivKey, opts.VaultType, cfg); err != nil {
		return fmt.Errorf("failed to set cluster admin email: %w", err)
	}

	// A skipped deploy must not rewrite the extra Secret: its freshly signed tokens would make
	// Reloader restart the running release.
	if !installer.IsStepSkipped(cfg, opts.SkipSteps, installer.CodesphereStep) {
		if err := prepareChartVaultSecret(ctx, opts, cfg); err != nil {
			return err
		}
	}

	workdir := env.GetOmsWorkdir()
	pm := installer.NewPackage(workdir, opts.Package)
	if opts.LocalComponents {
		pm = installer.NewPackage(filepath.Dir(opts.Package), opts.Package)
	}
	cm := installer.NewConfig()
	im := system.NewImage(ctx)

	ci := &installer.CodesphereInstaller{
		ConfigPath:        opts.ConfigPath,
		VaultPath:         opts.Vault,
		PrivKey:           opts.PrivKey,
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
		Context:           ctx,
	}
	if err := ci.Install(pm, cm, im, runtime.GOOS, runtime.GOARCH); err != nil {
		return fmt.Errorf("failed to install platform: %w", err)
	}

	if opts.CodesphereViaArgoCD && !installer.IsStepSkipped(cfg, opts.SkipSteps, codesphereStep) {
		if err := deployCodesphereViaArgoCD(ctx, opts, cfg, pm); err != nil {
			return fmt.Errorf("failed to deploy Codesphere through ArgoCD: %w", err)
		}
	}

	return nil
}

func deployCodesphereViaArgoCD(ctx context.Context, opts *InstallCodesphereOpts, cfg files.RootConfig, pm installer.PackageManager) error {
	installerBOM, err := bom.Parse(pm.GetDependencyPath("bom.json"))
	if err != nil {
		return fmt.Errorf("failed to read the package BOM: %w", err)
	}

	_, restConfig, err := installer.VaultAndRESTConfig(opts.Vault, opts.PrivKey, opts.VaultType, cfg)
	if err != nil {
		return fmt.Errorf("failed to load the Kubernetes config: %w", err)
	}

	kubeClient, err := installer.NewKubeClient(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	if err := csapps.Install(ctx, kubeClient, &cfg, installerBOM, log.Printf); err != nil {
		return fmt.Errorf("failed to deploy the cs-applications: %w", err)
	}

	return nil
}

// prepareChartVaultSecret writes the extra vault Secret if one is requested and passes a new
// sync trigger to the chart through the merged config.
func prepareChartVaultSecret(
	ctx context.Context,
	opts *InstallCodesphereOpts,
	cfg files.RootConfig,
) error {
	if opts.ExtraVaultSecret != "" {
		if err := writeExtraVaultSecret(ctx, opts, cfg); err != nil {
			return err
		}
	}

	if err := vault.WriteSyncTriggerToConfig(opts.ConfigPath, rand.Text()); err != nil {
		return fmt.Errorf("failed to pass the sync trigger to the chart: %w", err)
	}

	return nil
}

func writeExtraVaultSecret(ctx context.Context, opts *InstallCodesphereOpts, cfg files.RootConfig) error {
	installVault, restConfig, err := installer.VaultAndRESTConfig(opts.Vault, opts.PrivKey, opts.VaultType, cfg)
	if err != nil {
		return fmt.Errorf("failed to load vault and Kubernetes config: %w", err)
	}

	kubeClient, err := installer.NewKubeClient(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	creator := vault.NewVaultSecretCreator(kubeClient)
	if err := creator.CreateSecretFromVault(ctx, installVault, vault.VaultSecretNamespace, opts.ExtraVaultSecret); err != nil {
		return fmt.Errorf("failed to write extra vault secret: %w", err)
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

	util.AddCmd(codesphere, platform.cmd)
	platform.cmd.RunE = platform.RunE
}
