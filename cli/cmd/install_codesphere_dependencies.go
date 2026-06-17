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
	"github.com/spf13/cobra"
)

// InstallCodesphereDepenciesCmd runs the cluster dependency steps (Phase 2).
type InstallCodesphereDepenciesCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

func (c *InstallCodesphereDepenciesCmd) RunE(_ *cobra.Command, _ []string) error {
	return installCodesphereDepencies(c.Opts, c.Env)
}

func installCodesphereDepencies(opts *InstallCodesphereOpts, env env.Env) error {
	workdir := env.GetOmsWorkdir()
	pm := installer.NewPackage(workdir, opts.Package)
	cm := installer.NewConfig()
	im := system.NewImage(context.Background())

	ci := &installer.CodesphereInstaller{
		ConfigPath:       opts.Config,
		VaultPath:        opts.Vault,
		PrivKey:          opts.PrivKey,
		Force:            opts.Force,
		SkipSteps:        opts.SkipSteps,
		AllowedSteps:     installer.DependenciesSteps,
		DirectConnection: opts.DirectConnection,
		AutoApprove:      opts.AutoApprove,
	}
	if err := ci.Install(pm, cm, im, runtime.GOOS, runtime.GOARCH); err != nil {
		return fmt.Errorf("failed to install dependencies: %w", err)
	}
	return nil
}

func AddInstallCodesphereDepenciesCmd(codesphere *cobra.Command, opts *InstallCodesphereOpts) {
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
		Opts: opts,
		Env:  env.NewEnv(),
	}

	AddCmd(codesphere, deps.cmd)
	deps.cmd.RunE = deps.RunE
}
