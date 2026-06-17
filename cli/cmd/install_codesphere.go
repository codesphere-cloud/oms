// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/system"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

// InstallCodesphereCmd represents the codesphere command
type InstallCodesphereCmd struct {
	cmd  *cobra.Command
	Opts *InstallCodesphereOpts
	Env  env.Env
}

type InstallCodesphereOpts struct {
	*GlobalOptions
	Package          string
	Force            bool
	Config           string
	Vault            string
	PrivKey          string
	SkipSteps        []string
	CodesphereOnly   bool
	DirectConnection bool
	AutoApprove      bool
}

func (c *InstallCodesphereCmd) RunE(_ *cobra.Command, _ []string) error {
	if err := installCodesphereInfra(c.Opts, c.Env); err != nil {
		return err
	}
	if err := installCodesphereDepencies(c.Opts, c.Env); err != nil {
		return err
	}
	return installCodespherePlatform(c.Opts, c.Env)
}

func AddInstallCodesphereCmd(install *cobra.Command, opts *GlobalOptions) {
	codesphere := InstallCodesphereCmd{
		cmd: &cobra.Command{
			Use:   "codesphere",
			Short: "Install a Codesphere instance",
			Long: io.Long(`Install a Codesphere instance with the provided package, configuration file, and private key.
			Uses the private-cloud-installer.js script included in the package to perform the installation.`),
			Example: formatExamples("install codesphere", []io.Example{
				{
					Cmd:  "-p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s copy-dependencies,extract-dependencies,load-container-images,ceph,postgres,kubernetes,docker",
					Desc: "Skip most pre-installation steps. E.g. if you only need to re-apply Codesphere's helm charts",
				},
				{
					Cmd:  "-p codesphere-v1.2.3-installer-lite.tar.gz -k <path-to-private-key> -c config.yaml -s load-container-images",
					Desc: "Skip loading container images. Necessary when installing a lite package that doesn't include any container images",
				},
			}),
		},
		Opts: &InstallCodesphereOpts{GlobalOptions: opts},
		Env:  env.NewEnv(),
	}
	codesphere.cmd.PersistentFlags().StringVarP(&codesphere.Opts.Package, "package", "p", "", "Package file (e.g. codesphere-v1.2.3-installer.tar.gz) to load binaries, installer etc. from")
	codesphere.cmd.PersistentFlags().BoolVarP(&codesphere.Opts.Force, "force", "f", false, "Enforce package extraction")
	codesphere.cmd.PersistentFlags().StringVarP(&codesphere.Opts.Config, "config", "c", "", "Path to the Codesphere Private Cloud configuration file (yaml)")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.Vault, "vault", "prod.vault.yaml", "Path to the SOPS-encrypted prod.vault.yaml file used for config templating")
	codesphere.cmd.PersistentFlags().StringVarP(&codesphere.Opts.PrivKey, "priv-key", "k", "", "Path to the private key to encrypt/decrypt secrets")
	codesphere.cmd.PersistentFlags().StringSliceVarP(&codesphere.Opts.SkipSteps, "skip-steps", "s", []string{}, "Steps to be skipped. E.g. copy-dependencies, extract-dependencies, load-container-images, ceph, postgres, kubernetes, docker")
	codesphere.cmd.PersistentFlags().BoolVar(&codesphere.Opts.DirectConnection, "direct-connection", false, "Use direct connection for installation, requires having access to the cluster nodes from your machine")
	codesphere.cmd.PersistentFlags().BoolVar(&codesphere.Opts.AutoApprove, "auto-approve", true, "Auto approve confirmation prompts with default values")
	codesphere.cmd.Flags().BoolVar(&codesphere.Opts.CodesphereOnly, "codesphere-only", false, "Install only Codesphere without dependencies")

	util.MarkPersistentFlagRequired(codesphere.cmd, "package")
	util.MarkPersistentFlagRequired(codesphere.cmd, "config")
	util.MarkPersistentFlagRequired(codesphere.cmd, "priv-key")

	AddCmd(install, codesphere.cmd)

	codesphere.cmd.RunE = codesphere.RunE

	AddInstallCodesphereInfraCmd(codesphere.cmd, codesphere.Opts)
	AddInstallCodesphereDepenciesCmd(codesphere.cmd, codesphere.Opts)
	AddInstallCodespherePlatformCmd(codesphere.cmd, codesphere.Opts)
}

func (c *InstallCodesphereCmd) ExtractAndInstall(pm installer.PackageManager, cm installer.ConfigManager, im system.ImageManager, goos string, goarch string) error {
	type phaseConfig struct {
		steps             []string
		skipImageBuilding bool
	}
	phases := []phaseConfig{
		{steps: installer.InfraSteps, skipImageBuilding: false},
		{steps: installer.DependenciesSteps, skipImageBuilding: true},
		{steps: installer.PlatformSteps, skipImageBuilding: true},
	}
	for _, phase := range phases {
		ci := &installer.CodesphereInstaller{
			ConfigPath:        c.Opts.Config,
			VaultPath:         c.Opts.Vault,
			PrivKey:           c.Opts.PrivKey,
			Force:             c.Opts.Force,
			SkipSteps:         c.Opts.SkipSteps,
			AllowedSteps:      phase.steps,
			SkipImageBuilding: phase.skipImageBuilding,
			DirectConnection:  c.Opts.DirectConnection,
			AutoApprove:       c.Opts.AutoApprove,
		}
		if err := ci.Install(pm, cm, im, goos, goarch); err != nil {
			return err
		}
	}
	return nil
}

func (c *InstallCodesphereCmd) ListPackageContents(pm installer.PackageManager) ([]string, error) {
	return installer.ListPackageContents(pm)
}
