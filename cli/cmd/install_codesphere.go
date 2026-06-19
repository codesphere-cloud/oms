// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
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
	// ArgoCD deployment (pre-step in Phase 2)
	ArgoCDVersion        string
	ArgoCDRegistryURL    string
	ArgoCDForceConflicts bool
	ArgoCDRepoURL        string
	ArgoCDValues         []string
}

func (c *InstallCodesphereCmd) RunE(_ *cobra.Command, _ []string) error {
	cfg, cleanup, err := parseInstallConfig(c.Opts, installer.NewConfig())
	if err != nil {
		return err
	}
	defer cleanup()

	infraInstaller := &installer.CodesphereInstaller{
		SkipSteps:    c.Opts.SkipSteps,
		AllowedSteps: installer.InfraSteps,
	}
	dependenciesInstaller := &installer.CodesphereInstaller{
		SkipSteps:    c.Opts.SkipSteps,
		AllowedSteps: installer.DependenciesSteps,
	}
	platformInstaller := &installer.CodesphereInstaller{
		SkipSteps:    c.Opts.SkipSteps,
		AllowedSteps: installer.PlatformSteps,
	}

	if c.Opts.CodesphereOnly {
		return installCodespherePlatform(c.Opts, c.Env)
	}

	if len(infraInstaller.ExecutableSteps(cfg)) > 0 {
		if err := installCodesphereInfra(c.Opts, c.Env); err != nil {
			return err
		}
	}

	if len(dependenciesInstaller.ExecutableSteps(cfg)) > 0 || !installer.IsStepSkipped(cfg, c.Opts.SkipSteps, installer.ArgoCDStep) {
		if err := installCodesphereDepencies(c.Opts, c.Env); err != nil {
			return err
		}
	}

	if len(platformInstaller.ExecutableSteps(cfg)) == 0 {
		return nil
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
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.Vault, "vault", "", "Path to the SOPS-encrypted prod.vault.yaml file used for config templating")
	codesphere.cmd.PersistentFlags().StringVarP(&codesphere.Opts.PrivKey, "priv-key", "k", "", "Path to the private key to encrypt/decrypt secrets")
	codesphere.cmd.PersistentFlags().StringSliceVarP(&codesphere.Opts.SkipSteps, "skip-steps", "s", []string{}, "Steps to be skipped. E.g. copy-dependencies, extract-dependencies, load-container-images, ceph, postgres, kubernetes, docker, argocd")
	codesphere.cmd.PersistentFlags().BoolVar(&codesphere.Opts.DirectConnection, "direct-connection", false, "Use direct connection for installation, requires having access to the cluster nodes from your machine")
	codesphere.cmd.PersistentFlags().BoolVar(&codesphere.Opts.AutoApprove, "auto-approve", true, "Auto approve confirmation prompts with default values")
	codesphere.cmd.Flags().BoolVar(&codesphere.Opts.CodesphereOnly, "codesphere-only", false, "Install only Codesphere without dependencies")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.ArgoCDVersion, "argo-version", "", "ArgoCD Helm chart version to install")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.ArgoCDRegistryURL, "argo-registry-url", "", "OCI registry URL for the ArgoCD Helm chart (defaults to registry.server from config.yaml)")
	codesphere.cmd.PersistentFlags().BoolVar(&codesphere.Opts.ArgoCDForceConflicts, "argo-force-conflicts", false, "Force SSA ownership conflicts during ArgoCD install")
	codesphere.cmd.PersistentFlags().StringVar(&codesphere.Opts.ArgoCDRepoURL, "argo-repo", argocd.DefaultRepoURL, "ArgoCD Helm chart repository URL")
	codesphere.cmd.PersistentFlags().StringArrayVar(&codesphere.Opts.ArgoCDValues, "argo-values", nil, "ArgoCD values YAML file (can be specified multiple times)")

	util.MarkPersistentFlagRequired(codesphere.cmd, "package")
	util.MarkPersistentFlagRequired(codesphere.cmd, "config")
	util.MarkPersistentFlagRequired(codesphere.cmd, "priv-key")

	AddCmd(install, codesphere.cmd)

	codesphere.cmd.RunE = codesphere.RunE

	AddInstallCodesphereInfraCmd(codesphere.cmd, codesphere.Opts)
	AddInstallCodesphereDepenciesCmd(codesphere.cmd, codesphere.Opts)
	AddInstallCodespherePlatformCmd(codesphere.cmd, codesphere.Opts)
}
