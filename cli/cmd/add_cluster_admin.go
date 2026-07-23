// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	packageio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/clusteradmin"
	intutil "github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

type AddClusterAdminCmd struct {
	cmd  *cobra.Command
	Opts AddClusterAdminOpts
}

type AddClusterAdminOpts struct {
	*util.GlobalOptions
	clusteradmin.Opts
}

func (c *AddClusterAdminCmd) RunE(_ *cobra.Command, _ []string) error {
	clientset, _, err := intutil.NewClients()
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return clusteradmin.AddClusterAdmin(c.cmd.Context(), clientset, c.Opts.Opts)
}

func AddAddClusterAdminCmd(parent *cobra.Command, opts *util.GlobalOptions) {
	c := AddClusterAdminCmd{
		cmd: &cobra.Command{
			Use:   "add-cluster-admin",
			Short: "Set the cluster admin email in a Kubernetes secret",
			Long: packageio.Long(`Sets the cluster admin email in the target Kubernetes cluster by writing it to a
				Kubernetes secret. The email is stored under the 'email' key of the secret, which the platform
				deployment consumes via a secretKeyRef. The secret is created if it does not exist yet and
				updated otherwise, so running the command again overwrites the previous email.

				The target cluster is determined by the current kubeconfig context. Set the KUBECONFIG
				environment variable to target a different kubeconfig.`),
			Example: util.FormatExamples("add-cluster-admin", []packageio.Example{
				{Cmd: "--email niklas@codesphere.com", Desc: "Set the cluster admin email using the default secret and namespace"},
				{Cmd: "--email admin@codesphere.com --namespace kube-system --secret-name cluster-admin-email", Desc: "Set the cluster admin email in a custom namespace"},
			}),
		},
		Opts: AddClusterAdminOpts{GlobalOptions: opts},
	}
	c.cmd.RunE = c.RunE

	flags := c.cmd.Flags()
	flags.StringVar(&c.Opts.Email, "email", "", "Email address of the cluster admin (required)")
	flags.StringVar(&c.Opts.Namespace, "namespace", clusteradmin.DefaultNamespace, "Kubernetes namespace where the secret is stored")
	flags.StringVar(&c.Opts.SecretName, "secret-name", clusteradmin.DefaultSecretName, "Name of the Kubernetes secret holding the cluster admin email")

	util.MarkFlagRequired(c.cmd, "email")

	util.AddCmd(parent, c.cmd)
}
