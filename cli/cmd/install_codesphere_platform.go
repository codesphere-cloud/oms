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

// InstallCodespherePlatformCmd runs only the Codesphere platform step (Phase 3).
type InstallCodespherePlatformCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

func (c *InstallCodespherePlatformCmd) RunE(_ *cobra.Command, _ []string) error {
	effectiveOpts, _, cleanup, err := prepareInstallConfig(c.Opts, installer.NewConfig())
	if err != nil {
		return err
	}
	defer cleanup()

	return installCodespherePlatform(effectiveOpts, c.Env)
}

func installCodespherePlatform(opts *InstallCodesphereOpts, env env.Env) error {
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
		DirectConnection: opts.DirectConnection,
		AutoApprove:      opts.AutoApprove,
	}
	if err := ci.Install(pm, cm, im, runtime.GOOS, runtime.GOARCH); err != nil {
		return fmt.Errorf("failed to install platform: %w", err)
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
