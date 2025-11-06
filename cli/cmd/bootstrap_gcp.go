// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/env"
)

// BootstrapGcpCmd represents the baseimage command
type BootstrapGcpCmd struct {
	cmd           *cobra.Command
	Opts          *GlobalOptions
	Env           env.Env
	CodesphereEnv *bootstrap.CodesphereEnvironemnt
}

func (c *BootstrapGcpCmd) RunE(_ *cobra.Command, args []string) error {

	err := c.BootstrapGcp()
	if err != nil {
		return fmt.Errorf("failed to bootstrap: %w", err)
	}

	return nil
}

func AddBootstrapGcpCmd(root *cobra.Command, opts *GlobalOptions) {
	bootstrapGcpCmd := BootstrapGcpCmd{
		cmd: &cobra.Command{
			Use:   "bootstrap-gcp",
			Short: "Bootstrap GCP infrastructure for Codesphere",
			Long: io.Long(`Bootstraps GCP infrastructure required to run Codesphere clusters on GCP.
				This includes setting up projects, service accounts, and necessary IAM roles.
				Depending on your setup, additional configuration may be required after bootstrapping.
				Ensure you have the necessary permissions to create and manage GCP resources before proceeding.
				Not for production use.`),
		},
		Opts:          opts,
		Env:           env.NewEnv(),
		CodesphereEnv: &bootstrap.CodesphereEnvironemnt{},
	}

	flags := bootstrapGcpCmd.cmd.Flags()
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.ProjectName, "project-name", "", "Unique GCP Project Name (required)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.BillingAccount, "billing-account", "", "GCP Billing Account ID (required)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.BaseDomain, "base-domain", "", "Base domain for Codesphere (required)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.GithubAppClientID, "github-app-client-id", "", "Github App Client ID (required)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.GithubAppClientSecret, "github-app-client-secret", "", "Github App Client Secret (required)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.SecretsDir, "secrets-dir", "/etc/codesphere/secrets", "Directory for secrets (default: /etc/codesphere/secrets)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.FolderID, "folder-id", "", "GCP Folder ID (optional)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.SSHPublicKeyPath, "ssh-public-key-path", "~/.ssh/id_rsa.pub", "SSH Public Key Path (default: ~/.ssh/id_rsa.pub)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.SSHPrivateKeyPath, "ssh-private-key-path", "~/.ssh/id_rsa", "SSH Private Key Path (default: ~/.ssh/id_rsa)")
	flags.BoolVar(&bootstrapGcpCmd.CodesphereEnv.Preemptible, "preemptible", false, "Use preemptible VMs for Codesphere infrastructure (default: false)")
	flags.IntVar(&bootstrapGcpCmd.CodesphereEnv.DatacenterID, "datacenter-id", 1, "Datacenter ID (default: 1)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.CustomPgIP, "custom-pg-ip", "", "Custom PostgreSQL IP (optional)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.InstallConfig, "install-config", "config.yaml", "Path to install config file (optional)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.SecretsFile, "secrets-file", "prod.vault.yaml", "Path to secrets files (optional)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.Region, "region", "europe-west4", "GCP Region (default: europe-west4)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.Zone, "zone", "europe-west4-a", "GCP Zone (default: europe-west4-a)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.DNSProjectID, "dns-project-id", "", "GCP Project ID for Cloud DNS (optional)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.DNSZoneName, "dns-zone-name", "oms-testing", "Cloud DNS Zone Name (optional)")
	flags.StringVar(&bootstrapGcpCmd.CodesphereEnv.InstallCodesphereVersion, "install-codesphere-version", "", "Codesphere version to install (default: none)")
	flags.BoolVar(&bootstrapGcpCmd.CodesphereEnv.WriteConfig, "write-config", true, "Write generated install config to file (default: true)")

	cobra.MarkFlagRequired(flags, "project-name")
	cobra.MarkFlagRequired(flags, "billing-account")
	cobra.MarkFlagRequired(flags, "base-domain")
	cobra.MarkFlagRequired(flags, "ssh-key-path")

	bootstrapGcpCmd.cmd.RunE = bootstrapGcpCmd.RunE
	root.AddCommand(bootstrapGcpCmd.cmd)
}

func (c *BootstrapGcpCmd) BootstrapGcp() error {
	bootstrapper, err := bootstrap.NewGCPBootstrapper(c.Env, c.CodesphereEnv)
	if err != nil {
		return err
	}
	env, err := bootstrapper.Bootstrap()
	envBytes, err2 := json.MarshalIndent(env, "", "  ")
	envString := string(envBytes)
	if err2 != nil {
		envString = ""
	}
	if err != nil {
		if env.Jumpbox.ExternalIP != "" {
			log.Printf("To debug on the jumpbox host:\nssh-add $SSH_KEY_PATH; ssh -o StrictHostKeyChecking=no -o ForwardAgent=yes -o SendEnv=OMS_PORTAL_API_KEY root@%s", env.Jumpbox.ExternalIP)
		}
		return fmt.Errorf("failed to bootstrap GCP: %w, env: %s", err, envString)
	}
	log.Println("GCP infrastructure bootstrapped:")

	log.Println(envString)

	log.Printf("Start the Codesphere installation using OMS from the jumpbox host:\nssh-add $SSH_KEY_PATH; ssh -o StrictHostKeyChecking=no -o ForwardAgent=yes -o SendEnv=OMS_PORTAL_API_KEY root@%s", env.Jumpbox.ExternalIP)

	log.Printf("When the installation is done, run the k0s configuration script generated at the k0s-1 host %s /root/configure-k0s.sh.", env.ControlPlaneNodes[0].InternalIP)

	return err
}
