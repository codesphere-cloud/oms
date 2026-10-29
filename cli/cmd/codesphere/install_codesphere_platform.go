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
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	if err := validateArgoCDConfig(effectiveOpts, cfg); err != nil {
		return err
	}

	if err := ensureCodespherePackageForVersion(effectiveOpts, cfg, c.Env); err != nil {
		return err
	}

	return installCodespherePlatform(cmd.Context(), effectiveOpts, cfg, c.Env)
}

func deploysViaArgoCD(opts *InstallCodesphereOpts, cfg files.RootConfig) bool {
	return opts.CodesphereViaArgoCD && !installer.IsStepSkipped(cfg, opts.SkipSteps, codesphereStep)
}

func validateArgoCDConfig(opts *InstallCodesphereOpts, cfg files.RootConfig) error {
	if !deploysViaArgoCD(opts, cfg) {
		return nil
	}

	if err := csapps.ValidateConfig(&cfg); err != nil {
		return fmt.Errorf("invalid config for deploying through ArgoCD: %w", err)
	}

	return nil
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

	viaArgoCD := deploysViaArgoCD(opts, cfg)

	var kubeClient client.Client

	if viaArgoCD {
		var err error

		kubeClient, err = handOverDeployToArgoCD(ctx, opts, cfg)
		if err != nil {
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

	if viaArgoCD {
		if err := deployCodesphereViaArgoCD(ctx, kubeClient, cfg, pm); err != nil {
			return fmt.Errorf("failed to deploy Codesphere through ArgoCD: %w", err)
		}
	}

	return nil
}

// handOverDeployToArgoCD tells the Codesphere installer to leave the chart to OMS, and which
// version ArgoCD has installed, before the installer migrates the database.
func handOverDeployToArgoCD(ctx context.Context, opts *InstallCodesphereOpts, cfg files.RootConfig) (client.Client, error) {
	_, restConfig, err := installer.VaultAndRESTConfig(opts.Vault, opts.PrivKey, opts.VaultType, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load the Kubernetes config: %w", err)
	}

	kubeClient, err := installer.NewKubeClient(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	installedVersion, err := csapps.InstalledVersion(ctx, kubeClient, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to read the installed Codesphere version: %w", err)
	}

	if err := csapps.HandOverToInstaller(opts.ConfigPath, installedVersion); err != nil {
		return nil, fmt.Errorf("failed to hand the chart deploy over to OMS: %w", err)
	}

	return kubeClient, nil
}

func deployCodesphereViaArgoCD(ctx context.Context, kubeClient client.Client, cfg files.RootConfig, pm installer.PackageManager) error {
	installerBOM, err := bom.Parse(pm.GetDependencyPath("bom.json"))
	if err != nil {
		return fmt.Errorf("failed to read the package BOM: %w", err)
	}

	if err := csapps.Install(ctx, kubeClient, &cfg, installerBOM, log.Printf); err != nil {
		return fmt.Errorf("failed to deploy the cs-applications: %w", err)
	}

	return nil
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

	util.AddCmd(codesphere, platform.cmd)
	platform.cmd.RunE = platform.RunE
}
