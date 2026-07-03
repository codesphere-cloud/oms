// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"runtime"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/clusteradmin"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
)

// InstallCodespherePlatformCmd runs only the Codesphere platform step (Phase 3).
type InstallCodespherePlatformCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

func (c *InstallCodespherePlatformCmd) RunE(_ *cobra.Command, _ []string) error {
	effectiveOpts, cfg, cleanup, err := prepareInstallConfig(c.Opts, installer.NewConfig())
	if err != nil {
		return err
	}
	defer cleanup()

	return installCodespherePlatform(effectiveOpts, cfg, c.Env)
}

func installCodespherePlatform(opts *InstallCodesphereOpts, cfg files.RootConfig, env env.Env) error {
	if err := ensureClusterAdminSecret(context.Background(), opts, cfg); err != nil {
		return fmt.Errorf("failed to set cluster admin email: %w", err)
	}

	workdir := env.GetOmsWorkdir()
	pm := installer.NewPackage(workdir, opts.Package)
	cm := installer.NewConfig()
	im := system.NewImage(context.Background())

	ci := &installer.CodesphereInstaller{
		ConfigPath:       opts.ConfigPath,
		VaultPath:        opts.Vault,
		PrivKey:          opts.PrivKey,
		Force:            opts.Force,
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

// ensureClusterAdminSecret applies the cluster admin email configured via
// codesphere.clusterAdminEmail to the cluster-admin-email secret before the
// platform is installed, so the auth-service finds it on first start.
// It is a no-op when the config does not set an email.
func ensureClusterAdminSecret(ctx context.Context, opts *InstallCodesphereOpts, cfg files.RootConfig) error {
	email := cfg.Codesphere.ClusterAdminEmail
	if email == "" {
		return nil
	}

	vaultPath, err := resolveVaultPath(opts.Vault, cfg)
	if err != nil {
		return err
	}
	vault, err := installer.LoadVaultData(vaultPath, opts.PrivKey)
	if err != nil {
		return fmt.Errorf("failed to load vault %s: %w", vaultPath, err)
	}
	kubeConfigContent, err := kubeConfigContentFromVault(vault)
	if err != nil {
		return err
	}
	restConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeConfigContent))
	if err != nil {
		return fmt.Errorf("failed to load kubernetes config from vault: %w", err)
	}
	clientset, _, err := util.NewClientsFromRESTConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return clusteradmin.AddClusterAdmin(ctx, clientset, clusteradmin.Opts{
		Email:           email,
		Namespace:       clusteradmin.DefaultNamespace,
		SecretName:      clusteradmin.DefaultSecretName,
		CreateNamespace: true,
	})
}

func AddInstallCodespherePlatformCmd(codesphere *cobra.Command, opts *InstallCodesphereOpts) {
	platform := InstallCodespherePlatformCmd{
		cmd: &cobra.Command{
			Use:   "platform",
			Short: "Install the Codesphere platform (Phase 3)",
			Long: io.Long(`Install the Codesphere platform (Phase 3).
			Runs step: codesphere.
			Requires the infrastructure and dependencies phases to have completed successfully.`),
			Example: formatExamples("install codesphere platform", []io.Example{
				{
					Cmd:  "-p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml",
					Desc: "Install Codesphere platform only",
				},
			}),
		},
		Opts: opts,
		Env:  env.NewEnv(),
	}

	AddCmd(codesphere, platform.cmd)
	platform.cmd.RunE = platform.RunE
}
