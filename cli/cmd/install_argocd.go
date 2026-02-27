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

// InstallArgoCD represents the argocd command
type InstallArgoCD struct {
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

func (c *InstallArgoCD) RunE(_ *cobra.Command, args []string) error {
	argocd := installer.NewArgoCD()
	err := argocd.PreInstall()
	// err := argocd.Install()
	if err != nil {
		return fmt.Errorf("failed to install chart ArgoCD: %w", err)
	}

	return nil
}
func AddInstallArgoCD(install *cobra.Command, opts *GlobalOptions) {
	argocd := InstallArgoCD{
		cmd: &cobra.Command{
			Use:   "argocd",
			Short: "Install an ArgoCD helm release",
			Long:  io.Long(`Install an ArgoCD helm release`),
			Example: formatExamplesWithBinary("install ArgoCD", []packageio.Example{
				{Cmd: "", Desc: "Install an ArgoCD helm release of chart https://argoproj.github.io/argo-helm/argo-cd "},
				{Cmd: "--version <version>", Desc: "Version of the ArgoCD helm chart to install"},
			}, "oms-cli"),
		},
	}
	argocd.cmd.Flags().StringVarP(&argocd.Opts.Version, "version", "v", "", "Version of the ArgoCD helm chart to install")
	install.AddCommand(argocd.cmd)
	argocd.cmd.RunE = argocd.RunE
}
