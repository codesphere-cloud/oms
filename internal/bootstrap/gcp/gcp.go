// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/codesphere-cloud/oms/internal/portal"
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
	"managed-services",
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
	NodeClient   node.NodeClient
	PortalClient portal.Portal
	GitHubClient github.GitHubClient
}

type CodesphereEnvironment struct {
	ProjectID            string       `json:"project_id"`
	ProjectTTL           string       `json:"project_ttl"`
	ProjectName          string       `json:"project_name"`
	DNSProjectID         string       `json:"dns_project_id"`
	Jumpbox              *node.Node   `json:"jumpbox"`
	PostgreSQLNode       *node.Node   `json:"postgres_node"`
	ControlPlaneNodes    []*node.Node `json:"control_plane_nodes"`
	CephNodes            []*node.Node `json:"ceph_nodes"`
	ContainerRegistryURL string       `json:"-"`
	ExistingConfigUsed   bool         `json:"-"`
	InstallVersion       string       `json:"install_version"`
	InstallLocal         string       `json:"install_local"`
	InstallHash          string       `json:"install_hash"`
	InstallSkipSteps     []string     `json:"install_skip_steps"`
	Preemptible          bool         `json:"preemptible"`
	SpotVMs              bool         `json:"spot_vms"`
	WriteConfig          bool         `json:"-"`
	GatewayIP            string       `json:"gateway_ip"`
	PublicGatewayIP      string       `json:"public_gateway_ip"`
	RegistryType         RegistryType `json:"registry_type"`
	GitHubPAT            string       `json:"-"`
	GitHubAppName        string       `json:"-"`
	GitHubTeamOrg        string       `json:"github_team_org"`
	GitHubTeamSlug       string       `json:"github_team_slug"`
	RegistryUser         string       `json:"-"`
	Experiments          []string     `json:"experiments"`
	FeatureFlags         []string     `json:"feature_flags"`

	// OpenBao
	OpenBaoURI      string `json:"-"`
	OpenBaoEngine   string `json:"-"`
	OpenBaoUser     string `json:"-"`
	OpenBaoPassword string `json:"-"`

	// Config
	InstallConfigPath string              `json:"-"`
	SecretsFilePath   string              `json:"-"`
	InstallConfig     *files.RootConfig   `json:"-"`
	Secrets           *files.InstallVault `json:"-"`

	// GCP Specific
	ProjectDisplayName    string `json:"project_display_name"`
	BillingAccount        string `json:"billing_account"`
	BaseDomain            string `json:"base_domain"`
	GitHubAppClientID     string `json:"-"`
	GitHubAppClientSecret string `json:"-"`
	SecretsDir            string `json:"secrets_dir"`
	FolderID              string `json:"folder_id"`
	SSHPublicKeyPath      string `json:"-"`
	SSHPrivateKeyPath     string `json:"-"`
	DatacenterID          int    `json:"-"`
	CustomPgIP            string `json:"custom_pg_ip"`
	Region                string `json:"region"`
	Zone                  string `json:"zone"`
	DNSZoneName           string `json:"dns_zone_name"`
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
		ctx:          ctx,
		stlog:        stlog,
		fw:           fw,
		icg:          icg,
		GCPClient:    gcpClient,
		Env:          CodesphereEnv,
		NodeClient:   sshRunner,
		PortalClient: portalClient,
		Time:         time,
		GitHubClient: gitHubClient,
	}, nil
}

func GetInfraFilePath() string {
	workdir := env.NewEnv().GetOmsWorkdir()
	return fmt.Sprintf("%s/gcp-infra.json", workdir)
}

// LoadInfraFile reads and parses the GCP infrastructure file from the specified path.
// Returns the environment, whether the file exists, and any error.
// If the file doesn't exist, returns an empty environment with exists=false and nil error.
func LoadInfraFile(fw util.FileIO, infraFilePath string) (CodesphereEnvironment, bool, error) {
	if !fw.Exists(infraFilePath) {
		return CodesphereEnvironment{}, false, nil
	}

	content, err := fw.ReadFile(infraFilePath)
	if err != nil {
		return CodesphereEnvironment{}, true, fmt.Errorf("failed to read gcp infra file: %w", err)
	}

	var env CodesphereEnvironment
	if err := json.Unmarshal(content, &env); err != nil {
		return CodesphereEnvironment{}, true, fmt.Errorf("failed to unmarshal gcp infra file: %w", err)
	}
	return env, true, nil
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

	return nil
}

// ValidateInput checks that the required input parameters are set and valid
func (b *GCPBootstrapper) ValidateInput() error {
	err := b.validateInstallVersion()
	if err != nil {
		return err
	}

	err = b.validateVMProvisioningOptions()
	if err != nil {
		return err
	}

	return b.validateGitHubParams()
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
	if err == nil && b.Env.InstallConfig.Registry != nil && b.Env.InstallConfig.Registry.Server == localRegistryServer &&
		b.Env.InstallConfig.Registry.Username != "" && b.Env.InstallConfig.Registry.Password != "" {
		b.stlog.Logf("Local container registry already running on postgres node")
		return nil
	}

	b.Env.InstallConfig.Registry.Server = localRegistryServer
	b.Env.InstallConfig.Registry.Username = "custom-registry"
	b.Env.InstallConfig.Registry.Password = shortuuid.New()

	commands := []string{
		"apt-get update",
		"apt-get install -y podman apache2-utils",
		"htpasswd -bBc /root/registry.password " + b.Env.InstallConfig.Registry.Username + " " + b.Env.InstallConfig.Registry.Password,
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
	b.Env.InstallConfig.Registry.Username = b.Env.RegistryUser
	b.Env.InstallConfig.Registry.Password = b.Env.GitHubPAT
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

	err = b.runInstallCommand(fullPackageFilename)
	if err != nil {
		return fmt.Errorf("failed to install Codesphere from jumpbox: %w", err)
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
	downloadCmd := fmt.Sprintf("oms download package -f %s -H %s %s", packageFilename, b.Env.InstallHash, b.Env.InstallVersion)
	err := b.Env.Jumpbox.RunSSHCommand("root", downloadCmd)
	if err != nil {
		return "", fmt.Errorf("failed to download Codesphere package from jumpbox: %w", err)
	}

	return fullPackageFilename, nil
}

func (b *GCPBootstrapper) runInstallCommand(packageFilename string) error {
	b.stlog.Logf("Installing Codesphere...")
	installCmd := fmt.Sprintf("oms install codesphere -c /etc/codesphere/config.yaml -k %s/age_key.txt -p %s%s",
		b.Env.SecretsDir, packageFilename, b.generateSkipStepsArg())
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
