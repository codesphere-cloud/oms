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

// InstallCodesphereInfraCmd runs only the infrastructure steps (Phase 1).
type InstallCodesphereInfraCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

func (c *InstallCodesphereInfraCmd) RunE(_ *cobra.Command, _ []string) error {
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
		AllowedSteps:     installer.InfraSteps,
		DirectConnection: c.Opts.DirectConnection,
		AutoApprove:      c.Opts.AutoApprove,
	}
	if err := ci.Install(pm, cm, im, runtime.GOOS, runtime.GOARCH); err != nil {
		return fmt.Errorf("failed to install infra: %w", err)
	}
	return nil
}

func AddInstallCodesphereInfraCmd(codesphere *cobra.Command, opts *GlobalOptions) {
	infra := InstallCodesphereInfraCmd{
		cmd: &cobra.Command{
			Use:   "infra",
			Short: "Install Codesphere infrastructure (Phase 1)",
			Long: io.Long(`Install infrastructure dependencies for a Codesphere instance (Phase 1).
			Runs steps: copy-dependencies, extract-dependencies, load-container-images, sops, docker, postgres, ceph, kubernetes.`),
			Example: formatExamples("install codesphere infra", []io.Example{
				{
					Cmd:  "-p codesphere-v1.2.3-installer.tar.gz -k <path-to-private-key> -c config.yaml",
					Desc: "Install infrastructure components only",
				},
				{
					Cmd:  "-p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s load-container-images",
					Desc: "Skip loading container images when using a lite package",
				},
			}),
		},
		Opts: &InstallCodesphereOpts{GlobalOptions: opts},
		Env:  env.NewEnv(),
	}
	infra.cmd.Flags().StringVarP(&infra.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load binaries, installer etc. from")
	infra.cmd.Flags().BoolVarP(&infra.Opts.Force, "force", "f", false, "Enforce package extraction")
	infra.cmd.Flags().StringVarP(&infra.Opts.Config, "config", "c", "", "Path to the Codesphere Private Cloud configuration file (yaml)")
	infra.cmd.Flags().StringVar(&infra.Opts.Vault, "vault", "prod.vault.yaml", "Path to the SOPS-encrypted prod.vault.yaml file used for config templating")
	infra.cmd.Flags().StringVarP(&infra.Opts.PrivKey, "priv-key", "k", "", "Path to the private key to encrypt/decrypt secrets")
	infra.cmd.Flags().StringSliceVarP(&infra.Opts.SkipSteps, "skip-steps", "s", []string{}, "Infra steps to skip. E.g. copy-dependencies, load-container-images, ceph, kubernetes")
	infra.cmd.Flags().BoolVar(&infra.Opts.DirectConnection, "direct-connection", false, "Use direct connection for installation, requires having access to the cluster nodes from your machine")
	infra.cmd.Flags().BoolVar(&infra.Opts.AutoApprove, "auto-approve", true, "Auto approve confirmation prompts with default values")

	util.MarkFlagRequired(infra.cmd, "package")
	util.MarkFlagRequired(infra.cmd, "config")
	util.MarkFlagRequired(infra.cmd, "priv-key")

	AddCmd(codesphere, infra.cmd)
	infra.cmd.RunE = infra.RunE
}
