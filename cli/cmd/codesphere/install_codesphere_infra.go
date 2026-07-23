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
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/spf13/cobra"
)

// InstallCodesphereInfraCmd runs only the infrastructure steps (Phase 1).
type InstallCodesphereInfraCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

func (c *InstallCodesphereInfraCmd) RunE(_ *cobra.Command, _ []string) error {
	effectiveOpts, _, cleanup, err := prepareInstallConfig(c.Opts, installer.NewConfig())
	if err != nil {
		return err
	}
	defer cleanup()

	return installCodesphereInfra(effectiveOpts, c.Env)
}

func installCodesphereInfra(opts *InstallCodesphereOpts, env env.Env) error {
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
		AllowedSteps:     installer.InfraSteps,
		DirectConnection: opts.DirectConnection,
		AutoApprove:      opts.AutoApprove,
	}
	if err := ci.Install(pm, cm, im, runtime.GOOS, runtime.GOARCH); err != nil {
		return fmt.Errorf("failed to install infra: %w", err)
	}
	return nil
}

func AddInstallCodesphereInfraCmd(codesphere *cobra.Command, opts *InstallCodesphereOpts) {
	infra := InstallCodesphereInfraCmd{
		cmd: &cobra.Command{
			Use:   "infra",
			Short: "Install Codesphere infrastructure (Phase 1)",
			Long: io.Long(`Install infrastructure dependencies for a Codesphere instance (Phase 1).
			Runs steps: copy-dependencies, extract-dependencies, load-container-images, sops, docker, postgres, ceph, kubernetes.`),
			Example: util.FormatExamples("install codesphere infra", []io.Example{
				{
					Cmd:  "-p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml",
					Desc: "Install infrastructure components only",
				},
				{
					Cmd:  "-p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s load-container-images",
					Desc: "Skip loading container images when using a lite package",
				},
			}),
		},
		Opts: opts,
		Env:  env.NewEnv(),
	}

	util.AddCmd(codesphere, infra.cmd)
	infra.cmd.RunE = infra.RunE
}
