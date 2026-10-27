// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/github"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/portal"
	intutil "github.com/codesphere-cloud/oms/internal/util"
)

// BootstrapGcpCmd holds the bootstrap-gcp command parameters, which
// provisions the GCP infrastructure to install a Codesphere cluster on.
type BootstrapGcpCmd struct {
	cmd               *cobra.Command
	Opts              *util.GlobalOptions
	Env               env.Env
	CodesphereEnv     *gcp.CodesphereEnvironment
	InputRegistryType string
	SSHQuiet          bool
	StepTimer         bool
	StorageEngine     string

	// experiments backs the deprecated --experiments flag; its values
	// are folded into the internal bucket for backwards compatibility.
	experiments []string
}

// RunE is the starting point for the bootstrap command that executes the bootstrap logic.
func (c *BootstrapGcpCmd) RunE(_ *cobra.Command, args []string) error {
	err := c.BootstrapGcp()
	if err != nil {
		return fmt.Errorf("failed to bootstrap: %w", err)
	}

	return nil
}

// AddBootstrapGcpCmd registers GCP bootstrap modes and infrastructure management commands.
func AddBootstrapGcpCmd(parent *cobra.Command, opts *util.GlobalOptions) {
	command := newBootstrapGcpCmd(opts).cmd
	util.AddCmd(parent, command)
	AddBootstrapGcpSingleNodeCmd(command, opts)
	AddBootstrapGcpMultiNodeCmd(command, opts)
	AddBootstrapGcpPostconfigCmd(command, opts)
	AddBootstrapGcpCleanupCmd(command, opts)
	AddBootstrapGcpRestartVMsCmd(command, opts)
}

// newBootstrapGcpCmd exposes the multi-node workflow at the root command.
func newBootstrapGcpCmd(opts *util.GlobalOptions) *BootstrapGcpCmd {
	c := newBootstrapGcpMultiNodeCmd(opts)
	c.cmd.Use = "bootstrap-gcp"
	c.cmd.Short = "Bootstrap GCP infrastructure for Codesphere"
	c.cmd.Long += "\n\nUses the multi-node installer. Use single-node for one VM."

	return c
}

func (c *BootstrapGcpCmd) addSharedBootstrapFlags() {
	flags := c.cmd.Flags()
	flags.StringVar(&c.CodesphereEnv.InstallConfigTemplatePath, "install-config-template", "", "Initial config.yaml template, applied before --config files")
	flags.StringArrayVar(&c.CodesphereEnv.InstallConfigs, "config", nil, "Local config.yaml file; repeat to merge in order before VM settings")
	flags.StringVar(&c.CodesphereEnv.InstallVaultPath, "vault", "", "SOPS vault for config templating and initial install secrets")
	flags.StringVar(&c.CodesphereEnv.InstallVaultPrivKey, "priv-key", "", "Age private key for --vault (or use the age key environment variable)")
	flags.StringVar(&c.CodesphereEnv.MachineType, "machine-type", "", "GCP machine type override for all VMs (default: role-specific sizes; e2-standard-8 for a single VM)")

	patHelp := "GitHub Personal Access Token for pulling Codesphere images from ghcr.io (defaults to OMS_REGISTRY_PASSWORD). Required scope: read:packages."
	userHelp := "Registry username for pulling Codesphere images from ghcr.io (required)"

	if !c.CodesphereEnv.SingleVM {
		patHelp += " Team SSH key lookup with --github-team-org/--github-team-slug also requires read:org. Required for the local-container registry type when installing Codesphere."
		userHelp = "Registry username for ghcr.io. Required for the GitHub registry type, and for the local-container registry type when installing Codesphere."
	}

	flags.StringVar(&c.CodesphereEnv.GitHubPAT, "github-pat", "", patHelp)
	flags.StringVar(&c.CodesphereEnv.RegistryUser, "registry-user", "", userHelp)

	flags.BoolVar(&c.StepTimer, util.TimerFlag, false, util.ShowStepDurationHelp)
	flags.StringVar(&c.CodesphereEnv.ProjectName, "project-name", "", "Unique GCP Project Name (required)")
	flags.StringVar(&c.CodesphereEnv.ProjectTTL, "project-ttl", "2h", "Time to live for the GCP project. Cleanup workflows will remove it afterwards. (default: 2 hours)")
	flags.StringVar(&c.CodesphereEnv.BillingAccount, "billing-account", "", "GCP Billing Account ID (required)")
	flags.StringVar(&c.CodesphereEnv.BaseDomain, "base-domain", "", "Base domain for Codesphere (required)")
	flags.StringVar(&c.CodesphereEnv.FolderID, "folder-id", "", "GCP Folder ID (optional)")
	flags.StringVar(&c.CodesphereEnv.SSHPublicKeyPath, "ssh-public-key-path", "~/.ssh/id_rsa.pub", "SSH Public Key Path (default: ~/.ssh/id_rsa.pub)")
	flags.StringVar(&c.CodesphereEnv.SSHPrivateKeyPath, "ssh-private-key-path", "~/.ssh/id_rsa", "SSH Private Key Path (default: ~/.ssh/id_rsa)")
	flags.StringVar(&c.CodesphereEnv.DNSProjectID, "dns-project-id", "", "GCP Project ID for Cloud DNS (optional)")
	flags.StringVar(&c.CodesphereEnv.DNSZoneName, "dns-zone-name", "oms-testing", "Cloud DNS Zone Name (optional)")
	flags.BoolVar(&c.CodesphereEnv.SpotVMs, "spot-vms", false, "Use Spot VMs for Codesphere infrastructure. Falls back to standard VMs if spot capacity unavailable. (default: false)")
	flags.Int64Var(&c.CodesphereEnv.RootDiskSize, "root-disk-size", 50, "Instance root disk size in GB (default: 50)")
	flags.BoolVar(&c.SSHQuiet, "ssh-quiet", false, "Suppress SSH command output (default: false)")
	flags.StringVar(&c.CodesphereEnv.Region, "region", "europe-west4", "GCP Region (default: europe-west4)")
	flags.StringVar(&c.CodesphereEnv.Zone, "zone", "europe-west4-a", "GCP Zone (default: europe-west4-a)")
	flags.StringVar(&c.CodesphereEnv.InstallLocal, "install-local", "", "Install Codesphere from local package (default: none)")
	flags.StringVar(&c.CodesphereEnv.InstallVersion, "install-version", "", "Codesphere version to install (default: none)")
	flags.StringVar(&c.CodesphereEnv.InstallHash, "install-hash", "", "Codesphere package hash to install (default: none)")
	flags.StringVar(&c.CodesphereEnv.RemoteOmsBinaryPath, "remote-oms-binary", "", "Path to a local Linux amd64 OMS binary to copy to and use on the remote host instead of downloading a release (optional)")

	util.MarkFlagRequired(c.cmd, "project-name")
	util.MarkFlagRequired(c.cmd, "billing-account")
	util.MarkFlagRequired(c.cmd, "base-domain")
}

// prepareBootstrapOptions resolves credentials and validates shared configuration flags.
func (c *BootstrapGcpCmd) prepareBootstrapOptions() error {
	if c.CodesphereEnv.GitHubPAT == "" {
		c.CodesphereEnv.GitHubPAT = os.Getenv("OMS_REGISTRY_PASSWORD")
	}

	if c.CodesphereEnv.InstallVaultPrivKey != "" && c.CodesphereEnv.InstallVaultPath == "" {
		return fmt.Errorf("--priv-key requires --vault")
	}

	return nil
}

// BootstrapGcp initializes external clients and starts the selected bootstrap workflow.
func (c *BootstrapGcpCmd) BootstrapGcp() error {
	ctx := c.cmd.Context()
	stlog := bootstrap.NewStepLogger(false, bootstrap.WithTimer(c.StepTimer))
	defer stlog.PrintSummary()

	icg, err := installer.NewInstallConfigManager("plain", "")
	if err != nil {
		return fmt.Errorf("failed to initialize conig manager: %w", err)
	}

	gcpClient := gcp.NewGCPClient(ctx, stlog, os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	fw := intutil.NewFilesystemWriter()
	portalClient := portal.NewPortalClient()

	githubClient, err := github.NewGitHubClient(ctx, c.CodesphereEnv.GitHubPAT)
	if err != nil {
		return fmt.Errorf("failed to create github client: %w", err)
	}

	sshClient := node.NewSSHNodeClient(c.SSHQuiet)
	if c.CodesphereEnv.SingleVM {
		sshClient.RegistryPassword = c.CodesphereEnv.GitHubPAT
	}

	bs, err := gcp.NewGCPBootstrapper(
		ctx,
		c.Env,
		stlog,
		c.CodesphereEnv,
		icg,
		gcpClient,
		fw,
		sshClient,
		portalClient,
		intutil.NewTime(),
		githubClient,
	)
	if err != nil {
		return fmt.Errorf("failed to create gcp bootstrapper: %w", err)
	}

	c.CodesphereEnv.OmsWorkdir = c.Env.GetOmsWorkdir()
	if c.CodesphereEnv.SingleVM {
		return c.bootstrapSingleVM(bs)
	}

	c.CodesphereEnv.RegistryType = gcp.ResolveRegistryType(c.InputRegistryType, c.cmd.Flags().Changed("registry-type"), c.CodesphereEnv.Airgapped)

	// The value alone cannot distinguish the default 1 from an explicit --datacenter-id=1.
	c.CodesphereEnv.DatacenterIDExplicit = c.cmd.Flags().Changed("datacenter-id")

	if c.cmd.Flags().Changed("experiments") {
		if c.cmd.Flags().Changed("internal-flags") {
			log.Printf("Warning: both --experiments and --internal-flags were set; ignoring deprecated --experiments values %v", c.experiments)
		} else {
			c.CodesphereEnv.InternalFlags = c.experiments
		}
	}

	err = bs.Bootstrap()

	writeInfraErr := bs.WriteInfraFile()
	if writeInfraErr != nil {
		return fmt.Errorf("failed to write infra details: %w", writeInfraErr)
	}

	if err != nil {
		if bs.Env.Jumpbox != nil && bs.Env.Jumpbox.GetExternalIP() != "" {
			log.Printf("To debug on the jumpbox host:\nssh-add $SSH_KEY_PATH; ssh -o StrictHostKeyChecking=no -o ForwardAgent=yes -o SendEnv=OMS_PORTAL_API_KEY root@%s", bs.Env.Jumpbox.GetExternalIP())
		}

		return fmt.Errorf("failed to bootstrap GCP: %w", err)
	}

	log.Println("\n🎉🎉🎉 GCP infrastructure bootstrapped successfully!")
	log.Printf("Access the jumpbox using:\nssh-add $SSH_KEY_PATH; ssh -o StrictHostKeyChecking=no -o ForwardAgent=yes -o SendEnv=OMS_PORTAL_API_KEY -o SendEnv=OMS_PORTAL_API root@%s", bs.Env.Jumpbox.GetExternalIP())

	if bs.Env.InstallVersion != "" {
		log.Printf("Access Codesphere in your web browser at https://cs.%s", bs.Env.BaseDomain)

		for _, dc := range bs.Env.DataCenters {
			log.Printf("Data center %d hosts workspaces under %s", dc.ID, dc.WorkspaceHostingBaseDomain)
		}

		return nil
	}

	packageFile := "<package-name>-" + gcp.InstallerArchiveName
	if gcp.RegistryType(bs.Env.RegistryType) == gcp.RegistryTypeGitHub {
		log.Printf("Images are pulled directly from GHCR, so container images are not loaded from the package.")
	}

	if gcp.RegistryType(bs.Env.RegistryType) == gcp.RegistryTypeLocalContainer {
		log.Printf("The local registry is empty until the package artifacts are mirrored into it (run from jumpbox):\noms copy package -p <package-name>-%s --dest %s --yes",
			gcp.InstallerArchiveName, bs.Env.ContainerRegistryURL)
	}

	// The commands the bootstrapper would have run itself, so that an operator running them by hand
	// gets the same ones instead of copies that drift from them.
	if len(bs.Env.DataCenters) > 1 {
		log.Printf("example install commands (run from jumpbox). Run the data center 1 command to completion first — the other data centers share its database:")
	} else {
		log.Printf("example install command (run from jumpbox):")
	}

	for _, dc := range bs.Env.DataCenters {
		log.Printf("# data center %d\n%s", dc.ID, bs.InstallCommand(dc, packageFile))
	}

	return nil
}
