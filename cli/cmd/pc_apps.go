// Copyright (c) Codesphere Inc. SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"os"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// InstallPCAppsCmd represents the pc-apps command
type InstallPCAppsCmd struct {
	cmd  *cobra.Command
	Opts InstallPCAppsOpts
}

type InstallPCAppsOpts struct {
	*GlobalOptions
	Chart       string
	Version     string
	Namespace   string
	Username    string
	ValuesFiles []string
}

func (c *InstallPCAppsCmd) RunE(_ *cobra.Command, args []string) error {
	var password string
	if c.Opts.Username != "" {
		pw, err := c.resolvePassword()
		if err != nil {
			return err
		}
		password = pw
	}

	pcApps, err := installer.NewPCApps(
		c.Opts.Chart,
		c.Opts.Version,
		c.Opts.Namespace,
		c.Opts.Username,
		password,
		c.Opts.ValuesFiles,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize pc-apps installer: %w", err)
	}

	if err := pcApps.Install(context.Background()); err != nil {
		return fmt.Errorf("failed to install pc-apps: %w", err)
	}

	return nil
}

// resolvePassword reads the password from the OMS_REPO_PASSWORD environment variable,
// or prompts the user interactively if the env var is not set.
func (c *InstallPCAppsCmd) resolvePassword() (string, error) {
	if pw := os.Getenv("OMS_REPO_PASSWORD"); len(pw) != 0 {
		return pw, nil
	}

	fmt.Print("Registry password/token: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}
	if len(pw) == 0 {
		return "", fmt.Errorf("password is required; set OMS_REPO_PASSWORD or enter it when prompted")
	}
	return string(pw), nil
}

func AddPCAppsCmd(parentCmd *cobra.Command, opts *GlobalOptions) {
	pcApps := InstallPCAppsCmd{
		Opts: InstallPCAppsOpts{
			GlobalOptions: opts,
		},
	}
	pcApps.cmd = &cobra.Command{
		Use:   "pc-apps",
		Short: "Install the pc-apps Helm chart from a private OCI registry",
		Long: packageio.Long(`Install or upgrade the pc-apps Helm chart from a private OCI registry
			into the target cluster. This chart deploys ArgoCD Application resources
			that manage the platform components.

			If --username is provided, the registry password is read from the
			OMS_REPO_PASSWORD environment variable or prompted interactively.
			Otherwise, credentials are read from the Kubernetes secret
			"argocd-codesphere-oci-read" in the argocd namespace (created by
			"oms beta install argocd").`),
		Example: formatExamples("beta install pc-apps", []packageio.Example{
			{Cmd: "--version 1.0.0", Desc: "Install a specific version (credentials from K8s secret)"},
			{Cmd: "--version 1.0.0 --username CodesphereBot", Desc: "Install with explicit registry credentials (prompts for password)"},
			{Cmd: "--chart oci://ghcr.io/codesphere-cloud/charts/pc-apps --version 1.0.0 -f base.yaml -f dc-overlay.yaml", Desc: "Install with custom chart and values files"},
		}),
	}
	pcApps.cmd.Flags().StringVar(&pcApps.Opts.Chart, "chart", "oci://ghcr.io/codesphere-cloud/charts/pc-apps", "Full OCI chart URL")
	pcApps.cmd.Flags().StringVar(&pcApps.Opts.Version, "version", "", "Chart version to install (default: latest)")
	pcApps.cmd.Flags().StringVar(&pcApps.Opts.Namespace, "namespace", "argocd", "Target namespace for the Helm release")
	pcApps.cmd.Flags().StringVar(&pcApps.Opts.Username, "username", "", "Username for OCI registry authentication (if omitted, reads from K8s secret)")
	pcApps.cmd.Flags().StringArrayVarP(&pcApps.Opts.ValuesFiles, "values", "f", nil, "Path to values YAML file (can be specified multiple times, merged in order)")
	pcApps.cmd.RunE = pcApps.RunE

	AddCmd(parentCmd, pcApps.cmd)
}
