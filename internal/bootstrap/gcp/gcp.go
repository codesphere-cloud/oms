// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/github"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/testuser"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/lithammer/shortuuid"
	"go.yaml.in/yaml/v3"
	"google.golang.org/api/dns/v1"
)

type RegistryType string

const (
	RegistryTypeLocalContainer   RegistryType = "local-container"
	RegistryTypeArtifactRegistry RegistryType = "artifact-registry"
	RegistryTypeGitHub           RegistryType = "github"
)

// CheckOMSManagedLabel checks if the given labels map indicates an OMS-managed project.
// A project is considered OMS-managed if it has the 'oms-managed' label set to "true".
func CheckOMSManagedLabel(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	value, exists := labels[OMSManagedLabel]
	return exists && value == "true"
}

// GetDNSRecordNames returns the DNS record names that OMS creates for a given base domain.
func GetDNSRecordNames(baseDomain string) []struct {
	Name  string
	Rtype string
} {
	return []struct {
		Name  string
		Rtype string
	}{
		{fmt.Sprintf("cs.%s.", baseDomain), "A"},
		{fmt.Sprintf("*.cs.%s.", baseDomain), "A"},
		{fmt.Sprintf("ws.%s.", baseDomain), "A"},
		{fmt.Sprintf("*.ws.%s.", baseDomain), "A"},
	}
}

var DefaultExperiments []string = []string{
	"headless-services",
	"vcluster",
	"custom-service-image",
	"ms-in-ls",
	"secret-management",
	"sub-path-mount",
}

type GCPBootstrapper struct {
	ctx       context.Context
	stlog     *bootstrap.StepLogger
	fw        util.FileIO
	icg       installer.InstallConfigManager
	Time      util.Time
	GCPClient GCPClientManager
	// Environment
	Env *CodesphereEnvironment
	// SSH command runner
	NodeClient       node.NodeClient
	PortalClient     portal.Portal
	GitHubClient     github.GitHubClient
	OmsBinaryBuilder func() (string, func(), error)
}

type CodesphereEnvironment struct {
	ProjectID                     string          `json:"project_id"`
	ProjectTTL                    string          `json:"project_ttl"`
	ProjectName                   string          `json:"project_name"`
	DNSProjectID                  string          `json:"dns_project_id"`
	Jumpbox                       *node.Node      `json:"jumpbox"`
	PostgreSQLNode                *node.Node      `json:"postgres_node"`
	ControlPlaneNodes             []*node.Node    `json:"control_plane_nodes"`
	CephNodes                     []*node.Node    `json:"ceph_nodes"`
	ContainerRegistryURL          string          `json:"-"`
	ExistingConfigUsed            bool            `json:"-"`
	InstallVersion                string          `json:"install_version"`
	InstallLocal                  string          `json:"install_local"`
	InstallHash                   string          `json:"install_hash"`
	InstallSkipSteps              []string        `json:"install_skip_steps"`
	Preemptible                   bool            `json:"preemptible"`
	SpotVMs                       bool            `json:"spot_vms"`
	WriteConfig                   bool            `json:"-"`
	RecoverConfig                 bool            `json:"-"`
	GatewayIP                     string          `json:"gateway_ip"`
	PublicGatewayIP               string          `json:"public_gateway_ip"`
	RegistryType                  RegistryType    `json:"registry_type"`
	GitHubPAT                     string          `json:"-"`
	GitHubAppName                 string          `json:"-"`
	GitHubTeamOrg                 string          `json:"github_team_org"`
	GitHubTeamSlug                string          `json:"github_team_slug"`
	RegistryUser                  string          `json:"-"`
	Experiments                   []string        `json:"experiments"`
	FeatureFlags                  map[string]bool `json:"feature_flags"`
	ExternalLokiEndpoint          string          `json:"external_loki_endpoint,omitempty"`
	ExternalLokiSecret            string          `json:"-"`
	ExternalLokiUser              string          `json:"external_loki_user,omitempty"`
	PrometheusRemoteWriteUser     string          `json:"prometheus_remote_write_user,omitempty"`
	PrometheusRemoteWritePassword string          `json:"-"`
	PrometheusRemoteWriteURL      string          `json:"prometheus_remote_write_url,omitempty"`

	// ACME Issuer
	GoogleACMEIssuer bool `json:"google_acme_issuer,omitempty"`

	// OpenBao
	OpenBaoURI      string `json:"-"`
	OpenBaoEngine   string `json:"-"`
	OpenBaoUser     string `json:"-"`
	OpenBaoPassword string `json:"-"`

	CentralOtelEndpoint    string `json:"-"`
	CentralOtelUsername    string `json:"-"`
	CentralOtelPassword    string `json:"-"`
	CentralOtelSpanMetrics bool   `json:"-"`
	LocalTraceEndpoint     string `json:"-"`

	// Config
	InstallConfigPath string              `json:"-"`
	SecretsFilePath   string              `json:"-"`
	InstallConfig     *files.RootConfig   `json:"-"`
	Secrets           *files.InstallVault `json:"-"`

	// GCP Specific
	ProjectDisplayName         string `json:"project_display_name"`
	BillingAccount             string `json:"billing_account"`
	BaseDomain                 string `json:"base_domain"`
	GitHubAppClientID          string `json:"-"`
	GitHubAppClientSecret      string `json:"-"`
	GitLabAppClientID          string `json:"-"`
	GitLabAppClientSecret      string `json:"-"`
	BitbucketAppClientID       string `json:"-"`
	BitbucketAppClientSecret   string `json:"-"`
	AzureDevOpsAppClientID     string `json:"-"`
	AzureDevOpsAppClientSecret string `json:"-"`
	OidcProviderName           string `json:"-"`
	OidcIssuerURL              string `json:"-"`
	OidcClientID               string `json:"-"`
	OidcClientSecret           string `json:"-"`
	SecretsDir                 string `json:"secrets_dir"`
	FolderID                   string `json:"folder_id"`
	SSHPublicKeyPath           string `json:"-"`
	SSHPrivateKeyPath          string `json:"-"`
	DatacenterID               int    `json:"-"`
	DatacenterName             string `json:"-"`
	CustomPgIP                 string `json:"custom_pg_ip"`
	Region                     string `json:"region"`
	Zone                       string `json:"zone"`
	DNSZoneName                string `json:"dns_zone_name"`

	// Test user creation
	CreateTestUser bool   `json:"-"`
	OmsWorkdir     string `json:"-"`
	RootDiskSize   int64  `json:"root_disk_size"`
}

func NewGCPBootstrapper(
	ctx context.Context,
	env env.Env,
	stlog *bootstrap.StepLogger,
	CodesphereEnv *CodesphereEnvironment,
	icg installer.InstallConfigManager,
	gcpClient GCPClientManager,
	fw util.FileIO,
	sshRunner node.NodeClient,
	portalClient portal.Portal,
	time util.Time,
	gitHubClient github.GitHubClient,
) (*GCPBootstrapper, error) {
	return &GCPBootstrapper{
		ctx:              ctx,
		stlog:            stlog,
		fw:               fw,
		icg:              icg,
		GCPClient:        gcpClient,
		Env:              CodesphereEnv,
		NodeClient:       sshRunner,
		PortalClient:     portalClient,
		Time:             time,
		GitHubClient:     gitHubClient,
		OmsBinaryBuilder: BuildOmsLinuxBinary,
	}, nil
}

func (b *GCPBootstrapper) Bootstrap() error {
	err := b.stlog.Step("Validate input", b.ValidateInput)
	if err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}

	err = b.stlog.Step("Ensure install config", b.EnsureInstallConfig)
	if err != nil {
		return fmt.Errorf("failed to ensure install config: %w", err)
	}

	err = b.stlog.Step("Ensure secrets", b.EnsureSecrets)
	if err != nil {
		return fmt.Errorf("failed to ensure secrets: %w", err)
	}

	err = b.stlog.Step("Ensure project", b.EnsureProject)
	if err != nil {
		return fmt.Errorf("failed to ensure GCP project: %w", err)
	}

	err = b.stlog.Step("Write infrastructure file", b.WriteInfraFile)
	if err != nil {
		return fmt.Errorf("failed to write infrastructure file: %w", err)
	}

	err = b.stlog.Step("Ensure billing", b.EnsureBilling)
	if err != nil {
		return fmt.Errorf("failed to ensure billing is enabled: %w", err)
	}

	err = b.stlog.Step("Ensure APIs enabled", b.EnsureAPIsEnabled)
	if err != nil {
		return fmt.Errorf("failed to enable required APIs: %w", err)
	}

	if b.Env.RegistryType == RegistryTypeArtifactRegistry {
		err = b.stlog.Step("Ensure artifact registry", b.EnsureArtifactRegistry)
		if err != nil {
			return fmt.Errorf("failed to ensure artifact registry: %w", err)
		}
	}

	err = b.stlog.Step("Ensure service accounts", b.EnsureServiceAccounts)
	if err != nil {
		return fmt.Errorf("failed to ensure service accounts: %w", err)
	}

	err = b.stlog.Step("Ensure IAM roles", b.EnsureIAMRoles)
	if err != nil {
		return fmt.Errorf("failed to ensure IAM roles: %w", err)
	}

	err = b.stlog.Step("Ensure VPC", b.EnsureVPC)
	if err != nil {
		return fmt.Errorf("failed to ensure VPC: %w", err)
	}

	err = b.stlog.Step("Ensure firewall rules", b.EnsureFirewallRules)
	if err != nil {
		return fmt.Errorf("failed to ensure firewall rules: %w", err)
	}

	err = b.stlog.Step("Ensure compute instances", b.EnsureComputeInstances)
	if err != nil {
		return fmt.Errorf("failed to ensure compute instances: %w", err)
	}

	err = b.stlog.Step("Ensure gateway IP addresses", b.EnsureGatewayIPAddresses)
	if err != nil {
		return fmt.Errorf("failed to ensure external IP addresses: %w", err)
	}

	err = b.stlog.Step("Ensure root login enabled", b.EnsureRootLoginEnabled)
	if err != nil {
		return fmt.Errorf("failed to ensure root login is enabled: %w", err)
	}

	err = b.stlog.Step("Ensure jumpbox configured", b.EnsureJumpboxConfigured)
	if err != nil {
		return fmt.Errorf("failed to ensure jumpbox is configured: %w", err)
	}

	err = b.stlog.Step("Ensure hosts are configured", b.EnsureHostsConfigured)
	if err != nil {
		return fmt.Errorf("failed to ensure hosts are configured: %w", err)
	}

	if b.Env.RegistryType == RegistryTypeLocalContainer {
		err = b.stlog.Step("Ensure local container registry", b.EnsureLocalContainerRegistry)
		if err != nil {
			return fmt.Errorf("failed to ensure local container registry: %w", err)
		}
	}

	if b.Env.RegistryType == RegistryTypeGitHub {
		err = b.stlog.Step("Ensure GitHub access configured", b.EnsureGitHubAccessConfigured)
		if err != nil {
			return fmt.Errorf("failed to update install config: %w", err)
		}
	}

	if b.Env.WriteConfig {
		err = b.stlog.Step("Update install config", b.UpdateInstallConfig)
		if err != nil {
			return fmt.Errorf("failed to update install config: %w", err)
		}

		err = b.stlog.Step("Ensure age key", b.EnsureAgeKey)
		if err != nil {
			return fmt.Errorf("failed to ensure age key: %w", err)
		}

		err = b.stlog.Step("Encrypt vault", b.EncryptVault)
		if err != nil {
			return fmt.Errorf("failed to encrypt vault: %w", err)
		}
	}

	err = b.stlog.Step("Ensure DNS records", b.EnsureDNSRecords)
	if err != nil {
		return fmt.Errorf("failed to ensure DNS records: %w", err)
	}

	err = b.stlog.Step("Generate k0s config script", b.GenerateK0sConfigScript)
	if err != nil {
		return fmt.Errorf("failed to generate k0s config script: %w", err)
	}

	if b.Env.InstallVersion != "" || b.Env.InstallLocal != "" {
		err = b.stlog.Step("Install Codesphere", b.InstallCodesphere)
		if err != nil {
			return fmt.Errorf("failed to install Codesphere: %w", err)
		}

		err = b.stlog.Step("Run k0s config script", b.RunK0sConfigScript)
		if err != nil {
			return fmt.Errorf("failed to run k0s config script: %w", err)
		}
	}

	if b.Env.CreateTestUser {
		if err := b.createTestUser(); err != nil {
			log.Printf("warning: failed to create test user: %v", err)
		}
	}

	return nil
}

// createTestUser creates a test user in the PostgreSQL instance using the testuser package and logs the credentials.
func (b *GCPBootstrapper) createTestUser() error {
	if b.Env.PostgreSQLNode == nil {
		return fmt.Errorf("postgres node not found in bootstrap environment")
	}

	pgHost := b.Env.PostgreSQLNode.GetExternalIP()
	if pgHost == "" {
		return fmt.Errorf("postgres node has no external IP")
	}

	if b.Env.InstallConfig == nil {
		return fmt.Errorf("install config not found in bootstrap environment")
	}
	pgPasswordSecret := b.icg.GetVault().GetSecret(files.SecretPostgresPassword)
	if pgPasswordSecret == nil || pgPasswordSecret.Fields == nil {
		return fmt.Errorf("postgres admin password not found in vault")
	}
	pgPassword := pgPasswordSecret.Fields.Password

	result, err := testuser.CreateTestUser(testuser.CreateTestUserOpts{
		Host:         pgHost,
		Port:         testuser.DefaultPort,
		User:         testuser.DefaultUser,
		Password:     pgPassword,
		DBName:       testuser.DefaultDBName,
		SSLMode:      "require",
		DatacenterID: b.Env.DatacenterID,
	})
	if err != nil {
		return err
	}

	testuser.LogAndPersistResult(result, b.Env.OmsWorkdir)
	return nil
}
func (b *GCPBootstrapper) ValidateInput() error {
	err := b.validateInstallVersion()
	if err != nil {
		return err
	}

	err = b.validateVMProvisioningOptions()
	if err != nil {
		return err
	}

	err = b.validateGitHubParams()
	if err != nil {
		return err
	}

	err = b.validateGitProviderParams()
	if err != nil {
		return err
	}

	err = b.validateOidcParams()
	if err != nil {
		return err
	}

	err = b.validateExternalLokiParams()
	if err != nil {
		return err
	}

	err = b.validatePrometheusRemoteWriteParams()
	if err != nil {
		return err
	}

	return b.validateTelemetryExportParams()
}

// validateInstallVersion checks if the specified install version exists and contains the required installer artifact
func (b *GCPBootstrapper) validateInstallVersion() error {
	if b.Env.InstallLocal != "" {
		if b.Env.InstallVersion != "" || b.Env.InstallHash != "" {
			return fmt.Errorf("cannot specify both install-local and install-version/install-hash")
		}
		if !b.fw.Exists(b.Env.InstallLocal) {
			return fmt.Errorf("local installer package not found at path: %s", b.Env.InstallLocal)
		}
		return nil
	}
	if b.Env.InstallVersion == "" {
		return nil
	}
	build, err := b.PortalClient.GetBuild(portal.CodesphereProduct, b.Env.InstallVersion, b.Env.InstallHash)
	if err != nil {
		return fmt.Errorf("failed to get codesphere package: %w", err)
	}

	if b.Env.InstallHash == "" {
		b.Env.InstallHash = build.Hash
	}

	requiredFilename := "installer.tar.gz"
	if b.Env.RegistryType == RegistryTypeGitHub {
		requiredFilename = "installer-lite.tar.gz"
	}
	filenames := []string{}
	// Validate required file exists in package artifacts
	for _, artifact := range build.Artifacts {
		filenames = append(filenames, artifact.Filename)
		if artifact.Filename == requiredFilename {
			return nil
		}
	}

	return fmt.Errorf("specified package does not contain required installer artifact %s. Existing artifacts: %s", requiredFilename, strings.Join(filenames, ", "))
}

// validateGitHubParams checks if the GitHub credentials are fully specified if GitHub registry is selected
func (b *GCPBootstrapper) validateGitHubParams() error {
	if b.Env.GitHubTeamSlug != "" && b.Env.GitHubTeamOrg != "" && b.Env.GitHubPAT == "" {
		return fmt.Errorf("GitHub PAT is required to extract public keys of GitHub team members")
	}

	ghTeamParams := []string{b.Env.GitHubTeamSlug, b.Env.GitHubTeamOrg}
	if slices.Contains(ghTeamParams, "") && strings.Join(ghTeamParams, "") != "" {
		return fmt.Errorf("GitHub team parameters are not fully specified (all or none of GitHubTeamSlug, GitHubTeamOrg must be set)")
	}

	ghAppParams := []string{b.Env.GitHubAppName, b.Env.GitHubAppClientID, b.Env.GitHubAppClientSecret}
	if slices.Contains(ghAppParams, "") && strings.Join(ghAppParams, "") != "" {
		return fmt.Errorf("GitHub app credentials are not fully specified (all or none of GitHubAppName, GitHubAppClientID, GitHubAppClientSecret must be set)")
	}

	return nil
}

// validateGitProviderParams checks that git provider credentials are fully specified (both client ID and secret, or neither)
func (b *GCPBootstrapper) validateGitProviderParams() error {
	providers := []struct {
		name   string
		id     string
		secret string
	}{
		{"GitLab", b.Env.GitLabAppClientID, b.Env.GitLabAppClientSecret},
		{"Bitbucket", b.Env.BitbucketAppClientID, b.Env.BitbucketAppClientSecret},
		{"Azure DevOps", b.Env.AzureDevOpsAppClientID, b.Env.AzureDevOpsAppClientSecret},
	}

	for _, p := range providers {
		if p.id != "" && p.secret == "" {
			return fmt.Errorf("%s client ID is set but client secret is missing", p.name)
		}
		if p.secret != "" && p.id == "" {
			return fmt.Errorf("%s client secret is set but client ID is missing", p.name)
		}
	}

	return nil
}

// validateOidcParams checks that OIDC OAuth provider credentials are fully specified (all or none of issuer URL, client ID, client secret)
func (b *GCPBootstrapper) validateOidcParams() error {
	oidcParams := []string{b.Env.OidcIssuerURL, b.Env.OidcClientID, b.Env.OidcClientSecret}
	if slices.Contains(oidcParams, "") && strings.Join(oidcParams, "") != "" {
		return fmt.Errorf("OIDC OAuth provider credentials are not fully specified (all or none of OidcIssuerURL, OidcClientID, OidcClientSecret must be set)")
	}

	return nil
}

func (b *GCPBootstrapper) validateExternalLokiParams() error {
	if b.Env.ExternalLokiEndpoint != "" {
		return nil
	}

	if b.Env.ExternalLokiSecret != "" || b.Env.ExternalLokiUser != "" {
		return fmt.Errorf("external Loki endpoint is required when external Loki secret or user is set")
	}

	return nil
}

func (b *GCPBootstrapper) validatePrometheusRemoteWriteParams() error {
	if b.Env.PrometheusRemoteWriteURL != "" && (b.Env.PrometheusRemoteWriteUser == "" || b.Env.PrometheusRemoteWritePassword == "") {
		return fmt.Errorf("prometheus remote write username and password must both be set when remote write URL is specified")
	}
	if (b.Env.PrometheusRemoteWriteUser != "" || b.Env.PrometheusRemoteWritePassword != "") && b.Env.PrometheusRemoteWriteURL == "" {
		return fmt.Errorf("prometheus remote write URL is required when remote write username or password is set")
	}
	return nil
}

func (b *GCPBootstrapper) validateTelemetryExportParams() error {
	if b.Env.CentralOtelEndpoint != "" && b.Env.CentralOtelPassword == "" {
		return fmt.Errorf("central OTel password is required when central OTel endpoint is set")
	}

	if b.Env.CentralOtelUsername != "" && b.Env.CentralOtelPassword == "" {
		return fmt.Errorf("central OTel username is set but password is missing")
	}
	if b.Env.CentralOtelPassword != "" && b.Env.CentralOtelUsername == "" {
		return fmt.Errorf("central OTel password is set but username is missing")
	}

	return nil
}

func (b *GCPBootstrapper) EnsureArtifactRegistry() error {
	repoName := "codesphere-registry"

	repo, err := b.GCPClient.GetArtifactRegistry(b.Env.ProjectID, b.Env.Region, repoName)
	if err == nil && repo != nil {
		b.Env.InstallConfig.Registry.Server = repo.GetRegistryUri()
		return nil
	}

	repo, err = b.GCPClient.CreateArtifactRegistry(b.Env.ProjectID, b.Env.Region, repoName)
	if err != nil || repo == nil {
		return fmt.Errorf("failed to create artifact registry: %w, repo: %v", err, repo)
	}

	return nil
}

func (b *GCPBootstrapper) ensureDnsPermissions() error {
	dnsProject := b.Env.DNSProjectID
	if b.Env.DNSProjectID == "" {
		dnsProject = b.Env.ProjectID
	}
	err := b.ensureIAMRoleWithRetry(dnsProject, "cloud-controller", b.Env.ProjectID, []string{"roles/dns.admin"})
	if err != nil {
		return err
	}
	return nil
}

func (b *GCPBootstrapper) EnsureVPC() error {
	networkName := fmt.Sprintf("%s-vpc", b.Env.ProjectID)
	subnetName := fmt.Sprintf("%s-%s-subnet", b.Env.ProjectID, b.Env.Region)
	routerName := fmt.Sprintf("%s-router", b.Env.ProjectID)
	natName := fmt.Sprintf("%s-nat-gateway", b.Env.ProjectID)

	// Create VPC
	err := b.GCPClient.CreateVPC(b.Env.ProjectID, b.Env.Region, networkName, subnetName, routerName, natName)
	if err != nil {
		return fmt.Errorf("failed to ensure VPC: %w", err)
	}

	return nil
}

func (b *GCPBootstrapper) EnsureFirewallRules() error {
	networkName := fmt.Sprintf("%s-vpc", b.Env.ProjectID)

	// Allow external SSH to Jumpbox
	sshRule := &computepb.Firewall{
		Name:      protoString("allow-ssh-ext"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", b.Env.ProjectID, networkName)),
		Direction: protoString("INGRESS"),
		Priority:  protoInt32(1000),
		Allowed: []*computepb.Allowed{
			{
				IPProtocol: protoString("tcp"),
				Ports:      []string{"22"},
			},
		},
		SourceRanges: []string{"0.0.0.0/0"},
		TargetTags:   []string{"ssh"},
		Description:  protoString("Allow external SSH to Jumpbox"),
	}
	err := b.GCPClient.CreateFirewallRule(b.Env.ProjectID, sshRule)
	if err != nil {
		return fmt.Errorf("failed to create jumpbox ssh firewall rule: %w", err)
	}

	// Allow all internal traffic
	internalRule := &computepb.Firewall{
		Name:      protoString("allow-internal"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", b.Env.ProjectID, networkName)),
		Direction: protoString("INGRESS"),
		Priority:  protoInt32(1000),
		Allowed: []*computepb.Allowed{
			{IPProtocol: protoString("all")},
		},
		SourceRanges: []string{"10.10.0.0/20"},
		Description:  protoString("Allow all internal traffic"),
	}
	err = b.GCPClient.CreateFirewallRule(b.Env.ProjectID, internalRule)
	if err != nil {
		return fmt.Errorf("failed to create internal firewall rule: %w", err)
	}

	// Allow all egress
	egressRule := &computepb.Firewall{
		Name:      protoString("allow-all-egress"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", b.Env.ProjectID, networkName)),
		Direction: protoString("EGRESS"),
		Priority:  protoInt32(1000),
		Allowed: []*computepb.Allowed{
			{IPProtocol: protoString("all")},
		},
		DestinationRanges: []string{"0.0.0.0/0"},
		Description:       protoString("Allow all egress"),
	}
	err = b.GCPClient.CreateFirewallRule(b.Env.ProjectID, egressRule)
	if err != nil {
		return fmt.Errorf("failed to create egress firewall rule: %w", err)
	}

	// Allow ingress for web (HTTP/HTTPS)
	webRule := &computepb.Firewall{
		Name:      protoString("allow-ingress-web"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", b.Env.ProjectID, networkName)),
		Direction: protoString("INGRESS"),
		Priority:  protoInt32(1000),
		Allowed: []*computepb.Allowed{
			{IPProtocol: protoString("tcp"), Ports: []string{"80", "443"}},
		},
		SourceRanges: []string{"0.0.0.0/0"},
		Description:  protoString("Allow HTTP/HTTPS ingress"),
	}
	err = b.GCPClient.CreateFirewallRule(b.Env.ProjectID, webRule)
	if err != nil {
		return fmt.Errorf("failed to create web firewall rule: %w", err)
	}

	// Allow ingress for PostgreSQL
	postgresRule := &computepb.Firewall{
		Name:      protoString("allow-ingress-postgres"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", b.Env.ProjectID, networkName)),
		Direction: protoString("INGRESS"),
		Priority:  protoInt32(1000),
		Allowed: []*computepb.Allowed{
			{IPProtocol: protoString("tcp"), Ports: []string{"5432"}},
		},
		SourceRanges: []string{"0.0.0.0/0"},
		TargetTags:   []string{"postgres"},
		Description:  protoString("Allow external access to PostgreSQL"),
	}
	err = b.GCPClient.CreateFirewallRule(b.Env.ProjectID, postgresRule)
	if err != nil {
		return fmt.Errorf("failed to create postgres firewall rule: %w", err)
	}

	return nil
}

// EnsureGatewayIPAddresses reserves 2 static external IP addresses for the ingress
// controllers of the cluster.
func (b *GCPBootstrapper) EnsureGatewayIPAddresses() error {
	var err error
	b.Env.GatewayIP, err = b.EnsureExternalIP("gateway")
	if err != nil {
		return fmt.Errorf("failed to ensure gateway IP: %w", err)
	}
	b.Env.PublicGatewayIP, err = b.EnsureExternalIP("public-gateway")
	if err != nil {
		return fmt.Errorf("failed to ensure public gateway IP: %w", err)
	}

	return nil
}

// EnsureExternalIP ensures that a static external IP address with the given name exists.
func (b *GCPBootstrapper) EnsureExternalIP(name string) (string, error) {
	desiredAddress := &computepb.Address{
		Name:        &name,
		AddressType: protoString("EXTERNAL"),
		Region:      &b.Env.Region,
	}

	// Figure out if address already exists and get IP
	address, err := b.GCPClient.GetAddress(b.Env.ProjectID, b.Env.Region, name)
	if err == nil && address != nil {
		return address.GetAddress(), nil
	}

	createdIP, err := b.GCPClient.CreateAddress(b.Env.ProjectID, b.Env.Region, desiredAddress)
	if err != nil && !IsAlreadyExistsError(err) {
		return "", fmt.Errorf("failed to create address %s: %w", name, err)
	}

	if createdIP != "" {
		return createdIP, nil
	}

	address, err = b.GCPClient.GetAddress(b.Env.ProjectID, b.Env.Region, name)

	if err == nil && address != nil {
		return address.GetAddress(), nil
	}

	return "", fmt.Errorf("failed to get address %s after creation", name)
}

func (b *GCPBootstrapper) EnsureRootLoginEnabled() error {
	allNodes := []*node.Node{
		b.Env.Jumpbox,
	}
	allNodes = append(allNodes, b.Env.ControlPlaneNodes...)
	allNodes = append(allNodes, b.Env.PostgreSQLNode)
	allNodes = append(allNodes, b.Env.CephNodes...)

	for _, node := range allNodes {
		err := b.stlog.Substep(fmt.Sprintf("Ensuring root login enabled on %s", node.GetName()), func() error {
			return b.ensureRootLoginEnabledInNode(node)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *GCPBootstrapper) ensureRootLoginEnabledInNode(node *node.Node) error {
	err := node.NodeClient.WaitReady(node, 30*time.Second)
	if err != nil {
		return fmt.Errorf("timed out waiting for SSH service to start on %s: %w", node.GetName(), err)
	}

	hasRootLogin := node.HasRootLoginEnabled()
	if hasRootLogin {
		return nil
	}

	for i := range 3 {
		err := node.EnableRootLogin()
		if err == nil {
			break
		}
		if i == 2 {
			return fmt.Errorf("failed to enable root login on %s: %w", node.GetName(), err)
		}
		b.stlog.LogRetry()
		b.Time.Sleep(10 * time.Second)
	}

	return nil
}

func (b *GCPBootstrapper) EnsureJumpboxConfigured() error {
	if !b.Env.Jumpbox.HasAcceptEnvConfigured() {
		err := b.Env.Jumpbox.ConfigureAcceptEnv()
		if err != nil {
			return fmt.Errorf("failed to configure AcceptEnv on jumpbox: %w", err)
		}
	}

	hasOms := b.Env.Jumpbox.HasCommand("oms")
	if hasOms {
		return nil
	}

	err := b.Env.Jumpbox.InstallOms()
	if err != nil {
		return fmt.Errorf("failed to install OMS on jumpbox: %w", err)
	}

	return nil
}

func (b *GCPBootstrapper) EnsureHostsConfigured() error {
	allNodes := append(b.Env.ControlPlaneNodes, b.Env.PostgreSQLNode)
	allNodes = append(allNodes, b.Env.CephNodes...)

	for _, node := range allNodes {
		if !node.HasInotifyWatchesConfigured() {
			err := node.ConfigureInotifyWatches()
			if err != nil {
				return fmt.Errorf("failed to configure inotify watches on %s: %w", node.GetName(), err)
			}
		}
		if !node.HasMemoryMapConfigured() {
			err := node.ConfigureMemoryMap()
			if err != nil {
				return fmt.Errorf("failed to configure memory map on %s: %w", node.GetName(), err)
			}
		}
	}

	return nil
}

// EnsureLocalContainerRegistry installs a docker registry on the postgres node to speed up image loading time
func (b *GCPBootstrapper) EnsureLocalContainerRegistry() error {
	localRegistryServer := b.Env.PostgreSQLNode.GetInternalIP() + ":5000"

	// Figure out if registry is already running
	b.stlog.Logf("Checking if local container registry is already running on postgres node")
	checkCommand := `test "$(podman ps --filter 'name=registry' --format '{{.Names}}' | wc -l)" -eq "1"`
	err := b.Env.PostgreSQLNode.RunSSHCommand("root", checkCommand)
	registryUsername := ""
	registryPassword := ""
	if s := b.icg.GetVault().GetSecret(files.SecretRegistryUsername); s != nil && s.Fields != nil {
		registryUsername = s.Fields.Password
	}
	if s := b.icg.GetVault().GetSecret(files.SecretRegistryPassword); s != nil && s.Fields != nil {
		registryPassword = s.Fields.Password
	}
	if err == nil && b.Env.InstallConfig.Registry != nil && b.Env.InstallConfig.Registry.Server == localRegistryServer &&
		registryUsername != "" && registryPassword != "" {
		b.stlog.Logf("Local container registry already running on postgres node")
		return nil
	}

	b.Env.InstallConfig.Registry.Server = localRegistryServer
	registryUsername = "custom-registry"
	registryPassword = shortuuid.New()
	b.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretRegistryUsername, Fields: &files.SecretFields{Password: registryUsername}})
	b.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretRegistryPassword, Fields: &files.SecretFields{Password: registryPassword}})

	commands := []string{
		"apt-get update",
		"apt-get install -y podman apache2-utils",
		"htpasswd -bBc /root/registry.password " + registryUsername + " " + registryPassword,
		"openssl req -newkey rsa:4096 -nodes -sha256 -keyout /root/registry.key -x509 -days 365 -out /root/registry.crt -subj \"/C=DE/ST=BW/L=Karlsruhe/O=Codesphere/CN=" + b.Env.PostgreSQLNode.GetInternalIP() + "\" -addext \"subjectAltName = DNS:postgres,IP:" + b.Env.PostgreSQLNode.GetInternalIP() + "\"",
		"podman rm -f registry || true",
		`podman run -d \
		--restart=always --name registry --net=host\
		--env REGISTRY_HTTP_ADDR=0.0.0.0:5000 \
		--env REGISTRY_AUTH=htpasswd \
		--env REGISTRY_AUTH_HTPASSWD_REALM='Registry Realm' \
		--env REGISTRY_AUTH_HTPASSWD_PATH=/auth/registry.password \
		-v /root/registry.password:/auth/registry.password \
		--env REGISTRY_HTTP_TLS_CERTIFICATE=/certs/registry.crt \
		--env REGISTRY_HTTP_TLS_KEY=/certs/registry.key \
		-v /root/registry.crt:/certs/registry.crt \
		-v /root/registry.key:/certs/registry.key \
		registry:2`,
		`mkdir -p /etc/docker/certs.d/` + b.Env.InstallConfig.Registry.Server,
		`cp /root/registry.crt /etc/docker/certs.d/` + b.Env.InstallConfig.Registry.Server + `/ca.crt`,
	}
	for _, cmd := range commands {
		b.stlog.Logf("Running command on postgres node: %s", util.Truncate(cmd, 12))
		err := b.Env.PostgreSQLNode.RunSSHCommand("root", cmd)
		if err != nil {
			return fmt.Errorf("failed to run command on postgres node: %w", err)
		}
	}

	allNodes := append(b.Env.ControlPlaneNodes, b.Env.CephNodes...)
	for _, node := range allNodes {
		b.stlog.Logf("Configuring node '%s' to trust local registry certificate", node.GetName())
		err := b.Env.PostgreSQLNode.RunSSHCommand("root", "scp -o StrictHostKeyChecking=no /root/registry.crt root@"+node.GetInternalIP()+":/usr/local/share/ca-certificates/registry.crt")
		if err != nil {
			return fmt.Errorf("failed to copy registry certificate to node %s: %w", node.GetInternalIP(), err)
		}
		err = node.RunSSHCommand("root", "update-ca-certificates")
		if err != nil {
			return fmt.Errorf("failed to update CA certificates on node %s: %w", node.GetInternalIP(), err)
		}
		err = node.RunSSHCommand("root", "systemctl restart docker.service || true") // docker is probably not yet installed
		if err != nil {
			return fmt.Errorf("failed to restart docker service on node %s: %w", node.GetInternalIP(), err)
		}
	}

	return nil
}

func (b *GCPBootstrapper) EnsureGitHubAccessConfigured() error {
	if b.Env.GitHubPAT == "" {
		return fmt.Errorf("GitHub PAT is not set")
	}
	b.Env.InstallConfig.Registry.Server = "ghcr.io"
	b.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretRegistryUsername, Fields: &files.SecretFields{Password: b.Env.RegistryUser}})
	b.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretRegistryPassword, Fields: &files.SecretFields{Password: b.Env.GitHubPAT}})
	b.Env.InstallConfig.Registry.ReplaceImagesInBom = false
	b.Env.InstallConfig.Registry.LoadContainerImages = false
	return nil
}

func (b *GCPBootstrapper) EnsureDNSRecords() error {
	gcpProject := b.Env.DNSProjectID
	if b.Env.DNSProjectID == "" {
		gcpProject = b.Env.ProjectID
	}

	zoneName := b.Env.DNSZoneName
	err := b.GCPClient.EnsureDNSManagedZone(gcpProject, zoneName, b.Env.BaseDomain+".", "Codesphere DNS zone")
	if err != nil {
		return fmt.Errorf("failed to ensure DNS managed zone: %w", err)
	}

	records := []*dns.ResourceRecordSet{
		{
			Name:    fmt.Sprintf("cs.%s.", b.Env.BaseDomain),
			Type:    "A",
			Ttl:     300,
			Rrdatas: []string{b.Env.GatewayIP},
		},
		{
			Name:    fmt.Sprintf("*.cs.%s.", b.Env.BaseDomain),
			Type:    "A",
			Ttl:     300,
			Rrdatas: []string{b.Env.GatewayIP},
		},
		{
			Name:    fmt.Sprintf("*.ws.%s.", b.Env.BaseDomain),
			Type:    "A",
			Ttl:     300,
			Rrdatas: []string{b.Env.PublicGatewayIP},
		},
		{
			Name:    fmt.Sprintf("ws.%s.", b.Env.BaseDomain),
			Type:    "A",
			Ttl:     300,
			Rrdatas: []string{b.Env.PublicGatewayIP},
		},
	}

	err = b.GCPClient.EnsureDNSRecordSets(gcpProject, zoneName, records)
	if err != nil {
		return fmt.Errorf("failed to ensure DNS record sets: %w", err)
	}

	return nil
}

func (b *GCPBootstrapper) InstallCodesphere() error {
	fullPackageFilename, err := b.ensureCodespherePackageOnJumpbox()
	if err != nil {
		return fmt.Errorf("failed to ensure Codesphere package on jumpbox: %w", err)
	}

	if ltsSpec := FindLTSSpec(b.Env.InstallVersion); ltsSpec != nil {
		if ltsSpec.RequiresOmsBinaryUpdate {
			if err := b.ensureNewOmsBinaryOnJumpbox(); err != nil {
				return fmt.Errorf("failed to update OMS binary on jumpbox for %s: %w", b.Env.InstallVersion, err)
			}
		}
		if ltsSpec.RequiresCephMasterWatcher {
			b.startLTSCephMasterWatcher()
			defer b.stopLTSCephMasterWatcher()
		}
		return b.runLTSInstallPhases(fullPackageFilename, ltsSpec)
	}

	err = b.runInstallCommand(fullPackageFilename)
	if err != nil {
		return fmt.Errorf("failed to install Codesphere from jumpbox: %w", err)
	}

	return nil
}

// runLTSInstallPhases runs the three install phases separately for LTS versions.
// Phases 1 (infra) and 2 (dependencies) run without inter-node SSH; steps that
// need SSH (set-up-cluster, ms-backends, codesphere) are skipped. An SSH key
// is then copied to the jumpbox, and Phase 3 (platform) runs with codesphere
// included so the platform is deployed with inter-node SSH available.
func (b *GCPBootstrapper) runLTSInstallPhases(packageFilename string, ltsSpec *LTSSpec) error {
	ltsSkips := []string{"set-up-cluster", "ms-backends"}
	if ltsSpec.SkipPcApps {
		ltsSkips = append(ltsSkips, "argocd")
	}

	// Phase 1: Infrastructure (docker, postgres, ceph, kubernetes) — no SSH needed.
	b.stlog.Logf("Running infrastructure phase (Phase 1)...")
	infraSkips := append([]string{"codesphere"}, ltsSkips...)
	if err := b.runInstallPhase(packageFilename, "infra", infraSkips); err != nil {
		return fmt.Errorf("infra phase failed: %w", err)
	}

	// Phase 2: Dependencies (copy/extract) — skip SSH-needing steps.
	b.stlog.Logf("Running dependencies phase (Phase 2)...")
	if err := b.runInstallPhase(packageFilename, "dependencies", ltsSkips); err != nil {
		return fmt.Errorf("dependencies phase failed: %w", err)
	}

	// Set up SSH key so the jumpbox can reach the postgres VM for
	// database creation below.
	if ltsSpec.RequiresSSHKeyOnJumpbox {
		if err := b.ensureSSHKeyOnJumpbox(); err != nil {
			return err
		}
	}

	// Phase 3: Deploy Codesphere via helm directly (bypasses the old LTS
	// private-cloud-installer.js which can't handle the codesphere component).
	b.stlog.Logf("Deploying Codesphere platform via helm (Phase 3)...")
	if err := b.installCodesphereViaHelm(packageFilename); err != nil {
		return fmt.Errorf("platform phase (helm) failed: %w", err)
	}

	return nil
}

func (b *GCPBootstrapper) installCodesphereViaHelm(packageFilename string) error {
	if len(b.Env.ControlPlaneNodes) == 0 {
		return fmt.Errorf("no control plane nodes available for kubeconfig")
	}
	cpIP := b.Env.ControlPlaneNodes[0].GetInternalIP()

	b.stlog.Logf("Copying kubeconfig from control plane (%s)...", cpIP)
	mkdirCmd := "mkdir -p /var/lib/k0s/pki"
	if err := b.Env.Jumpbox.RunSSHCommand("root", mkdirCmd); err != nil {
		return fmt.Errorf("failed to create k0s dir on jumpbox: %w", err)
	}
	scpCmd := fmt.Sprintf("scp -o StrictHostKeyChecking=no root@%s:/var/lib/k0s/pki/admin.conf /var/lib/k0s/pki/admin.conf", cpIP)
	if err := b.Env.Jumpbox.RunSSHCommand("root", scpCmd); err != nil {
		return fmt.Errorf("failed to copy kubeconfig from control plane: %w", err)
	}
	sedCmd := fmt.Sprintf("sed -i 's|server: https://127.0.0.1:6443|server: https://%s:6443|; s|server: https://localhost:6443|server: https://%s:6443|' /var/lib/k0s/pki/admin.conf", cpIP, cpIP)
	if err := b.Env.Jumpbox.RunSSHCommand("root", sedCmd); err != nil {
		return fmt.Errorf("failed to update kubeconfig server address: %w", err)
	}

	csValues, err := yaml.Marshal(b.Env.InstallConfig.Codesphere)
	if err != nil {
		return fmt.Errorf("failed to marshal codesphere config for helm: %w", err)
	}
	writeValuesCmd := fmt.Sprintf("cat > /etc/codesphere/codesphere-values.yaml << 'OMSEOF'\n%s\nOMSEOF", string(csValues))
	if err := b.Env.Jumpbox.RunSSHCommand("root", writeValuesCmd); err != nil {
		return fmt.Errorf("failed to write codesphere values on jumpbox: %w", err)
	}

	globalVals := b.buildGlobalHelmValues()
	globalYAML, err := yaml.Marshal(map[string]interface{}{"global": globalVals})
	if err != nil {
		return fmt.Errorf("failed to marshal global helm values: %w", err)
	}
	writeGlobalCmd := fmt.Sprintf("cat > /etc/codesphere/global-values.yaml << 'OMSEOF'\n%s\nOMSEOF", string(globalYAML))
	if err := b.Env.Jumpbox.RunSSHCommand("root", writeGlobalCmd); err != nil {
		return fmt.Errorf("failed to write global values on jumpbox: %w", err)
	}

	script := `set -e
export KUBECONFIG=/var/lib/k0s/pki/admin.conf

KUBECTL=$(find /root/oms-workdir -name kubectl -type f 2>/dev/null | head -1)
HELM=$(find /root/oms-workdir -name "helm" -type f 2>/dev/null | head -1)
[ -z "$KUBECTL" ] && echo "ERROR: kubectl not found" && exit 1
[ -z "$HELM" ] && echo "ERROR: helm binary not found" && exit 1
chmod +x "$KUBECTL" "$HELM" 2>/dev/null || true

CHART=$(find /root/oms-workdir -path "*/deps/codesphere/files/chart/Chart.yaml" 2>/dev/null | head -1 | xargs dirname)
[ -z "$CHART" ] && echo "ERROR: codesphere chart not found" && exit 1

VAULT_FILE=/etc/codesphere/secrets/prod.vault.yaml
AGE_KEY=/etc/codesphere/secrets/age_key.txt
SECRETS_VALUES=/etc/codesphere/secrets-values.yaml

echo "Installing prerequisite CRDs..."
# cert-manager CRDs (needed for Certificate and Issuer resources)
"$KUBECTL" apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.crds.yaml
# Prometheus PodMonitor CRD
"$KUBECTL" apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.68.0/example/prometheus-operator-crd/monitoring.coreos.com_podmonitors.yaml

echo "Creating prerequisite namespaces and issuer..."
"$KUBECTL" create ns workspaces --dry-run=client -o yaml | "$KUBECTL" apply -f -
"$KUBECTL" create ns ws-o11y --dry-run=client -o yaml | "$KUBECTL" apply -f -
"$KUBECTL" apply -f - << 'ISSUER_EOF'
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: codesphere-issuer
spec:
  selfSigned: {}
ISSUER_EOF

# Decrypt vault and generate secrets values (maps vault secrets to global.*)
echo "Decrypting vault and generating helm secrets values..."
SOPS_AGE_KEY_FILE="$AGE_KEY" sops --decrypt "$VAULT_FILE" | python3 -c '
import sys, yaml
vault = yaml.safe_load(sys.stdin)
result = {"global": {}}
for s in vault.get("secrets", []):
    name = s["name"]
    if "fields" in s and s["fields"]:
        result["global"][name] = s["fields"].get("password", "")
    elif "file" in s and s["file"]:
        result["global"][name] = s["file"].get("content", "")
yaml.dump(result, sys.stdout, default_flow_style=False)
' > "$SECRETS_VALUES"
echo "Generated secrets values file."

# Clean up any orphaned resources from previous failed install attempts.
# Helm refuses to adopt resources that lack its ownership labels.
"$HELM" uninstall codesphere -n codesphere 2>/dev/null || true
"$KUBECTL" delete svc,deploy,ingress,configmap,secret,netpol,certificate,issuer --all -n codesphere 2>/dev/null || true

# Create GHCR pull secret for image pulling.
%s

# Create postgres Service + Endpoints so codesphere pods can reach
# the external postgres VM, and create required databases.
# (Run after cleanup so the Service isn't deleted.)
PG_IP=$(python3 -c "import yaml; c=yaml.safe_load(open('/etc/codesphere/config.yaml')); print(c.get('postgres',{}).get('primary',{}).get('ip',''))")
if [ -n "$PG_IP" ]; then
  "$KUBECTL" apply -f - << SERVICE_EOF
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: codesphere
spec:
  ports:
  - port: 5432
    targetPort: 5432
---
apiVersion: v1
kind: Endpoints
metadata:
  name: postgres
  namespace: codesphere
subsets:
- addresses:
  - ip: $PG_IP
  ports:
  - port: 5432
SERVICE_EOF
  echo "Created postgres Service → $PG_IP"
  ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 root@"$PG_IP" \
    "docker exec codesphere-postgres.service psql -U postgres -c 'CREATE DATABASE codesphere;' 2>/dev/null; \
     docker exec codesphere-postgres.service psql -U postgres -c 'CREATE DATABASE user_activity;' 2>/dev/null; \
     echo 'Databases ready.'" || echo "Warning: could not create databases on $PG_IP"
fi

echo "Installing codesphere platform via helm..."
"$HELM" upgrade --install codesphere "$CHART" \
  --namespace codesphere --create-namespace \
  -f /etc/codesphere/config.yaml \
  -f /etc/codesphere/codesphere-values.yaml \
  -f /etc/codesphere/global-values.yaml \
  -f "$SECRETS_VALUES" \
  --timeout 10m
echo "Codesphere platform deployed successfully."
`
	// Build the registry secret creation snippet.
	regCredSnippet := ""
	if b.Env.GitHubPAT != "" && b.Env.RegistryUser != "" {
		regCredSnippet = fmt.Sprintf(
			`"$KUBECTL" delete secret docker-regcred -n codesphere 2>/dev/null || true
"$KUBECTL" create secret docker-registry docker-regcred \
  --docker-server=ghcr.io \
  --docker-username=%s \
  --docker-password=%s \
  -n codesphere`, b.Env.RegistryUser, b.Env.GitHubPAT)
	}
	return b.Env.Jumpbox.RunSSHCommand("root", fmt.Sprintf(script, regCredSnippet))
}

// buildGlobalHelmValues constructs the global.* helm values from the OMS
// install config, generating token keys and service tokens where needed.
// This replicates what the old LTS private-cloud-installer.js normally does.
func (b *GCPBootstrapper) buildGlobalHelmValues() map[string]interface{} {
	g := map[string]interface{}{}

	// --- Token keys + service account tokens ---
	tokenVault := &files.InstallVault{}
	// Errors are non-fatal here; we proceed with whatever is generated.
	_ = secrets.EnsureAuthKeys(tokenVault)
	_ = secrets.EnsureServiceAccountTokens(tokenVault)
	for _, s := range tokenVault.Secrets {
		if s.File != nil {
			g[s.Name] = s.File.Content
		} else if s.Fields != nil {
			g[s.Name] = s.Fields.Password
		}
	}

	// --- Optional _secrets.tpl keys (empty defaults to avoid b64enc nil panics) ---
	for _, k := range []string{
		"stripeWebhookEndpointSecret", "stripePublishableKey", "stripeSecretKey",
		"sendGridApiKey", "digitalOceanApiToken", "mongoDbPasswordEncryptionKey", "mixpanelToken",
		"facebookClientId", "facebookClientSecret",
		"googleClientId", "googleClientSecret",
		"gitHubClientId", "gitHubClientSecret",
		"bitbucketClientId", "bitbucketClientSecret",
		"gitlabClientId", "gitlabClientSecret",
		"googleCloudAvatarPrivateKey", "googleCloudVmImagesPrivateKey",
		"googleCloudAvatarBucket", "googleCloudAvatarClientEmail", "googleCloudAvatarProjectId",
		"gitlabAppClientId", "gitlabAppClientSecret",
		"bitbucketAppsClientId", "bitbucketAppsClientSecret",
		"azureDevOpsAppClientId", "azureDevOpsAppClientSecret",
		"recaptchaSecret", "recaptchaSecretV3", "recaptchaKey", "recaptchaKeyV3",
		"postgresUserTeam", "postgresPasswordTeam",
		"postgresUserUserActivity", "postgresPasswordUserActivity",
		"postgresUserWorkspace", "postgresPasswordWorkspace",
		"postgresUserPublicapi", "postgresPasswordPublicapi",
	} {
		if _, exists := g[k]; !exists {
			g[k] = ""
		}
	}

	// --- Map config sections to global.* ---
	cfg := b.Env.InstallConfig
	cs := cfg.Codesphere

	dc := cfg.Datacenter
	g["dataCenterId"] = dc.ID
	g["defaultDataCenterId"] = dc.ID
	g["dataCenters"] = []interface{}{map[string]interface{}{
		"id": dc.ID, "name": dc.Name, "city": dc.City, "countryCode": dc.CountryCode,
	}}
	g["env"] = "prod"
	g["hostName"] = cs.Domain
	g["domainName"] = cs.Domain
	g["workspaceHostingBaseDomain"] = cs.WorkspaceHostingBaseDomain
	g["dnsServers"] = cs.DNSServers

	// Experiments: chart expects {name: [envs]}, config has []string
	expMap := map[string][]string{}
	for _, e := range cs.Experiments {
		expMap[e] = []string{"prod"}
	}
	g["experiments"] = expMap

	// Features: same transformation
	featMap := map[string][]string{}
	for k, v := range cs.Features {
		if v {
			featMap[k] = []string{"prod"}
		}
	}
	g["features"] = featMap

	g["deployConfig"] = cs.DeployConfig
	g["plans"] = cs.Plans
	g["gitProviders"] = cs.GitProviders
	if g["gitProviders"] == nil {
		g["gitProviders"] = map[string]interface{}{}
	}
	// Chart expects oauth.providers.{name}.enabled, not OMS oauth.oidc format.
	g["oauth"] = map[string]interface{}{
		"providers": map[string]interface{}{
			"github":    map[string]interface{}{"enabled": false},
			"gitlab":    map[string]interface{}{"enabled": false},
			"bitbucket": map[string]interface{}{"enabled": false},
			"google":    map[string]interface{}{"enabled": false},
			"facebook":  map[string]interface{}{"enabled": false},
		},
		"additionalProviders": map[string]interface{}{},
	}
	g["customDomains"] = cs.CustomDomains
	if cs.OpenBao != nil {
		g["openBao"] = cs.OpenBao
	} else {
		g["openBao"] = map[string]interface{}{
			"engine": "cs-secrets-engine",
			"user":   "admin",
		}
	}
	g["managedServices"] = cs.ManagedServices
	g["extraCaPem"] = cs.ExtraCAPem
	g["extraWorkspaceEnvVars"] = cs.ExtraWorkspaceEnvVars
	g["extraWorkspaceFiles"] = cs.ExtraWorkspaceFiles
	g["override"] = cs.Override

	// Postgres connection config (chart uses global.postgres.*, not top-level postgres.*)
	g["postgres"] = map[string]interface{}{
		"host":                 "postgres",
		"port":                 5432,
		"database":             "postgres",
		"userActivityDatabase": "user_activity",
		"ssl":                  map[string]interface{}{"rejectUnauthorized": false},
	}
	if cfg.Postgres.Primary != nil {
		if cfg.Postgres.Primary.IP != "" {
			g["postgres"].(map[string]interface{})["host"] = cfg.Postgres.Primary.IP
		}
		if cfg.Postgres.Primary.Hostname != "" {
			g["postgres"].(map[string]interface{})["host"] = cfg.Postgres.Primary.Hostname
		}
	}

	// workspaceObservability defaults (chart references many nested fields)
	g["workspaceObservability"] = map[string]interface{}{
		"kubeNamespace":                      "ws-o11y",
		"caSecretName":                       "ws-o11y-ca",
		"certIssuerName":                     "codesphere-issuer",
		"certificatesLifetimeDays":           365,
		"certificatesRenewBeforeDays":        30,
		"openSearchImage":                    "",
		"openSearchInitImage":                "",
		"openSearchStorageClass":             "rook-ceph-block",
		"openSearchVersion":                  "",
		"otelCollectorImage":                 "",
		"otelCollectorInitImage":             "",
		"opensearchUnderprovisionFactors":    map[string]interface{}{"cpu": 100, "memory": 256},
		"otelCollectorUnderprovisionFactors": map[string]interface{}{"cpu": 100, "memory": 256},
	}

	// workspaceImages defaults
	g["workspaceImages"] = map[string]interface{}{
		"server": map[string]interface{}{},
		"vpn":    map[string]interface{}{},
	}

	// workspaceRouterImage defaults
	g["workspaceRouterImage"] = map[string]interface{}{
		"name": "ghcr.io/codesphere-cloud/docker/workspace-router",
		"tag":  "latest",
	}

	// workspacePriorityClass defaults
	g["workspacePriorityClass"] = map[string]interface{}{
		"free": "workspace-free",
		"paid": "workspace-paid",
	}

	// publicIP from gateway
	g["publicIP"] = b.Env.GatewayIP

	// metrics defaults
	g["metrics"] = map[string]interface{}{
		"type": "prometheus",
		"prometheus": map[string]interface{}{
			"jobName":        "codesphere",
			"pushGatewayUrl": "",
		},
	}

	// ipService defaults
	g["ipService"] = map[string]interface{}{
		"loadBalancerKind": "metallb",
		"addressPools":     []string{},
	}

	// publicApi defaults
	g["publicApi"] = map[string]interface{}{
		"rateLimitPerMin": 60,
	}

	// Recaptcha defaults
	g["recaptchaV3Threshold"] = 0.5
	g["showPromotions"] = false
	g["sendGridListId"] = ""
	g["twitterTrackingId"] = ""
	g["googleTrackingId"] = ""
	g["teamCleanupWhitelist"] = []string{}

	// workspace hosting
	g["hosts"] = []interface{}{}
	g["availableDataCenters"] = []interface{}{dc.ID}
	g["namespace"] = "codesphere"
	g["mounterHmacSecret"] = "" // filled by vault if present

	// Cert issuer from config
	g["certIssuer"] = map[string]interface{}{
		"type": cs.CertIssuer.Type,
		"acme": cs.CertIssuer.Acme,
	}
	g["deployCert"] = cfg.Cluster.Certificates.CA

	// Ceph
	ceph := cfg.Ceph
	g["ceph"] = map[string]interface{}{
		"mdsNamespace":              "rook-ceph",
		"credentialsSecretName":     "rook-ceph",
		"activeMds":                 1,
		"monEndpointsConfigMapName": "rook-ceph-mon-endpoints",
		"storageClass":              "rook-cephfs",
		"cephAdmSshKey":             ceph.CephAdmSSHKey,
		"csiKubeletDir":             ceph.CsiKubeletDir,
		"nodesSubnet":               ceph.NodesSubnet,
		"hosts":                     ceph.Hosts,
		"osds":                      ceph.OSDs,
	}

	// Network policies
	g["networkPolicies"] = map[string]interface{}{
		"workspace": map[string]interface{}{
			"namespace":     "workspaces",
			"podSubnet":     "10.244.0.0/16",
			"serviceSubnet": "10.96.0.0/12",
			"cephIps":       []string{},
		},
		"publicGateway":         map[string]interface{}{"namespace": "codesphere", "name": "public-gateway"},
		"workspaceReverseProxy": map[string]interface{}{"namespace": "codesphere", "name": "workspace-reverse-proxy"},
		"gateway":               map[string]interface{}{"namespace": "codesphere", "name": "gateway"},
		"restrictProxyIngress":  false,
	}

	// Frontend gateway
	g["frontendGateway"] = map[string]interface{}{
		"redirectMarketingPages": map[string]interface{}{"enabled": false},
		"redirectToIde":          true,
		"cert":                   map[string]interface{}{"algorithm": "RSA", "size": 2048},
		"config":                 map[string]interface{}{"worker": map[string]interface{}{"worker_processes": "1"}},
		"image":                  map[string]interface{}{"name": "ghcr.io/codesphere-cloud/docker/nginx", "tag": "1.26.3"},
		"replicas":               1,
		"requests":               map[string]interface{}{"cpu": "1000m", "ephemeral-storage": "50M", "memory": "80Mi"},
		"issuer":                 "codesphere-issuer",
	}

	// Branding
	g["branding"] = map[string]interface{}{
		"desc":               "Codesphere - Your zero-config cloud IDE",
		"docsUrl":            "https://codesphere.com/docs/en",
		"pipelineExampleUrl": "https://github.com/codesphere-cloud/nodejs-template/blob/main/ci.yml",
		"faviconHref":        "/ide/assets/favicon-32x32.png",
		"title":              "Codesphere",
		"tosUrl":             "https://codesphere.com/terms",
	}

	// Email
	g["emailConfig"] = map[string]interface{}{
		"noReplyAddress":   "noreply@codesphere.com",
		"supportAddress":   "support@codesphere.com",
		"organizationName": "Codesphere",
		"blogLink":         "https://codesphere.com/blog",
		"feedbackLink":     "https://codesphere.com/feedback",
		"tutorialLink":     "https://codesphere.com/docs",
		"topLogoUrl":       "https://codesphere.com/logo.png",
	}

	// Other hardcoded defaults from the chart
	g["logAsJson"] = true
	g["customDomainIngressClass"] = "nginx"
	g["allowWorkspacesOnControlPlane"] = cfg.Kubernetes.ManagedByCodesphere
	g["imageTag"] = cfg.GeneratedForVersion
	if g["imageTag"] == "" {
		g["imageTag"] = "v1.77.2"
	}
	g["freeWorkspaceTeamLimit"] = 5
	g["freeWorkspaceClusterLimit"] = 3
	g["freeGpuWsTeamLimit"] = 0
	g["gracePeriodDays"] = 21
	g["useDedicatedWorkspaceNodes"] = true
	g["useUsageBasedBillingForNewCustomers"] = false
	g["trustyThresholdCent"] = 1000

	// Services defaults
	g["services"] = map[string]interface{}{
		"priorityClass": "system-cluster-critical",
		"marketplace":   map[string]interface{}{"replicas": 1, "image": "ghcr.io/codesphere-cloud/docker/marketplace"},
	}

	// OTEL defaults
	g["otel"] = map[string]interface{}{
		"config":   map[string]interface{}{},
		"limits":   map[string]interface{}{},
		"requests": map[string]interface{}{},
		"replicas": 1,
	}

	return g
}

// runInstallPhase runs a single install phase on the jumpbox with the given
// skip steps. It builds the full oms install codesphere command for the phase.
func (b *GCPBootstrapper) runInstallPhase(packageFilename, phase string, extraSkips []string) error {
	skipSteps := append([]string{}, extraSkips...)
	if b.Env.RegistryType == RegistryTypeGitHub {
		skipSteps = append(skipSteps, "load-container-images")
	}

	cmd := fmt.Sprintf("oms install codesphere %s -c /etc/codesphere/config.yaml -k %s/age_key.txt --vault %s -p %s",
		phase, b.Env.SecretsDir, filepath.Join(b.Env.SecretsDir, "prod.vault.yaml"), packageFilename)
	if len(skipSteps) > 0 {
		cmd += " -s " + strings.Join(skipSteps, ",")
	}
	return b.Env.Jumpbox.RunSSHCommand("root", cmd)
}

// ensureNewOmsBinaryOnJumpbox copies a freshly-built linux/amd64 OMS binary to
// the jumpbox, replacing the old installed version.
func (b *GCPBootstrapper) ensureNewOmsBinaryOnJumpbox() error {
	b.stlog.Logf("Updating OMS binary on jumpbox for %s compatibility...", b.Env.InstallVersion)

	binaryPath, cleanup, err := b.OmsBinaryBuilder()
	if err != nil {
		return fmt.Errorf("failed to prepare OMS linux binary: %w", err)
	}
	defer cleanup()

	const remoteTmpPath = "/tmp/oms-new"
	if err := b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, binaryPath, remoteTmpPath); err != nil {
		return fmt.Errorf("failed to copy OMS binary to jumpbox: %w", err)
	}

	if err := b.Env.Jumpbox.RunSSHCommand("root", fmt.Sprintf("chmod +x %s && mv %s /usr/local/bin/oms", remoteTmpPath, remoteTmpPath)); err != nil {
		return fmt.Errorf("failed to install OMS binary on jumpbox: %w", err)
	}

	return nil
}

// ensureSSHKeyOnJumpbox copies the user's SSH private key to the jumpbox at
// /root/.ssh/id_rsa and writes an SSH config so that the LTS installer's
// private-cloud-installer.js can SSH to worker nodes via its internal SshClient.
func (b *GCPBootstrapper) ensureSSHKeyOnJumpbox() error {
	b.stlog.Logf("Copying SSH private key to jumpbox for inter-node access...")

	srcPath := b.Env.SSHPrivateKeyPath
	if srcPath == "" {
		return fmt.Errorf("SSH private key path not set (use --ssh-private-key-path)")
	}

	keyBytes, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read SSH private key from %s: %w", srcPath, err)
	}

	// Set up .ssh directory with correct permissions (SSH is strict: 700 for dir).
	setupCmd := "mkdir -p /root/.ssh && chmod 700 /root/.ssh"
	if err := b.Env.Jumpbox.RunSSHCommand("root", setupCmd); err != nil {
		return fmt.Errorf("failed to create .ssh directory on jumpbox: %w", err)
	}

	// Write the key via heredoc.
	writeKeyCmd := fmt.Sprintf("cat > /root/.ssh/id_rsa << 'OMSEOF'\n%s\nOMSEOF\nchmod 600 /root/.ssh/id_rsa", string(keyBytes))
	if err := b.Env.Jumpbox.RunSSHCommand("root", writeKeyCmd); err != nil {
		return fmt.Errorf("failed to write SSH private key on jumpbox: %w", err)
	}

	// Write an SSH config that explicitly uses this identity file so the
	// LTS installer's ssh child process always finds it.
	sshConfig := "Host *\n  IdentityFile /root/.ssh/id_rsa\n  StrictHostKeyChecking no\n  UserKnownHostsFile /dev/null\n"
	writeConfigCmd := fmt.Sprintf("cat > /root/.ssh/config << 'OMSEOF'\n%sOMSEOF\nchmod 600 /root/.ssh/config", sshConfig)
	if err := b.Env.Jumpbox.RunSSHCommand("root", writeConfigCmd); err != nil {
		return fmt.Errorf("failed to write SSH config on jumpbox: %w", err)
	}

	return nil
}

// startLTSCephMasterWatcher starts a background process on the ceph master node that continuously
// re-adds the master to the Ceph orchestrator host inventory. This is required for LTS versions
// because the installer's configureHosts step applies a declarative host spec containing only the
// non-master nodes, which removes the master from the inventory. The watcher restores it within
// seconds, before the subsequent configureMonitors step runs.
func (b *GCPBootstrapper) startLTSCephMasterWatcher() {
	if len(b.Env.CephNodes) == 0 || len(b.Env.InstallConfig.Ceph.Hosts) == 0 {
		return
	}
	masterHost := b.Env.InstallConfig.Ceph.Hosts[0]
	// Use cephadm shell (same as the installer) so the command runs inside the ceph container,
	// bypassing any standalone-binary or keyring availability issues on the host.
	// The FSID is auto-detected from /var/lib/ceph/; all output is logged for diagnostics.
	cmd := fmt.Sprintf(
		`nohup bash -c "while true; do FSID=\$(ls /var/lib/ceph/ 2>/dev/null | head -1); [ -n \"\$FSID\" ] && [ -x /usr/local/bin/cephadm ] && /usr/local/bin/cephadm shell --fsid \"\$FSID\" -- ceph orch host add %s %s 2>&1; sleep 3; done" > /tmp/ceph-host-watcher.log 2>&1 & echo $! > /tmp/ceph-host-watcher.pid`,
		masterHost.Hostname,
		masterHost.IPAddress,
	)
	if err := b.Env.CephNodes[0].RunSSHCommand("root", cmd); err != nil {
		b.stlog.Logf("Note: could not start ceph master host watcher on %s: %v", masterHost.Hostname, err)
	}
}

// stopLTSCephMasterWatcher stops the background watcher started by startLTSCephMasterWatcher.
func (b *GCPBootstrapper) stopLTSCephMasterWatcher() {
	if len(b.Env.CephNodes) == 0 || len(b.Env.InstallConfig.Ceph.Hosts) == 0 {
		return
	}
	cmd := `kill $(cat /tmp/ceph-host-watcher.pid 2>/dev/null) 2>/dev/null; rm -f /tmp/ceph-host-watcher.pid /tmp/ceph-host-watcher.log`
	_ = b.Env.CephNodes[0].RunSSHCommand("root", cmd)
}

func (b *GCPBootstrapper) ensureCodespherePackageOnJumpbox() (string, error) {
	packageFilename := "installer.tar.gz"
	if b.Env.RegistryType == RegistryTypeGitHub {
		packageFilename = "installer-lite.tar.gz"
	}

	if b.Env.InstallLocal != "" {
		b.stlog.Logf("Copying local package %s to jumpbox...", b.Env.InstallLocal)
		fullPackageFilename := fmt.Sprintf("local-%s", packageFilename)
		err := b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, b.Env.InstallLocal, "/root/"+fullPackageFilename)
		if err != nil {
			return "", fmt.Errorf("failed to copy local install package to jumpbox: %w", err)
		}
		return fullPackageFilename, nil
	}

	if b.Env.InstallVersion == "" {
		return "", errors.New("either install version or a local package must be specified to install Codesphere")
	}

	fullPackageFilename := portal.BuildPackageFilenameFromParts(b.Env.InstallVersion, b.Env.InstallHash, packageFilename)
	if b.Env.InstallHash == "" {
		return "", fmt.Errorf("install hash must be set when install version is set")
	}
	b.stlog.Logf("Downloading Codesphere package...")
	downloadCmd := fmt.Sprintf("oms download package -f %s -H %s %s",
		packageFilename, b.Env.InstallHash, b.Env.InstallVersion)
	if err := b.Env.Jumpbox.RunSSHCommand("root", downloadCmd); err != nil {
		return "", fmt.Errorf("failed to download Codesphere package from jumpbox: %w", err)
	}

	return fullPackageFilename, nil
}

func (b *GCPBootstrapper) runInstallCommand(packageFilename string) error {
	b.stlog.Logf("Installing Codesphere...")

	// LTS packages whose bom.json predates the pc-applications component
	// need the ArgoCD+pc-apps pre-step skipped to avoid a missing-BOM error.
	if ltsSpec := FindLTSSpec(b.Env.InstallVersion); ltsSpec != nil {
		if ltsSpec.SkipPcApps {
			b.Env.InstallSkipSteps = append(b.Env.InstallSkipSteps, "argocd")
		}
		if ltsSpec.SkipSetupCluster {
			b.Env.InstallSkipSteps = append(b.Env.InstallSkipSteps, "set-up-cluster", "ms-backends", "codesphere")
		}
	}

	installCmd := fmt.Sprintf("oms install codesphere -c /etc/codesphere/config.yaml -k %s/age_key.txt --vault %s -p %s%s",
		b.Env.SecretsDir, filepath.Join(b.Env.SecretsDir, "prod.vault.yaml"), packageFilename, b.generateSkipStepsArg())
	return b.Env.Jumpbox.RunSSHCommand("root", installCmd)
}

func (b *GCPBootstrapper) generateSkipStepsArg() string {
	skipSteps := b.Env.InstallSkipSteps
	if b.Env.RegistryType == RegistryTypeGitHub {
		skipSteps = append(skipSteps, "load-container-images")
	}
	if len(skipSteps) == 0 {
		return ""
	}

	return " -s " + strings.Join(skipSteps, ",")
}

func (b *GCPBootstrapper) GenerateK0sConfigScript() error {
	script := `#!/bin/bash

cat <<EOF > cloud.conf
[Global]
project-id = "$PROJECT_ID"
EOF

cat <<EOF >> cc-deployment.yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: cloud-controller-manager
  namespace: kube-system
  labels:
    component: cloud-controller-manager
spec:
  selector:
    matchLabels:
      component: cloud-controller-manager
  template:
    metadata:
      labels:
        component: cloud-controller-manager
    spec:
      serviceAccountName: cloud-controller-manager
      containers:
      - name: cloud-controller-manager
        image: k8scloudprovidergcp/cloud-controller-manager:latest
        command:
        - /usr/local/bin/cloud-controller-manager
        args:
        - --v=5
        - --cloud-provider=gce
        - --cloud-config=/etc/gce/cloud.conf
        - --leader-elect-resource-name=k0s-gcp-ccm
        - --use-service-account-credentials=true
        - --controllers=cloud-node,cloud-node-lifecycle,service
        - --allocate-node-cidrs=false
        - --configure-cloud-routes=false
        volumeMounts:
        - name: cloud-config-volume
          mountPath: /etc/gce
          readOnly: true
      volumes:
      - name: cloud-config-volume
        configMap:
          name: cloud-config
      tolerations:
      - key: node.cloudprovider.kubernetes.io/uninitialized
        value: "true"
        effect: NoSchedule
      - key: node-role.kubernetes.io/master
        effect: NoSchedule
      - key: node-role.kubernetes.io/control-plane
        effect: NoSchedule
EOF

KUBECTL="/etc/codesphere/deps/kubernetes/files/k0s kubectl"
$KUBECTL create configmap cloud-config --from-file=cloud.conf -n kube-system
echo alias kubectl=\"$KUBECTL\" >> /root/.bashrc
echo alias k=\"$KUBECTL\" >> /root/.bashrc

$KUBECTL apply -f https://raw.githubusercontent.com/kubernetes/cloud-provider-gcp/refs/tags/providers/v0.28.2/deploy/packages/default/manifest.yaml

$KUBECTL apply -f cc-deployment.yaml

# set loadBalancerIP for public-gateway-controller and gateway-controller
$KUBECTL patch svc public-gateway-controller -n codesphere -p '{"spec": {"loadBalancerIP": "'` + b.Env.PublicGatewayIP + `'"}}'
$KUBECTL patch svc gateway-controller -n codesphere -p '{"spec": {"loadBalancerIP": "'` + b.Env.GatewayIP + `'"}}'

sed -i 's/k0scontroller/k0scontroller --enable-cloud-provider/g' /etc/systemd/system/k0scontroller.service

ssh -o StrictHostKeyChecking=no root@` + b.Env.ControlPlaneNodes[1].GetInternalIP() + ` "sed -i 's/k0sworker/k0sworker --enable-cloud-provider/g' /etc/systemd/system/k0sworker.service; systemctl daemon-reload; systemctl restart k0sworker"

ssh -o StrictHostKeyChecking=no root@` + b.Env.ControlPlaneNodes[2].GetInternalIP() + ` "sed -i 's/k0sworker/k0sworker --enable-cloud-provider/g' /etc/systemd/system/k0sworker.service; systemctl daemon-reload; systemctl restart k0sworker"

systemctl daemon-reload
systemctl restart k0scontroller
`
	// Probably we need to enable the cloud provider plugin in k0s configuration.
	// --enable-cloud-provider on worker nodes systemd file /etc/systemd/system/k0sworker.service
	// in addition on the first node: /etc/systemd/system/k0scontroller.service the flag --enable-cloud-provider

	err := b.fw.WriteFile("configure-k0s.sh", []byte(script), 0755)
	if err != nil {
		return fmt.Errorf("failed to write configure-k0s.sh: %w", err)
	}
	err = b.Env.ControlPlaneNodes[0].NodeClient.CopyFile(b.Env.ControlPlaneNodes[0], "configure-k0s.sh", "/root/configure-k0s.sh")
	if err != nil {
		return fmt.Errorf("failed to copy configure-k0s.sh to control plane node: %w", err)
	}
	err = b.Env.ControlPlaneNodes[0].RunSSHCommand("root", "chmod +x /root/configure-k0s.sh")
	if err != nil {
		return fmt.Errorf("failed to make configure-k0s.sh executable on control plane node: %w", err)
	}
	return nil
}

func (b *GCPBootstrapper) RunK0sConfigScript() error {
	err := b.Env.ControlPlaneNodes[0].RunSSHCommand("root", "/root/configure-k0s.sh")
	if err != nil {
		return fmt.Errorf("failed to install Codesphere from jumpbox: %w", err)
	}

	return nil
}
