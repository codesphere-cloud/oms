// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

// InstallPCAppsCmd represents the pc-apps command
type InstallPCAppsCmd struct {
	cmd  *cobra.Command
	Opts InstallPCAppsOpts
}

type InstallPCAppsOpts struct {
	*GlobalOptions
	Version        string
	Namespace      string
	ValuesFiles    []string
	ForceConflicts bool
}

func (c *InstallPCAppsCmd) RunE(cmd *cobra.Command, args []string) error {
	kubeConfig, err := ctrlconfig.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to load kubernetes config: %w", err)
	}

	kubeClient, err := ctrlclient.New(kubeConfig, ctrlclient.Options{})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	pcApps, err := installer.NewPCApps(
		kubeClient,
		c.Opts.Version,
		c.Opts.Namespace,
		c.Opts.ValuesFiles,
		c.Opts.ForceConflicts,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize pc-apps installer: %w", err)
	}

	if err := pcApps.Install(cmd.Context()); err != nil {
		return fmt.Errorf("failed to install pc-apps: %w", err)
	}

	return nil
}

func AddPCAppsCmd(parentCmd *cobra.Command, opts *GlobalOptions) {
	pcApps := InstallPCAppsCmd{
		Opts: InstallPCAppsOpts{
			GlobalOptions: opts,
		},
	}
	pcApps.cmd = &cobra.Command{
		Use:   "pc-apps",
		Short: "Install the pc-applications Helm chart from a private OCI registry",
		Long: packageio.Long(`Install or upgrade the pc-applications Helm chart from a private OCI
			registry into the target cluster. This chart deploys ArgoCD Application
			resources that manage the platform components.

			Registry credentials and chart URL are read automatically from the
			Kubernetes secret "argocd-codesphere-oci-read" in the argocd namespace.
			This secret is created by "oms beta install argocd --deploy-dc-config".`),
		Example: formatExamples("beta install pc-apps", []packageio.Example{
			{Cmd: "--version 1.0.0", Desc: "Install a specific version"},
			{Cmd: "--version 1.0.0 -f base.yaml -f dc-overlay.yaml", Desc: "Install with custom values files"},
			{Cmd: "--version 1.0.0 --namespace custom-ns", Desc: "Install into a custom namespace"},
			{Cmd: "--version 1.0.0 --force-conflicts", Desc: "Force SSA ownership conflicts during install or upgrade"},
		}),
	}
	pcApps.cmd.Flags().StringVar(&pcApps.Opts.Version, "version", "", "Chart version to install (required)")
	pcApps.cmd.Flags().StringVar(&pcApps.Opts.Namespace, "namespace", "argocd", "Target namespace for the Helm release")
	pcApps.cmd.Flags().StringArrayVarP(&pcApps.Opts.ValuesFiles, "values", "f", nil, "Path to values YAML file (can be specified multiple times, merged in order)")
	pcApps.cmd.Flags().BoolVar(&pcApps.Opts.ForceConflicts, "force-conflicts", false, "Force field ownership conflicts during install or upgrade (sets server-side apply ForceConflicts)")
	pcApps.cmd.RunE = pcApps.RunE

	util.MarkFlagRequired(pcApps.cmd, "version")

	AddCmd(parentCmd, pcApps.cmd)
}
