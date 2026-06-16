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

// InstallCodesphereDepenciesCmd runs the cluster dependency steps (Phase 2).
type InstallCodesphereDepenciesCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

func (c *InstallCodesphereDepenciesCmd) RunE(_ *cobra.Command, _ []string) error {
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
		AllowedSteps:     installer.DependenciesSteps,
		DirectConnection: c.Opts.DirectConnection,
		AutoApprove:      c.Opts.AutoApprove,
	}
	if err := ci.Install(pm, cm, im, runtime.GOOS, runtime.GOARCH); err != nil {
		return fmt.Errorf("failed to install dependencies: %w", err)
	}
	return nil
}

func AddInstallCodesphereDepenciesCmd(codesphere *cobra.Command, opts *GlobalOptions) {
	deps := InstallCodesphereDepenciesCmd{
		cmd: &cobra.Command{
			Use:   "dependencies",
			Short: "Install Codesphere cluster dependencies (Phase 2)",
			Long: io.Long(`Install cluster dependencies for a Codesphere instance (Phase 2).
			Runs steps: set-up-cluster, ms-backends.
			Requires the infrastructure phase to have completed successfully.`),
			Example: formatExamples("install codesphere dependencies", []io.Example{
				{
					Cmd:  "-p codesphere-v1.2.3-installer.tar.gz -k <path-to-private-key> -c config.yaml",
					Desc: "Install cluster dependencies only",
				},
			}),
		},
		Opts: &InstallCodesphereOpts{GlobalOptions: opts},
		Env:  env.NewEnv(),
	}
	deps.cmd.Flags().StringVarP(&deps.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load binaries, installer etc. from")
	deps.cmd.Flags().BoolVarP(&deps.Opts.Force, "force", "f", false, "Enforce package extraction")
	deps.cmd.Flags().StringVarP(&deps.Opts.Config, "config", "c", "", "Path to the Codesphere Private Cloud configuration file (yaml)")
	deps.cmd.Flags().StringVar(&deps.Opts.Vault, "vault", "prod.vault.yaml", "Path to the SOPS-encrypted prod.vault.yaml file used for config templating")
	deps.cmd.Flags().StringVarP(&deps.Opts.PrivKey, "priv-key", "k", "", "Path to the private key to encrypt/decrypt secrets")
	deps.cmd.Flags().StringSliceVarP(&deps.Opts.SkipSteps, "skip-steps", "s", []string{}, "Dependencies steps to skip. E.g. set-up-cluster, ms-backends")
	deps.cmd.Flags().BoolVar(&deps.Opts.DirectConnection, "direct-connection", false, "Use direct connection for installation, requires having access to the cluster nodes from your machine")
	deps.cmd.Flags().BoolVar(&deps.Opts.AutoApprove, "auto-approve", true, "Auto approve confirmation prompts with default values")

	util.MarkFlagRequired(deps.cmd, "package")
	util.MarkFlagRequired(deps.cmd, "config")
	util.MarkFlagRequired(deps.cmd, "priv-key")

	AddCmd(codesphere, deps.cmd)
	deps.cmd.RunE = deps.RunE
}
