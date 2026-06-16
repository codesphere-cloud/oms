// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"runtime"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

// InstallCodespherePlatformCmd runs only the Codesphere platform step (Phase 3).
type InstallCodespherePlatformCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

func (c *InstallCodespherePlatformCmd) RunE(_ *cobra.Command, _ []string) error {
	workdir := c.Env.GetOmsWorkdir()
	pm := installer.NewPackage(workdir, c.Opts.Package)
	cm := installer.NewConfig()
	im := system.NewImage(context.Background())

	ci := &installer.CodesphereInstaller{
		ConfigPath:       c.Opts.Config,
		VaultPath:        c.Opts.Vault,
		PrivKey:          c.Opts.PrivKey,
		Force:            c.Opts.Force,
		SkipSteps:        c.Opts.SkipSteps,
		AllowedSteps:     installer.PlatformSteps,
		DirectConnection: c.Opts.DirectConnection,
		AutoApprove:      c.Opts.AutoApprove,
	}
	if err := ci.Install(pm, cm, im, runtime.GOOS, runtime.GOARCH); err != nil {
		return fmt.Errorf("failed to install platform: %w", err)
	}
	return nil
}

func AddInstallCodespherePlatformCmd(codesphere *cobra.Command, opts *GlobalOptions) {
	platform := InstallCodespherePlatformCmd{
		cmd: &cobra.Command{
			Use:   "platform",
			Short: "Install the Codesphere platform (Phase 3)",
			Long: io.Long(`Install the Codesphere platform (Phase 3).
			Runs step: codesphere.
			Requires the infrastructure and dependencies phases to have completed successfully.`),
			Example: formatExamples("install codesphere platform", []io.Example{
				{
					Cmd:  "-p codesphere-v1.2.3-installer.tar.gz -k <path-to-private-key> -c config.yaml",
					Desc: "Install Codesphere platform only",
				},
			}),
		},
		Opts: &InstallCodesphereOpts{GlobalOptions: opts},
		Env:  env.NewEnv(),
	}
	platform.cmd.Flags().StringVarP(&platform.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load binaries, installer etc. from")
	platform.cmd.Flags().BoolVarP(&platform.Opts.Force, "force", "f", false, "Enforce package extraction")
	platform.cmd.Flags().StringVarP(&platform.Opts.Config, "config", "c", "", "Path to the Codesphere Private Cloud configuration file (yaml)")
	platform.cmd.Flags().StringVar(&platform.Opts.Vault, "vault", "prod.vault.yaml", "Path to the SOPS-encrypted prod.vault.yaml file used for config templating")
	platform.cmd.Flags().StringVarP(&platform.Opts.PrivKey, "priv-key", "k", "", "Path to the private key to encrypt/decrypt secrets")
	platform.cmd.Flags().StringSliceVarP(&platform.Opts.SkipSteps, "skip-steps", "s", []string{}, "Platform steps to skip. E.g. codesphere")
	platform.cmd.Flags().BoolVar(&platform.Opts.DirectConnection, "direct-connection", false, "Use direct connection for installation, requires having access to the cluster nodes from your machine")
	platform.cmd.Flags().BoolVar(&platform.Opts.AutoApprove, "auto-approve", true, "Auto approve confirmation prompts with default values")

	util.MarkFlagRequired(platform.cmd, "package")
	util.MarkFlagRequired(platform.cmd, "config")
	util.MarkFlagRequired(platform.cmd, "priv-key")

	AddCmd(codesphere, platform.cmd)
	platform.cmd.RunE = platform.RunE
}
