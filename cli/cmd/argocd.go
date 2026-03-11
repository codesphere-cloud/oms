// Copyright (c) Codesphere Inc. SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/spf13/cobra"
)

// InstallArgoCDCmd represents the argocd command
type InstallArgoCDCmd struct {
	cmd  *cobra.Command
	Opts InstallArgoCDOpts
}

type InstallArgoCDOpts struct {
	*GlobalOptions
	Version          string
	DatacenterId     string
	GitPassword      string
	RegistryPassword string
	FullInstall      bool
}

func (c *InstallArgoCDCmd) RunE(_ *cobra.Command, args []string) error {
	if c.Opts.FullInstall {
		requiredFlags := map[string]string{
			"git-password":      c.Opts.GitPassword,
			"registry-password": c.Opts.RegistryPassword,
			"dc-id":             c.Opts.DatacenterId,
		}

		for flagName, value := range requiredFlags {
			if value == "" {
				return fmt.Errorf("flag --%s is required when --full-install is true", flagName)
			}
		}
	}
	install, err := installer.NewArgoCD(c.Opts.Version, c.Opts.DatacenterId, c.Opts.RegistryPassword, c.Opts.GitPassword, c.Opts.FullInstall)
	if err != nil {
		return fmt.Errorf("failed to initialize ArgoCD installer")
	}
	err = install.Install()
	if err != nil {
		return fmt.Errorf("failed to install chart ArgoCD: %w", err)
	}

	return nil
}

func AddArgoCDCmd(parentCmd *cobra.Command, opts *GlobalOptions) {
	argocd := InstallArgoCDCmd{
		cmd: &cobra.Command{
			Use:   "argocd",
			Short: "Install an ArgoCD helm release",
			Long:  packageio.Long(`Install an ArgoCD helm release`),
			Example: formatExamples("install ArgoCD", []packageio.Example{
				{Cmd: "", Desc: "Install an ArgoCD helm release of chart https://argoproj.github.io/argo-helm/argo-cd "},
				{Cmd: "--version <version>", Desc: "Version of the ArgoCD helm chart to install"},
			}),
		},
	}
	argocd.cmd.Flags().StringVar(&argocd.Opts.GitPassword, "git-password", "", "Password/token to read from the git repo where ArgoCD Application manifests are stored")
	argocd.cmd.Flags().StringVar(&argocd.Opts.RegistryPassword, "registry-password", "", "Password/token to read from the OCI registry (e.g. ghcr.io) where Helm chart artifacts are stored")
	argocd.cmd.Flags().StringVar(&argocd.Opts.DatacenterId, "dc-id", "", "Codesphere Datacenter ID where this ArgoCD is installed")
	argocd.cmd.Flags().StringVarP(&argocd.Opts.Version, "version", "v", "", "Version of the ArgoCD helm chart to install")
	argocd.cmd.Flags().BoolVar(&argocd.Opts.FullInstall, "full-install", false, "Install other resources (AppProjects, Repo Creds, ...) after installing the chart")
	argocd.cmd.RunE = argocd.RunE

	parentCmd.AddCommand(argocd.cmd)
}
