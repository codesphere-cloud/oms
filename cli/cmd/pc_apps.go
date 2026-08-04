// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

// InstallPCAppsCmd represents the pc-apps command
type InstallPCAppsCmd struct {
	cmd  *cobra.Command
	Opts InstallPCAppsOpts
}

type InstallPCAppsOpts struct {
	*util.GlobalOptions
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

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add kubernetes core scheme: %w", err)
	}
	if err := argov1alpha1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add ArgoCD scheme: %w", err)
	}

	kubeClient, err := ctrlclient.New(kubeConfig, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	pcApps, err := installer.NewPCApps(
		kubeClient,
		c.Opts.Version,
		c.Opts.Namespace,
		c.Opts.ValuesFiles,
		nil,
		c.Opts.ForceConflicts,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize pc-apps installer: %w", err)
	}

	if err := pcApps.Install(cmd.Context()); err != nil {
		return fmt.Errorf("failed to register pc-apps app-of-apps: %w", err)
	}

	return nil
}

func AddPCAppsCmd(parentCmd *cobra.Command, opts *util.GlobalOptions) {
	pcApps := InstallPCAppsCmd{
		Opts: InstallPCAppsOpts{
			GlobalOptions: opts,
		},
	}
	pcApps.cmd = &cobra.Command{
		Use:   "pc-apps",
		Short: "Register the pc-applications app-of-apps in ArgoCD",
		Long: packageio.Long(`Create or update the "pc-applications" ArgoCD Application (app of apps)
			that references the pc-applications Helm chart in a private OCI registry.
			ArgoCD pulls and syncs the chart itself, which in turn deploys the
			ArgoCD Application resources managing the platform components.

			The chart registry URL is read from the Kubernetes secret
			"argocd-codesphere-oci-read" in the argocd namespace, which also provides
			ArgoCD with the credentials to pull the chart. This secret is created by
			"oms beta install argocd --deploy-dc-config".`),
		Example: util.FormatExamples("beta install pc-apps", []packageio.Example{
			{Cmd: "--version 1.0.0", Desc: "Register a specific chart version"},
			{Cmd: "--version 1.0.0 -f base.yaml -f dc-overlay.yaml", Desc: "Register with custom values files"},
			{Cmd: "--version 1.0.0 --namespace custom-ns", Desc: "Deploy the apps into a custom namespace"},
			{Cmd: "--version 1.0.0 --force-conflicts", Desc: "Force SSA ownership conflicts when applying the Application"},
		}),
	}
	pcApps.cmd.Flags().StringVar(&pcApps.Opts.Version, "version", "", "Chart version to reference as the Application target revision (required)")
	pcApps.cmd.Flags().StringVar(&pcApps.Opts.Namespace, "namespace", "argocd", "Destination namespace the app-of-apps deploys into")
	pcApps.cmd.Flags().StringArrayVarP(&pcApps.Opts.ValuesFiles, "values", "f", nil, "Path to values YAML file (can be specified multiple times, merged in order)")
	pcApps.cmd.Flags().BoolVar(&pcApps.Opts.ForceConflicts, "force-conflicts", false, "Force field ownership conflicts when applying the Application (sets server-side apply ForceConflicts)")
	pcApps.cmd.RunE = pcApps.RunE

	util.MarkFlagRequired(pcApps.cmd, "version")

	util.AddCmd(parentCmd, pcApps.cmd)
}
