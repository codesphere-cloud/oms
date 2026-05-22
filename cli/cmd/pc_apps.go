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
	requiredFlags := map[string]string{
		"chart":    c.Opts.Chart,
		"username": c.Opts.Username,
	}

	for flagName, value := range requiredFlags {
		if value == "" {
			return fmt.Errorf("flag --%s is required", flagName)
		}
	}

	password, err := c.resolvePassword()
	if err != nil {
		return err
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
		cmd: &cobra.Command{
			Use:   "pc-apps",
			Short: "Install the pc-apps Helm chart from a private OCI registry",
			Long: packageio.Long(`Install or upgrade the pc-apps Helm chart from a private OCI registry
				into the target cluster. This chart deploys ArgoCD Application resources
				that manage the platform components.

				The registry password is read from the OMS_REPO_PASSWORD environment variable.
				If not set, it will be prompted interactively (hidden input).`),
			Example: formatExamples("install pc-apps", []packageio.Example{
				{Cmd: "--chart oci://ghcr.io/codesphere-cloud/charts/pc-apps --version 1.0.0 --username CodesphereBot", Desc: "Install a specific version (prompts for password)"},
				{Cmd: "--chart oci://ghcr.io/codesphere-cloud/charts/pc-apps --username CodesphereBot -f base.yaml -f dc-overlay.yaml", Desc: "Install latest with multiple values files"},
				{Cmd: "--chart oci://ghcr.io/codesphere-cloud/charts/pc-apps --username CodesphereBot --namespace custom-ns", Desc: "Install into a custom namespace"},
			}),
		},
	}
	pcApps.cmd.Flags().StringVar(&pcApps.Opts.Chart, "chart", "", "Full OCI chart URL (e.g. oci://ghcr.io/codesphere-cloud/charts/pc-apps)")
	pcApps.cmd.Flags().StringVar(&pcApps.Opts.Version, "version", "", "Chart version to install (default: latest)")
	pcApps.cmd.Flags().StringVar(&pcApps.Opts.Namespace, "namespace", "argocd", "Target namespace for the Helm release")
	pcApps.cmd.Flags().StringVar(&pcApps.Opts.Username, "username", "", "Username for OCI registry authentication")
	pcApps.cmd.Flags().StringArrayVarP(&pcApps.Opts.ValuesFiles, "values", "f", nil, "Path to values YAML file (can be specified multiple times, merged in order)")
	pcApps.cmd.RunE = pcApps.RunE

	AddCmd(parentCmd, pcApps.cmd)
}
