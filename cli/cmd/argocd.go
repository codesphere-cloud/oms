// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/spf13/cobra"
)

type ArgoCDCmd struct {
	cmd *cobra.Command
}

// InstallArgoCDCmd represents the argocd command
type InstallArgoCDCmd struct {
	cmd  *cobra.Command
	Opts InstallArgoCDOpts
}

type InstallArgoCDOpts struct {
	*GlobalOptions
	Version string
	Package string
	Config  string
	Force   bool
}

func (c *InstallArgoCDCmd) RunE(_ *cobra.Command, args []string) error {
	argocd := installer.NewArgoCD()
	// err := argocd.PreInstall()
	err := argocd.Install()
	if err != nil {
		return fmt.Errorf("failed to install chart ArgoCD: %w", err)
	}

	return nil
}

type GetAdminPasswordCmd struct {
	cmd *cobra.Command
}

func (c *GetAdminPasswordCmd) RunE(_ *cobra.Command, args []string) error {
	return nil
}

func AddArgoCDCmd(parentCmd *cobra.Command, opts *GlobalOptions) {
	argocd := ArgoCDCmd{
		cmd: &cobra.Command{
			Use:   "argocd",
			Short: "Commands to interact with ArgoCD",
		},
	}

	// argocd install
	install := InstallArgoCDCmd{
		cmd: &cobra.Command{
			Use:   "install",
			Short: "Install an ArgoCD helm release",
			Long:  io.Long(`Install an ArgoCD helm release`),
			Example: formatExamplesWithBinary("install ArgoCD", []packageio.Example{
				{Cmd: "", Desc: "Install an ArgoCD helm release of chart https://argoproj.github.io/argo-helm/argo-cd "},
				{Cmd: "--version <version>", Desc: "Version of the ArgoCD helm chart to install"},
			}, "oms-cli"),
		},
	}
	install.cmd.Flags().StringVarP(&install.Opts.Version, "version", "v", "", "Version of the ArgoCD helm chart to install")
	install.cmd.RunE = install.RunE
	argocd.cmd.AddCommand(install.cmd)

	// argocd get-admin-password
	getAdminPassword := GetAdminPasswordCmd{
		cmd: &cobra.Command{
			Use:   "get-admin-password",
			Short: "Retrieve the initial ArgoCD admin password",
		},
	}
	getAdminPassword.cmd.RunE = getAdminPassword.RunE
	argocd.cmd.AddCommand(getAdminPassword.cmd)

	parentCmd.AddCommand(argocd.cmd)
}
