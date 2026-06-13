// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	argocdinstaller "github.com/codesphere-cloud/oms/internal/argocd"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// InstallArgoCDCmd represents the argocd command
type InstallArgoCDCmd struct {
	cmd  *cobra.Command
	Opts InstallArgoCDOpts
}

type InstallArgoCDOpts struct {
	*GlobalOptions
	Version        string
	DatacenterId   string
	RegistryURL    string
	FullInstall    bool
	ForceConflicts bool
	RepoURL        string
	ValueFiles     []string
}

func (c *InstallArgoCDCmd) RunE(_ *cobra.Command, args []string) error {
	var ociPassword, gitPassword string

	if c.Opts.FullInstall {
		pw, err := resolveOCIPassword()
		if err != nil {
			return err
		}
		ociPassword = pw
		gitPassword = os.Getenv("OMS_GIT_PASSWORD")
	}

	install, err := argocdinstaller.NewInstaller(argocdinstaller.InstallerConfig{
		Version:        c.Opts.Version,
		DatacenterId:   c.Opts.DatacenterId,
		OciPassword:    ociPassword,
		OciRegistryURL: c.Opts.RegistryURL,
		GitPassword:    gitPassword,
		FullInstall:    c.Opts.FullInstall,
		ForceConflicts: c.Opts.ForceConflicts,
		RepoURL:        c.Opts.RepoURL,
		ValueFiles:     c.Opts.ValueFiles,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize ArgoCD installer: %w", err)
	}
	err = install.Install()
	if err != nil {
		return fmt.Errorf("failed to install chart ArgoCD: %w", err)
	}

	return nil
}

// resolveOCIPassword reads the OCI registry password from the OMS_REGISTRY_PASSWORD
// environment variable, or prompts the user interactively if not set.
func resolveOCIPassword() (string, error) {
	if pw := os.Getenv("OMS_REGISTRY_PASSWORD"); pw != "" {
		return pw, nil
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("OMS_REGISTRY_PASSWORD must be set in non-interactive environments")
	}

	fmt.Print("OCI registry password/token: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}
	if len(pw) == 0 {
		return "", fmt.Errorf("password is required; set OMS_REGISTRY_PASSWORD or enter it when prompted")
	}
	return string(pw), nil
}

func AddArgoCDCmd(parentCmd *cobra.Command, opts *GlobalOptions) {
	argocd := InstallArgoCDCmd{
		cmd: &cobra.Command{
			Use:   "argocd",
			Short: "Install an ArgoCD helm release",
			Long: packageio.Long(`Install or upgrade the ArgoCD helm release.

				When --deploy-dc-config is set, Codesphere-managed resources are applied after
				the chart install/upgrade:
				  - AppProjects (always)
				  - Helm OCI registry secret (always, requires OMS_REGISTRY_PASSWORD)
				  - Local cluster secret (only if --dc-id is provided)
				  - Git repo credentials (only if OMS_GIT_PASSWORD env var is set)

				Use --registry-url to point to a custom or mirrored OCI registry (defaults
				to ghcr.io/codesphere-cloud/charts).

				Environment variables:
				  OMS_REGISTRY_PASSWORD  Password/token for the Helm OCI registry (required for --deploy-dc-config)
				  OMS_GIT_PASSWORD       Password/token for git repo access (optional)`),
			Example: formatExamples("beta install argocd", []packageio.Example{
				{Cmd: "", Desc: "Install ArgoCD helm chart only"},
				{Cmd: "--version 7.8.0", Desc: "Install a specific chart version"},
				{Cmd: "--deploy-dc-config", Desc: "Install chart and apply Codesphere resources (prompts for OCI password)"},
				{Cmd: "--deploy-dc-config --dc-id 0", Desc: "Also register the local cluster as dc-0"},
			}),
		},
	}
	argocd.cmd.Flags().StringVar(&argocd.Opts.DatacenterId, "dc-id", "", "Codesphere Datacenter ID (optional, registers local cluster in ArgoCD)")
	argocd.cmd.Flags().StringVar(&argocd.Opts.RegistryURL, "registry-url", "ghcr.io/codesphere-cloud/charts", "OCI registry URL for the Helm chart repository")
	argocd.cmd.Flags().StringVarP(&argocd.Opts.Version, "version", "v", "", "Version of the ArgoCD helm chart to install")
	argocd.cmd.Flags().BoolVar(&argocd.Opts.FullInstall, "deploy-dc-config", false, "Apply Codesphere-managed resources (AppProjects, Repo Creds, ...) after installing the chart")
	argocd.cmd.Flags().StringArrayVarP(&argocd.Opts.ValueFiles, "values", "f", nil, "Specify values in a YAML file (can be specified multiple times)")
	argocd.cmd.Flags().BoolVar(&argocd.Opts.ForceConflicts, "force-conflicts", false, "Force field ownership conflicts during upgrade (sets server-side apply ForceConflicts)")
	argocd.cmd.Flags().StringVar(&argocd.Opts.RepoURL, "repo", "", "Helm chart repository URL; supports HTTP (default: https://argoproj.github.io/argo-helm) and OCI (e.g. oci://ghcr.io/argoproj/argo-helm)")
	argocd.cmd.RunE = argocd.RunE

	AddCmd(parentCmd, argocd.cmd)
}
