// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"context"
	"fmt"
	"runtime"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/spf13/cobra"
)

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

	if installer.UsesExternalSecrets(cfg.Codesphere) {
		if err := writeReleaseVault(ctx, opts, cfg); err != nil {
			return err
		}
	}

	workdir := env.GetOmsWorkdir()
	pm := installer.NewPackage(workdir, opts.Package)
	cm := installer.NewConfig()
	im := system.NewImage(ctx)

	ci := &installer.CodesphereInstaller{
		ConfigPath:       opts.ConfigPath,
		VaultPath:        opts.Vault,
		PrivKey:          opts.PrivKey,
		Force:            opts.Force,
		Verbose:          opts.Verbose,
		SkipSteps:        opts.SkipSteps,
		AllowedSteps:     installer.PlatformSteps,
		CodesphereOnly:   true,
		DirectConnection: opts.DirectConnection,
		AutoApprove:      opts.AutoApprove,
	}
	if err := ci.Install(pm, cm, im, runtime.GOOS, runtime.GOARCH); err != nil {
		return fmt.Errorf("failed to install platform: %w", err)
	}
	return nil
}

// writeReleaseVault writes the vault into the release's own secret, which the umbrella chart's
// ExternalSecrets read, and passes its checksum to the chart through the merged config.
func writeReleaseVault(
	ctx context.Context,
	opts *InstallCodesphereOpts,
	cfg files.RootConfig,
) error {
	installVault, restConfig, err := installer.VaultAndRESTConfig(opts.Vault, opts.PrivKey, opts.VaultType, cfg)
	if err != nil {
		return fmt.Errorf("failed to load vault and Kubernetes config: %w", err)
	}

	kubeClient, err := installer.NewKubeClient(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	checksum, err := installer.WriteReleaseVaultSecret(ctx, kubeClient, installVault, cfg.Codesphere)
	if err != nil {
		return fmt.Errorf("failed to write release vault secret: %w", err)
	}

	if err := installer.AddExternalSecretsChecksum(opts.ConfigPath, checksum); err != nil {
		return fmt.Errorf("failed to pass the vault checksum to the chart: %w", err)
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
