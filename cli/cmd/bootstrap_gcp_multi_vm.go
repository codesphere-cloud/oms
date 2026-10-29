// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/spf13/cobra"
)

// AddBootstrapGcpMultiNodeCmd registers the multi-node bootstrap subcommand.
func AddBootstrapGcpMultiNodeCmd(parent *cobra.Command, opts *util.GlobalOptions) {
	util.AddCmd(parent, newBootstrapGcpMultiNodeCmd(opts).cmd)
}

func newBootstrapGcpMultiNodeCmd(opts *util.GlobalOptions) *BootstrapGcpCmd {
	c := &BootstrapGcpCmd{
		cmd: &cobra.Command{
			Use:   "multi-node",
			Short: "Bootstrap a multi-node Codesphere cluster on GCP",
			Long: io.Long(`Bootstrap GCP projects, service accounts, and VMs for a Codesphere cluster.
				Running again with --install-version repeats every install step. k0s is reinstalled,
				which restarts it, only in data centers where a node does not run --k0s-version with
				its config script applied yet. To replace only Codesphere, add
				--install-codesphere-only and --recover-config.
				Not for production use.`),
			Args: cobra.NoArgs,
		},
		Opts:          opts,
		Env:           env.NewEnv(),
		CodesphereEnv: &gcp.CodesphereEnvironment{},
	}
	c.cmd.RunE = func(command *cobra.Command, args []string) error {
		if err := c.prepareBootstrapOptions(); err != nil {
			return err
		}

		return c.RunE(command, args)
	}
	c.addSharedBootstrapFlags()
	c.addMultiNodeBootstrapFlags()

	return c
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
	flags.BoolVar(&c.CodesphereEnv.InstallCodesphereOnly, "install-codesphere-only", false, "Re-install only Codesphere into an already bootstrapped project, e.g. to roll back to an earlier release. Passes --codesphere-only to the \"oms install codesphere\" the bootstrap runs on the jumpbox, and skips installing k0s and its config script. Requires --install-version or --install-local, and --recover-config (default: false)")
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
