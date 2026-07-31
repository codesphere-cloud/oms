// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/datacenter"
	"github.com/codesphere-cloud/oms/internal/clusteradmin"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/github"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/testuser"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/dns/v1"
)

type RegistryType string

const (
	RegistryTypeLocalContainer   RegistryType = "local-container"
	RegistryTypeArtifactRegistry RegistryType = "artifact-registry"
	RegistryTypeGitHub           RegistryType = "github"
)

// remoteK0sConfigScriptPath is where each data center's k0s configuration script is placed on
// that data center's first control plane node. Data centers have separate nodes, so the path can
// be the same for all of them.
const remoteK0sConfigScriptPath = "/root/configure-k0s.sh"

// installerNodeSecretsDir is where the Codesphere installer uploads a data center's age key on
// every one of that data center's nodes. The path is fixed even though the installer reads the key
// from the data center's own secrets.baseDir on the jumpbox, and the installer only creates
// baseDir on the node — so for a data center whose baseDir differs, the upload target would not
// exist. Creating it up front is harmless: data centers have separate nodes, so a node only ever
// holds its own data center's key.
const installerNodeSecretsDir = "/etc/codesphere/secrets"

// vpcSubnetCIDR is the range of the project's single subnet, shared by all data centers. It is
// also each data center's ceph.nodesSubnet; their Ceph clusters stay separate because each has
// its own hosts, monitors and FSID.
const vpcSubnetCIDR = "10.10.0.0/20"

// CheckOMSManagedLabel checks if the given labels map indicates an OMS-managed project.
// A project is considered OMS-managed if it has the 'oms-managed' label set to "true".
func CheckOMSManagedLabel(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	value, exists := labels[OMSManagedLabel]
	return exists && value == "true"
}

// DNSRecordName identifies a DNS record set that OMS manages.
type DNSRecordName struct {
	Name  string `json:"name"`
	Rtype string `json:"rtype"`
}

// GetDNSRecordNames returns the DNS record names a single-data-center bootstrap creates for a
// given base domain. It is the fallback for infra files written before multi-DC support, which
// do not record the created records.
func GetDNSRecordNames(baseDomain string) []DNSRecordName {
	return []DNSRecordName{
		{fmt.Sprintf("cs.%s.", baseDomain), "A"},
		{fmt.Sprintf("*.cs.%s.", baseDomain), "A"},
		{fmt.Sprintf("ws.%s.", baseDomain), "A"},
		{fmt.Sprintf("*.ws.%s.", baseDomain), "A"},
		{fmt.Sprintf("*.ssh.cs.%s.", baseDomain), "A"},
	}
}

// DataCenterDNSRecordNames returns every DNS record OMS creates for the given data center
// layout: the shared platform gateway names plus each data center's workspace and SSH names.
func DataCenterDNSRecordNames(baseDomain string, dcs []*datacenter.DataCenter) []DNSRecordName {
	records := []DNSRecordName{
		{fmt.Sprintf("cs.%s.", baseDomain), "A"},
		{fmt.Sprintf("*.cs.%s.", baseDomain), "A"},
	}
	for _, dc := range dcs {
		records = append(records,
			DNSRecordName{fmt.Sprintf("%s.", dc.WorkspaceHostingBaseDomain), "A"},
			DNSRecordName{fmt.Sprintf("*.%s.", dc.WorkspaceHostingBaseDomain), "A"},
			DNSRecordName{fmt.Sprintf("*.%s.", dc.SshBaseDomain), "A"},
		)
		if len(dcs) > 1 {
			records = append(records,
				DNSRecordName{fmt.Sprintf("%s.", dc.PlatformDomain(baseDomain)), "A"},
				DNSRecordName{fmt.Sprintf("*.%s.", dc.PlatformDomain(baseDomain)), "A"},
			)
		}
	}

	return records
}

// This should ALWAYS be empty. Internal flags are for internal feature
// development and not intended for customer use.
// Atm. it's not empty as the internal flags below are likely preview or
// feature flags, but are still in the internal bucket for historical
// reasons (before we only had one "experiments" bucket).
var DefaultInternalFlags []string = []string{
	"headless-services",
	"vcluster",
	"custom-service-image",
	"ms-in-ls",
}

var DefaultPreviewFlags []string = []string{
	"secret-management",
	"sub-path-mount",
	"workspace-ssh",
}

var DefaultFeatureFlags []string = []string{}

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
	NodeClient   node.NodeClient
	PortalClient portal.Portal
	GitHubClient github.GitHubClient
	// NewConfigManager creates the install config manager of a data center. Each data center
	// owns its own config and vault, so multi-DC bootstraps need more than one.
	NewConfigManager func() installer.InstallConfigManager
}

// primaryDC returns the first data center, which owns the shared PostgreSQL server and the
// platform gateway that codesphere.domain resolves to.
func (b *GCPBootstrapper) primaryDC() *datacenter.DataCenter {
	return b.Env.DataCenters[0]
}

// allNodes returns every node of the project: the jumpbox, the shared postgres node and all
// data centers' Ceph and k0s nodes.
func (b *GCPBootstrapper) allNodes() []*node.Node {
	nodes := []*node.Node{b.Env.Jumpbox, b.Env.PostgreSQLNode}
	for _, dc := range b.Env.DataCenters {
		nodes = append(nodes, dc.ControlPlaneNodes...)
		nodes = append(nodes, dc.CephNodes...)
	}

	return nodes
}

// clusterNodes returns every Ceph and k0s node of all data centers, i.e. all nodes except the
// jumpbox and the shared postgres node.
func (b *GCPBootstrapper) clusterNodes() []*node.Node {
	nodes := []*node.Node{}
	for _, dc := range b.Env.DataCenters {
		nodes = append(nodes, dc.ControlPlaneNodes...)
		nodes = append(nodes, dc.CephNodes...)
	}

	return nodes
}

type CodesphereEnvironment struct {
	ProjectID      string     `json:"project_id"`
	ProjectTTL     string     `json:"project_ttl"`
	ProjectName    string     `json:"project_name"`
	DNSProjectID   string     `json:"dns_project_id"`
	Jumpbox        *node.Node `json:"jumpbox"`
	PostgreSQLNode *node.Node `json:"postgres_node"`
	// MultiDC bootstraps two data centers that share the PostgreSQL server but run separate
	// Kubernetes and Ceph clusters.
	MultiDC bool `json:"multi_dc"`
	// DataCenters holds the per-data-center state. It always has at least one entry.
	DataCenters []*datacenter.DataCenter `json:"datacenters"`
	// DNSRecords records the DNS records the bootstrap created, so cleanup deletes exactly
	// those instead of recomputing the list.
	DNSRecords []DNSRecordName `json:"dns_records,omitempty"`
	// ControlPlaneNodes and CephNodes are where the primary data center's nodes lived before
	// multi-DC support. The steps that have not been migrated to DataCenters yet still use
	// them, and infra files written by an earlier OMS carry the nodes here.
	ControlPlaneNodes []*node.Node `json:"control_plane_nodes"`
	CephNodes         []*node.Node `json:"ceph_nodes"`
	// ContainerRegistryURL is the resolved registry server all data centers pull images from.
	ContainerRegistryURL          string       `json:"container_registry_url,omitempty"`
	RegistryUsername              string       `json:"-"`
	RegistryPassword              string       `json:"-"`
	ExistingConfigUsed            bool         `json:"-"`
	InstallVersion                string       `json:"install_version"`
	InstallLocal                  string       `json:"install_local"`
	InstallHash                   string       `json:"install_hash"`
	InstallSkipSteps              []string     `json:"install_skip_steps"`
	Preemptible                   bool         `json:"preemptible"`
	SpotVMs                       bool         `json:"spot_vms"`
	WriteConfig                   bool         `json:"-"`
	RecoverConfig                 bool         `json:"-"`
	GatewayIP                     string       `json:"gateway_ip"`
	PublicGatewayIP               string       `json:"public_gateway_ip"`
	SshProxyIP                    string       `json:"ssh_proxy_ip"`
	RegistryType                  RegistryType `json:"registry_type"`
	GitHubPAT                     string       `json:"-"`
	GitHubAppName                 string       `json:"-"`
	GitHubTeamOrg                 string       `json:"github_team_org"`
	GitHubTeamSlug                string       `json:"github_team_slug"`
	RegistryUser                  string       `json:"-"`
	InternalFlags                 []string     `json:"internal"`
	PreviewFlags                  []string     `json:"preview"`
	FeatureFlags                  []string     `json:"feature_flags"`
	ExternalLokiEndpoint          string       `json:"external_loki_endpoint,omitempty"`
	ExternalLokiSecret            string       `json:"-"`
	ExternalLokiUser              string       `json:"external_loki_user,omitempty"`
	PrometheusRemoteWriteUser     string       `json:"prometheus_remote_write_user,omitempty"`
	PrometheusRemoteWritePassword string       `json:"-"`
	PrometheusRemoteWriteURL      string       `json:"prometheus_remote_write_url,omitempty"`
	ClusterAdminEmail             string       `json:"cluster_admin_email,omitempty"`

	// ACME Issuer
	GoogleACMEIssuer bool `json:"google_acme_issuer,omitempty"`
	ACMEStaging      bool `json:"acme_staging,omitempty"`

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
	// DatacenterIDExplicit records whether --datacenter-id was set on the command line. The
	// value alone cannot distinguish the default 1 from an explicit 1.
	DatacenterIDExplicit bool   `json:"-"`
	CustomPgIP           string `json:"custom_pg_ip"`
	Region               string `json:"region"`
	Zone                 string `json:"zone"`
	DNSZoneName          string `json:"dns_zone_name"`

	// Test user creation
	CreateTestUser bool   `json:"-"`
	OmsWorkdir     string `json:"-"`
	RootDiskSize   int64  `json:"root_disk_size"`
	// Local OMS binary copied to the jumpbox instead of installing a release.
	RemoteOmsBinaryPath string `json:"-"`
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
		NewConfigManager: installer.NewInstallConfigManager,
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
		err = b.InstallCodesphere()
		if err != nil {
			return fmt.Errorf("failed to install Codesphere: %w", err)
		}

		// Every data center is installed before any k0s script runs, so a script never patches
		// gateway services an install has yet to create.
		err = b.RunK0sConfigScript()
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

// createTestUser creates a test user in the shared PostgreSQL instance using the testuser package
// and logs the credentials. The user's team is homed in the primary data center.
func (b *GCPBootstrapper) createTestUser() error {
	b.ensureDataCenters()

	if b.Env.PostgreSQLNode == nil {
		return fmt.Errorf("postgres node not found in bootstrap environment")
	}

	pgHost := b.Env.PostgreSQLNode.GetExternalIP()
	if pgHost == "" {
		return fmt.Errorf("postgres node has no external IP")
	}

	primary := b.primaryDC()
	if primary.InstallConfig == nil {
		return fmt.Errorf("install config not found in bootstrap environment")
	}
	pgPasswordSecret := primary.ConfigManager.GetVault().GetSecret(files.SecretPostgresPassword)
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
		DatacenterID: primary.ID,
	})
	if err != nil {
		return err
	}

	testuser.LogAndPersistResult(result, b.Env.OmsWorkdir)
	return nil
}
func (b *GCPBootstrapper) ValidateInput() error {
	if b.Env.GoogleACMEIssuer && b.Env.ACMEStaging {
		return fmt.Errorf("acme-staging cannot be combined with google-acme-issuer")
	}

	if b.Env.RemoteOmsBinaryPath != "" && !b.fw.Exists(b.Env.RemoteOmsBinaryPath) {
		return fmt.Errorf("remote OMS binary not found at path: %s", b.Env.RemoteOmsBinaryPath)
	}

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

	err = b.validateClusterAdminEmail()
	if err != nil {
		return err
	}

	return b.validateTelemetryExportParams()
}

func (b *GCPBootstrapper) validateClusterAdminEmail() error {
	if b.Env.ClusterAdminEmail == "" {
		return nil
	}

	// The email reaches the cluster via the install config, which is only
	// updated when the config is written.
	if !b.Env.WriteConfig {
		return fmt.Errorf("cluster admin email requires write-config to be enabled")
	}

	email, err := clusteradmin.NormalizeEmail(b.Env.ClusterAdminEmail)
	if err != nil {
		return fmt.Errorf("invalid cluster admin email: %w", err)
	}
	b.Env.ClusterAdminEmail = email

	return nil
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
		SourceRanges: []string{vpcSubnetCIDR},
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

// EnsureGatewayIPAddresses reserves the static external IP addresses of every data center: the
// ingress controllers of its cluster (gateway and public gateway) and its SSH workspace proxy.
func (b *GCPBootstrapper) EnsureGatewayIPAddresses() error {
	b.ensureDataCenters()

	for _, dc := range b.Env.DataCenters {
		if err := b.ensureGatewayIPAddresses(dc); err != nil {
			return err
		}
	}
	b.mirrorPrimaryDataCenter()

	return nil
}

// ensureGatewayIPAddresses reserves one data center's static external IP addresses. Their names
// carry the data-center suffix, so the primary data center keeps the unsuffixed names.
func (b *GCPBootstrapper) ensureGatewayIPAddresses(dc *datacenter.DataCenter) error {
	var err error
	dc.GatewayIP, err = b.EnsureExternalIP("gateway" + dc.Suffix)
	if err != nil {
		return fmt.Errorf("failed to ensure gateway IP: %w", err)
	}
	dc.PublicGatewayIP, err = b.EnsureExternalIP("public-gateway" + dc.Suffix)
	if err != nil {
		return fmt.Errorf("failed to ensure public gateway IP: %w", err)
	}
	dc.SshProxyIP, err = b.EnsureExternalIP("ssh-proxy" + dc.Suffix)
	if err != nil {
		return fmt.Errorf("failed to ensure ssh proxy IP: %w", err)
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
	b.ensureDataCenters()

	for _, node := range b.allNodes() {
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

	err := b.EnsureOmsInstalled()
	if err != nil {
		return fmt.Errorf("failed to ensure OMS is present on jumpbox: %w", err)
	}

	err = b.Env.Jumpbox.EnsureOmsDependencies()
	if err != nil {
		return fmt.Errorf("failed to ensure OMS dependencies on jumpbox: %w", err)
	}

	return nil
}

func (b *GCPBootstrapper) EnsureOmsInstalled() (err error) {
	if b.Env.RemoteOmsBinaryPath != "" {
		err = b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, b.Env.RemoteOmsBinaryPath, "/usr/local/bin/oms")
		if err != nil {
			return fmt.Errorf("failed to copy local OMS binary to jumpbox: %w", err)
		}

		err = b.Env.Jumpbox.RunSSHCommand("root", "chmod 0755 /usr/local/bin/oms")
		if err != nil {
			return fmt.Errorf("failed to make local OMS binary executable on jumpbox: %w", err)
		}
		return nil
	}

	if b.Env.Jumpbox.HasCommand("oms") {
		return nil
	}

	err = b.Env.Jumpbox.InstallOms()
	if err != nil {
		return fmt.Errorf("failed to install OMS on jumpbox: %w", err)
	}

	return nil
}

func (b *GCPBootstrapper) EnsureHostsConfigured() error {
	b.ensureDataCenters()

	allNodes := append([]*node.Node{b.Env.PostgreSQLNode}, b.clusterNodes()...)

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
		err := node.RunSSHCommand("root", "mkdir -p "+installerNodeSecretsDir)
		if err != nil {
			return fmt.Errorf("failed to create secrets directory on %s: %w", node.GetName(), err)
		}
	}

	// A secondary data center's secrets directory differs from the fixed path above, so create that
	// one too on its own nodes. Nodes belong to exactly one data center, so no node gets a foreign
	// data center's directory.
	for _, dc := range b.Env.DataCenters {
		if dc.SecretsDir == installerNodeSecretsDir {
			continue
		}
		for _, n := range append(append([]*node.Node{}, dc.ControlPlaneNodes...), dc.CephNodes...) {
			err := n.RunSSHCommand("root", "mkdir -p "+dc.SecretsDir)
			if err != nil {
				return fmt.Errorf("failed to create secrets directory on %s: %w", n.GetName(), err)
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
	b.ensureDataCenters()

	gcpProject := b.Env.DNSProjectID
	if b.Env.DNSProjectID == "" {
		gcpProject = b.Env.ProjectID
	}

	zoneName := b.Env.DNSZoneName
	err := b.GCPClient.EnsureDNSManagedZone(gcpProject, zoneName, b.Env.BaseDomain+".", "Codesphere DNS zone")
	if err != nil {
		return fmt.Errorf("failed to ensure DNS managed zone: %w", err)
	}

	// The platform is served from one domain shared by all data centers, pointing at the
	// primary data center's gateway.
	records := []*dns.ResourceRecordSet{
		dnsARecord(fmt.Sprintf("cs.%s.", b.Env.BaseDomain), b.primaryDC().GatewayIP),
		dnsARecord(fmt.Sprintf("*.cs.%s.", b.Env.BaseDomain), b.primaryDC().GatewayIP),
	}
	// Workspaces and their SSH endpoints resolve per data center, so each one gets its own
	// names pointing at its own public gateway and SSH proxy.
	for _, dc := range b.Env.DataCenters {
		records = append(records,
			dnsARecord(fmt.Sprintf("%s.", dc.WorkspaceHostingBaseDomain), dc.PublicGatewayIP),
			dnsARecord(fmt.Sprintf("*.%s.", dc.WorkspaceHostingBaseDomain), dc.PublicGatewayIP),
			dnsARecord(fmt.Sprintf("*.%s.", dc.SshBaseDomain), dc.SshProxyIP),
		)
		// The platform calls each data center's own services at <dc-id>.cs.<base-domain>, which
		// the wildcard above would send to the primary data center's gateway. A single data
		// center is that primary, so it needs no record of its own.
		if len(b.Env.DataCenters) > 1 {
			platformDomain := dc.PlatformDomain(b.Env.BaseDomain)
			records = append(records,
				dnsARecord(fmt.Sprintf("%s.", platformDomain), dc.GatewayIP),
				dnsARecord(fmt.Sprintf("*.%s.", platformDomain), dc.GatewayIP),
			)
		}
	}

	err = b.GCPClient.EnsureDNSRecordSets(gcpProject, zoneName, records)
	if err != nil {
		return fmt.Errorf("failed to ensure DNS record sets: %w", err)
	}

	// Record what was created so cleanup deletes exactly these records instead of recomputing
	// the list from the base domain.
	b.Env.DNSRecords = DataCenterDNSRecordNames(b.Env.BaseDomain, b.Env.DataCenters)

	return nil
}

// dnsARecord builds a short-TTL A record set, as used during initial setup.
func dnsARecord(name, ip string) *dns.ResourceRecordSet {
	return &dns.ResourceRecordSet{
		Name:    name,
		Type:    "A",
		Ttl:     300,
		Rrdatas: []string{ip},
	}
}

// InstallCodesphere installs Codesphere into every data center from the shared jumpbox, in
// ascending data center order. The order matters: the primary data center's install creates the
// database, roles and schema that the secondary ones reuse.
func (b *GCPBootstrapper) InstallCodesphere() error {
	b.ensureDataCenters()

	fullPackageFilename, err := b.ensureCodespherePackageOnJumpbox()
	if err != nil {
		return fmt.Errorf("failed to ensure Codesphere package on jumpbox: %w", err)
	}

	for _, dc := range b.Env.DataCenters {
		err = b.stlog.Step(dc.StepName("Install Codesphere"), func() error {
			return b.runInstallCommand(dc, fullPackageFilename)
		})
		if err != nil {
			return fmt.Errorf("failed to install Codesphere from jumpbox (data center %d): %w", dc.ID, err)
		}
	}

	return nil
}

// RunK0sConfigScript runs every data center's k0s configuration script on its first control
// plane node. It requires that data center's Codesphere install to have completed, since the
// script patches the gateway services the install creates.
func (b *GCPBootstrapper) RunK0sConfigScript() error {
	b.ensureDataCenters()

	for _, dc := range b.Env.DataCenters {
		err := b.stlog.Step(dc.StepName("Run k0s config script"), func() error {
			return b.runK0sConfigScript(dc)
		})
		if err != nil {
			return err
		}
	}

	return nil
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
	err := b.Env.Jumpbox.RunSSHCommand("root", downloadCmd)
	if err != nil {
		return "", fmt.Errorf("failed to download Codesphere package from jumpbox: %w", err)
	}

	return fullPackageFilename, nil
}

func (b *GCPBootstrapper) runInstallCommand(dc *datacenter.DataCenter, packageFilename string) error {
	b.stlog.Logf("Installing Codesphere in data center %d...", dc.ID)
	return b.Env.Jumpbox.RunSSHCommand("root", b.InstallCommand(dc, packageFilename))
}

// InstallCommand returns the command that installs Codesphere into the given data center from
// the jumpbox. It is also printed for the operator when the bootstrap does not install itself.
func (b *GCPBootstrapper) InstallCommand(dc *datacenter.DataCenter, packageFilename string) string {
	return fmt.Sprintf("oms install codesphere -c %s -k %s --vault %s -p %s%s",
		dc.RemoteConfigPath, dc.RemoteAgeKeyPath(), dc.RemoteVaultPath(), packageFilename, b.generateSkipStepsArg())
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

// GenerateK0sConfigScript writes and uploads the k0s cloud-provider configuration script of
// every data center to that data center's first control plane node.
func (b *GCPBootstrapper) GenerateK0sConfigScript() error {
	b.ensureDataCenters()

	for _, dc := range b.Env.DataCenters {
		if err := b.generateK0sConfigScript(dc); err != nil {
			return err
		}
	}

	return nil
}

func (b *GCPBootstrapper) generateK0sConfigScript(dc *datacenter.DataCenter) error {
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
$KUBECTL patch svc public-gateway-controller -n codesphere -p '{"spec": {"loadBalancerIP": "'` + dc.PublicGatewayIP + `'"}}'
$KUBECTL patch svc gateway-controller -n codesphere -p '{"spec": {"loadBalancerIP": "'` + dc.GatewayIP + `'"}}'

sed -i 's/k0scontroller/k0scontroller --enable-cloud-provider/g' /etc/systemd/system/k0scontroller.service

ssh -o StrictHostKeyChecking=no root@` + dc.ControlPlaneNodes[1].GetInternalIP() + ` "sed -i 's/k0sworker/k0sworker --enable-cloud-provider/g' /etc/systemd/system/k0sworker.service; systemctl daemon-reload; systemctl restart k0sworker"

ssh -o StrictHostKeyChecking=no root@` + dc.ControlPlaneNodes[2].GetInternalIP() + ` "sed -i 's/k0sworker/k0sworker --enable-cloud-provider/g' /etc/systemd/system/k0sworker.service; systemctl daemon-reload; systemctl restart k0sworker"

systemctl daemon-reload
systemctl restart k0scontroller
`
	// Probably we need to enable the cloud provider plugin in k0s configuration.
	// --enable-cloud-provider on worker nodes systemd file /etc/systemd/system/k0sworker.service
	// in addition on the first node: /etc/systemd/system/k0scontroller.service the flag --enable-cloud-provider

	localScript := dc.K0sConfigScriptPath()
	err := b.fw.WriteFile(localScript, []byte(script), 0755)
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", localScript, err)
	}
	controller := dc.ControlPlaneNodes[0]
	err = controller.NodeClient.CopyFile(controller, localScript, remoteK0sConfigScriptPath)
	if err != nil {
		return fmt.Errorf("failed to copy %s to control plane node: %w", localScript, err)
	}
	err = controller.RunSSHCommand("root", "chmod +x "+remoteK0sConfigScriptPath)
	if err != nil {
		return fmt.Errorf("failed to make %s executable: %w", localScript, err)
	}
	return nil
}

func (b *GCPBootstrapper) runK0sConfigScript(dc *datacenter.DataCenter) error {
	err := dc.ControlPlaneNodes[0].RunSSHCommand("root", remoteK0sConfigScriptPath)
	if err != nil {
		return fmt.Errorf("failed to configure k0s in data center %d: %w", dc.ID, err)
	}

	return nil
}
