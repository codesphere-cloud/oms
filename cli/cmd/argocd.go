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
}

func (c *InstallArgoCDCmd) RunE(_ *cobra.Command, args []string) error {
	install := installer.NewArgoCD(c.Opts.Version, c.Opts.DatacenterId, c.Opts.RegistryPassword, c.Opts.GitPassword)
	err := install.Install()
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
	argocd.cmd.Flags().StringVarP(&argocd.Opts.GitPassword, "git-password", "c", "", "Password/token to read from the git repo where ArgoCD Application manifests are stored")
	_ = argocd.cmd.MarkFlagRequired("git-password")
	argocd.cmd.Flags().StringVar(&argocd.Opts.RegistryPassword, "registry-password", "", "Password/token to read from the OCI registry (e.g. ghcr.io) where Helm chart artifacts are stored")
	_ = argocd.cmd.MarkFlagRequired("registry-password")
	argocd.cmd.Flags().StringVar(&argocd.Opts.DatacenterId, "dc-id", "", "Codesphere Datacenter ID where this ArgoCD is installed")
	_ = argocd.cmd.MarkFlagRequired("dc-id")
	argocd.cmd.Flags().StringVarP(&argocd.Opts.Version, "version", "v", "", "Version of the ArgoCD helm chart to install")
	argocd.cmd.RunE = argocd.RunE

	parentCmd.AddCommand(argocd.cmd)
}
