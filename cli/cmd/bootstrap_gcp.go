// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/bootstrap/local"
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
	legacyMode        bool

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
	command := newLegacyBootstrapGcpCmd(opts).cmd
	util.AddCmd(parent, command)
	util.AddCmd(command, newBootstrapGcpSingleNodeCmd(opts).cmd)
	util.AddCmd(command, newBootstrapGcpMultiNodeCmd(opts).cmd)
	AddBootstrapGcpPostconfigCmd(command, opts)
	AddBootstrapGcpCleanupCmd(command, opts)
	AddBootstrapGcpRestartVMsCmd(command, opts)
}

// newLegacyBootstrapGcpCmd preserves the original flag-based entry point.
func newLegacyBootstrapGcpCmd(opts *util.GlobalOptions) *BootstrapGcpCmd {
	c := newBootstrapGcpMultiNodeCmd(opts)
	c.legacyMode = true
	c.cmd.Use = "bootstrap-gcp"
	c.cmd.Short = "Bootstrap GCP infrastructure for Codesphere"
	c.cmd.Long += "\n\nDefaults to the multi-node installer. Use single-node for one VM. The legacy --single-vm flag is deprecated."
	c.cmd.Flags().BoolVar(&c.CodesphereEnv.SingleVM, "single-vm", false, "Install k0s and Codesphere on one GCP VM (also available as the single-node subcommand)")
	_ = c.cmd.Flags().MarkDeprecated("single-vm", "use bootstrap-gcp single-node instead")
	c.cmd.Flags().StringVar(&c.StorageEngine, "storage-engine", local.StorageEngineRookCeph, "Single VM: storage engine (rook-ceph or local)")

	return c
}

func newBootstrapGcpSingleNodeCmd(opts *util.GlobalOptions) *BootstrapGcpCmd {
	c := &BootstrapGcpCmd{
		cmd: &cobra.Command{
			Use:   "single-node",
			Short: "Bootstrap k0s and Codesphere on one GCP VM",
			Long: io.Long(`Install k0s and Codesphere on one GCP VM using the local bootstrap.
				Requires --registry-user, either --github-pat or OMS_REGISTRY_PASSWORD,
				and either --install-local or --install-version. The root disk defaults to 50 GB;
				a separate 100 GB Ceph disk is added. Supply installation settings through --config and --vault.
				Not for production use.`),
			Args: cobra.NoArgs,
		},
		Opts:          opts,
		Env:           env.NewEnv(),
		CodesphereEnv: &gcp.CodesphereEnvironment{SingleVM: true},
	}
	c.cmd.RunE = c.RunE
	c.addSharedBootstrapFlags()
	c.cmd.Flags().Lookup("spot-vms").Usage = "Use Spot VMs for Codesphere infrastructure. Falls back to standard VMs if spot capacity unavailable. (default: false)"
	c.cmd.Flags().Lookup("github-pat").Usage = "GitHub Personal Access Token for pulling Codesphere images from ghcr.io (defaults to OMS_REGISTRY_PASSWORD). Required scope: read:packages."
	c.cmd.Flags().Lookup("registry-user").Usage = "Registry username for pulling Codesphere images from ghcr.io (required)"
	c.cmd.Flags().StringVar(&c.StorageEngine, "storage-engine", local.StorageEngineRookCeph, "Storage engine (rook-ceph or local)")

	return c
}

func newBootstrapGcpMultiNodeCmd(opts *util.GlobalOptions) *BootstrapGcpCmd {
	c := &BootstrapGcpCmd{
		cmd: &cobra.Command{
			Use:   "multi-node",
			Short: "Bootstrap a multi-node Codesphere cluster on GCP",
			Long: io.Long(`Bootstrap GCP projects, service accounts, and VMs for a Codesphere cluster.
				Running again with --install-version repeats every install step, including the k0s
				config script, which restarts k0s. To replace only Codesphere, add
				--install-codesphere-only and --recover-config.
				Not for production use.`),
			Args: cobra.NoArgs,
		},
		Opts:          opts,
		Env:           env.NewEnv(),
		CodesphereEnv: &gcp.CodesphereEnvironment{},
	}
	c.cmd.RunE = c.RunE
	c.addSharedBootstrapFlags()
	c.addMultiNodeBootstrapFlags()

	return c
}

func (c *BootstrapGcpCmd) addSharedBootstrapFlags() {
	c.addBootstrapConfigFlags()
	flags := c.cmd.Flags()
	flags.BoolVar(&c.StepTimer, util.TimerFlag, false, util.ShowStepDurationHelp)
	flags.StringVar(&c.CodesphereEnv.ProjectName, "project-name", "", "Unique GCP Project Name (required)")
	flags.StringVar(&c.CodesphereEnv.ProjectTTL, "project-ttl", "2h", "Time to live for the GCP project. Cleanup workflows will remove it afterwards. (default: 2 hours)")
	flags.StringVar(&c.CodesphereEnv.BillingAccount, "billing-account", "", "GCP Billing Account ID (required)")
	flags.StringVar(&c.CodesphereEnv.BaseDomain, "base-domain", "", "Base domain for Codesphere (required)")
	flags.StringVar(&c.CodesphereEnv.FolderID, "folder-id", "", "GCP Folder ID (optional)")
	flags.StringVar(&c.CodesphereEnv.SSHPublicKeyPath, "ssh-public-key-path", "~/.ssh/id_rsa.pub", "SSH Public Key Path (default: ~/.ssh/id_rsa.pub)")
	flags.StringVar(&c.CodesphereEnv.SSHPrivateKeyPath, "ssh-private-key-path", "~/.ssh/id_rsa", "SSH Private Key Path (default: ~/.ssh/id_rsa)")
	flags.StringVar(&c.CodesphereEnv.GitHubPAT, "github-pat", "", "GitHub Personal Access Token used for direct image access and fetching team SSH keys (defaults to OMS_REGISTRY_PASSWORD). Required when using --github-team-org/--github-team-slug, and for the local-container registry type when installing Codesphere. Required scopes: read:packages, read:org.")
	flags.StringVar(&c.CodesphereEnv.DNSProjectID, "dns-project-id", "", "GCP Project ID for Cloud DNS (optional)")
	flags.StringVar(&c.CodesphereEnv.DNSZoneName, "dns-zone-name", "oms-testing", "Cloud DNS Zone Name (optional)")
	flags.BoolVar(&c.CodesphereEnv.SpotVMs, "spot-vms", false, "Use Spot VMs for Codesphere infrastructure. Falls back to standard VMs if spot capacity unavailable. Mutually exclusive with --preemptible (default: false)")
	flags.Int64Var(&c.CodesphereEnv.RootDiskSize, "root-disk-size", 50, "Instance root disk size in GB (default: 50)")
	flags.BoolVar(&c.SSHQuiet, "ssh-quiet", false, "Suppress SSH command output (default: false)")
	flags.StringVar(&c.CodesphereEnv.Region, "region", "europe-west4", "GCP Region (default: europe-west4)")
	flags.StringVar(&c.CodesphereEnv.Zone, "zone", "europe-west4-a", "GCP Zone (default: europe-west4-a)")
	flags.StringVar(&c.CodesphereEnv.InstallLocal, "install-local", "", "Install Codesphere from local package (default: none)")
	flags.StringVar(&c.CodesphereEnv.InstallVersion, "install-version", "", "Codesphere version to install (default: none)")
	flags.StringVar(&c.CodesphereEnv.InstallHash, "install-hash", "", "Codesphere package hash to install (default: none)")
	flags.StringVar(&c.CodesphereEnv.RemoteOmsBinaryPath, "remote-oms-binary", "", "Path to a local Linux amd64 OMS binary to copy to and use on the jumpbox instead of downloading a release (optional)")
	flags.StringVar(&c.CodesphereEnv.RegistryUser, "registry-user", "", "Registry username for ghcr.io. Required for the GitHub registry type, and for the local-container registry type when installing Codesphere, where it is used with --github-pat to mirror the Codesphere images into the local container registry (optional)")

	util.MarkFlagRequired(c.cmd, "project-name")
	util.MarkFlagRequired(c.cmd, "billing-account")
	util.MarkFlagRequired(c.cmd, "base-domain")
}

func (c *BootstrapGcpCmd) addMultiNodeBootstrapFlags() {
	flags := c.cmd.Flags()
	flags.StringVar(&c.CodesphereEnv.SecretsDir, "secrets-dir", "/etc/codesphere/secrets", "Directory for secrets (default: /etc/codesphere/secrets)")
	flags.StringVar(&c.CodesphereEnv.BitbucketAppClientID, "bitbucket-app-client-id", "", "Bitbucket App Client ID (optional)")
	flags.StringVar(&c.CodesphereEnv.BitbucketAppClientSecret, "bitbucket-app-client-secret", "", "Bitbucket App Client Secret (optional)")
	flags.StringVar(&c.CodesphereEnv.AzureDevOpsAppClientID, "azure-devops-app-client-id", "", "Azure DevOps App Client ID (optional)")
	flags.StringVar(&c.CodesphereEnv.AzureDevOpsAppClientSecret, "azure-devops-app-client-secret", "", "Azure DevOps App Client Secret (optional)")
	flags.StringVar(&c.CodesphereEnv.OidcProviderName, "oidc-provider-name", "", "OIDC OAuth provider display name (optional)")
	flags.StringVar(&c.CodesphereEnv.OidcIssuerURL, "oidc-issuer-url", "", "OIDC OAuth provider issuer URL (optional)")
	flags.StringVar(&c.CodesphereEnv.OidcClientID, "oidc-client-id", "", "OIDC OAuth provider Client ID (optional)")
	flags.StringVar(&c.CodesphereEnv.OidcClientSecret, "oidc-client-secret", "", "OIDC OAuth provider Client Secret (optional)")
	flags.StringVar(&c.CodesphereEnv.GitHubAppClientID, "github-app-client-id", "", "GitHub App Client ID (required)")
	flags.StringVar(&c.CodesphereEnv.GitHubAppClientSecret, "github-app-client-secret", "", "GitHub App Client Secret (required)")
	flags.StringVar(&c.CodesphereEnv.GitLabAppClientID, "gitlab-app-client-id", "", "GitLab App Client ID (optional)")
	flags.StringVar(&c.CodesphereEnv.GitLabAppClientSecret, "gitlab-app-client-secret", "", "GitLab App Client Secret (optional)")
	flags.StringVar(&c.CodesphereEnv.GitHubAppName, "github-app-name", "", "GitHub App Name (optional)")
	flags.StringVar(&c.CodesphereEnv.GitHubTeamOrg, "github-team-org", "", "GitHub organization used to fetch team SSH keys (optional, used with --github-team-slug). Requires --github-pat with at least the read:org scope.")
	flags.StringVar(&c.CodesphereEnv.GitHubTeamSlug, "github-team-slug", "", "GitHub team slug used to fetch team SSH keys (optional, used with --github-team-org). Requires --github-pat with at least the read:org scope.")
	flags.BoolVar(&c.CodesphereEnv.Preemptible, "preemptible", false, "Use preemptible VMs for Codesphere infrastructure. Mutually exclusive with --spot-vms (default: false)")
	flags.BoolVar(&c.CodesphereEnv.WriteConfig, "write-config", true, "Write generated install config to file (default: true)")
	flags.BoolVar(&c.CodesphereEnv.RecoverConfig, "recover-config", false, "Recover previously generated install config from the jumpbox. This will overwrite the local config! (default: false)")
	flags.BoolVar(&c.CodesphereEnv.CreateTestUser, "create-test-user", false, "Create a test user with API token on the bootstrapped instance for smoke testing (default: false)")
	flags.StringVar(&c.CodesphereEnv.InstallConfigPath, "install-config", "config.yaml", "Path to install config file (optional)")
	flags.StringVar(&c.CodesphereEnv.SecretsFilePath, "secrets-file", "prod.vault.yaml", "Path to secrets files (optional)")
	flags.IntVar(&c.CodesphereEnv.DatacenterID, "datacenter-id", 1, "Datacenter ID (default: 1)")
	flags.StringVar(&c.CodesphereEnv.DatacenterName, "datacenter-name", "dev", "Datacenter name (default: dev)")
	flags.BoolVar(&c.CodesphereEnv.MultiDC, "multi-dc", false, "Bootstrap two data centers that share one PostgreSQL server but run separate Kubernetes and Ceph clusters. Doubles the Ceph and k0s nodes to 14 VMs (~100 vCPUs) and reserves 6 static IPs, so the region's CPU quota may need raising. Cannot be combined with --datacenter-id. (default: false)")
	flags.StringVar(&c.CodesphereEnv.CustomPgIP, "custom-pg-ip", "", "Custom PostgreSQL IP (optional)")
	flags.StringArrayVarP(&c.CodesphereEnv.InstallSkipSteps, "install-skip-steps", "s", []string{}, "Installation steps to skip during Codesphere installation (optional)")
	flags.BoolVar(&c.CodesphereEnv.InstallCodesphereOnly, "install-codesphere-only", false, "Re-install only Codesphere into an already bootstrapped project, e.g. to roll back to an earlier release. Passes --codesphere-only to the \"oms install codesphere\" the bootstrap runs on the jumpbox, and skips installing k0s and its config script, which would restart k0s. Requires --install-version or --install-local, and --recover-config (default: false)")
	flags.StringArrayVar(&c.CodesphereEnv.PCAppsValues, "pc-apps-values", nil, "pc-apps values YAML file passed unchanged to the Codesphere installation (can be specified multiple times, optional)")
	flags.StringVar(&c.InputRegistryType, "registry-type", "github", "Container registry type to use (options: local-container, artifact-registry, github) (default: github)")
	flags.BoolVar(&c.CodesphereEnv.Airgapped, "airgapped", false, "Set up an airgapped installation: run a local container registry on the jumpbox, copy the package images into it and point the install config at it. k0s is installed from the k0s airgap image bundle. Firewall rules block internet egress of all VMs except the jumpbox. Selects the local-container registry type and fails if another --registry-type is given. (default: false)")
	flags.StringArrayVar(&c.CodesphereEnv.InternalFlags, "internal-flags", gcp.DefaultInternalFlags, "Internal flags to enable in Codesphere installation (optional)")
	flags.StringArrayVar(&c.experiments, "experiments", []string{}, "Deprecated: use --internal-flags instead. Values are added to the internal flags.")
	_ = flags.MarkDeprecated("experiments", "use --internal-flags instead")
	flags.StringArrayVar(&c.CodesphereEnv.PreviewFlags, "preview-flags", gcp.DefaultPreviewFlags, "Preview flags to enable in Codesphere installation (optional)")
	flags.StringArrayVar(&c.CodesphereEnv.FeatureFlags, "feature-flags", gcp.DefaultFeatureFlags, "Feature flags to enable in Codesphere installation (optional)")
	flags.StringVar(&c.CodesphereEnv.ExternalLokiEndpoint, "external-loki-endpoint", "", "External Loki endpoint for Grafana Alloy log forwarding (optional)")
	flags.StringVar(&c.CodesphereEnv.ExternalLokiSecret, "external-loki-secret", "", "External Loki password stored in the generated vault (optional)")
	flags.StringVar(&c.CodesphereEnv.ExternalLokiUser, "external-loki-user", "", "External Loki username for Grafana Alloy log forwarding (optional)")
	flags.StringVar(&c.CodesphereEnv.PrometheusRemoteWriteURL, "prometheus-remote-write-url", "", "Prometheus remote write URL (optional)")
	flags.StringVar(&c.CodesphereEnv.PrometheusRemoteWriteUser, "prometheus-remote-write-user", "", "Prometheus remote write username (optional)")
	flags.StringVar(&c.CodesphereEnv.PrometheusRemoteWritePassword, "prometheus-remote-write-password", "", "Prometheus remote write password stored in the generated vault (optional)")
	flags.StringVar(&c.CodesphereEnv.ClusterAdminEmail, "cluster-admin-email", "", "Email address of the initial cluster admin. Written to the install config and applied as the cluster-admin-email secret before the Codesphere platform is installed (optional)")
	flags.BoolVar(&c.CodesphereEnv.GoogleACMEIssuer, "google-acme-issuer", false, "Use Google Public CA as the ACME issuer instead of Let's Encrypt. External Account Binding credentials are obtained automatically via the publicca API (default: false)")
	flags.BoolVar(&c.CodesphereEnv.ACMEStaging, "acme-staging", false, "Use the Let's Encrypt staging ACME endpoint (certificates are not browser-trusted)")
	flags.StringVar(&c.CodesphereEnv.OpenBaoURI, "openbao-uri", "", "URI for OpenBao (optional)")
	flags.StringVar(&c.CodesphereEnv.OpenBaoEngine, "openbao-engine", "cs-secrets-engine", "OpenBao engine name (default: cs-secrets-engine)")
	flags.StringVar(&c.CodesphereEnv.OpenBaoUser, "openbao-user", "admin", "OpenBao username (optional)")
	flags.StringVar(&c.CodesphereEnv.OpenBaoPassword, "openbao-password", "", "OpenBao password (optional)")
	flags.StringVar(&c.CodesphereEnv.CentralOtelEndpoint, "central-otel-endpoint", "", "Central OpenTelemetry collector endpoint. Needed when sending spans to central collector (optional)")
	flags.StringVar(&c.CodesphereEnv.CentralOtelUsername, "central-otel-username", "", "Central OpenTelemetry username. Needed when sending spans to central collector (optional)")
	flags.StringVar(&c.CodesphereEnv.CentralOtelPassword, "central-otel-password", "", "Central OpenTelemetry password. Needed when sending spans to central collector (optional)")
	flags.StringVar(&c.CodesphereEnv.LocalTraceEndpoint, "local-trace-endpoint", "", "Endpoint for exporting traces to an in-cluster storage (optional)")
	flags.BoolVar(&c.CodesphereEnv.CentralOtelSpanMetrics, "central-otel-span-metrics", false, "Enable span metrics in Central OpenTelemetry export (default: false)")
	flags.StringVar(&c.CodesphereEnv.K0sVersion, "k0s-version", installer.DefaultK0sVersion, "K0s version to use for bootstrapping (optional)")
}

// BootstrapGcp initializes the bootstrapper, validates the input flags and starts bootstrapping.
// Returns an error if validation or github client connection fails
func (c *BootstrapGcpCmd) BootstrapGcp() error {
	if err := c.prepareBootstrapOptions(); err != nil {
		return err
	}

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
