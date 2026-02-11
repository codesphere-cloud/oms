// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/dns/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RegistryType string

const (
	RegistryTypeLocalContainer   RegistryType = "local-container"
	RegistryTypeArtifactRegistry RegistryType = "artifact-registry"
	RegistryTypeGitHub           RegistryType = "github"
)

type VMDef struct {
	Name            string
	MachineType     string
	Tags            []string
	AdditionalDisks []int64
	ExternalIP      bool
}

// Example VM definitions (expand as needed)
var vmDefs = []VMDef{
	{"jumpbox", "e2-medium", []string{"jumpbox", "ssh"}, []int64{}, true},
	{"postgres", "e2-standard-8", []string{"postgres"}, []int64{}, true},
	{"ceph-1", "e2-standard-8", []string{"ceph"}, []int64{20, 200}, false},
	{"ceph-2", "e2-standard-8", []string{"ceph"}, []int64{20, 200}, false},
	{"ceph-3", "e2-standard-8", []string{"ceph"}, []int64{20, 200}, false},
	{"ceph-4", "e2-standard-8", []string{"ceph"}, []int64{20, 200}, false},
	{"k0s-1", "e2-standard-16", []string{"k0s"}, []int64{}, false},
	{"k0s-2", "e2-standard-16", []string{"k0s"}, []int64{}, false},
	{"k0s-3", "e2-standard-16", []string{"k0s"}, []int64{}, false},
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
	GCPClient GCPClientManager
	// Environment
	Env *CodesphereEnvironment
	// SSH command runner
	NodeClient   node.NodeClient
	PortalClient portal.Portal
}

type CodesphereEnvironment struct {
	ProjectID                string       `json:"project_id"`
	ProjectName              string       `json:"project_name"`
	DNSProjectID             string       `json:"dns_project_id"`
	DNSProjectServiceAccount string       `json:"dns_project_service_account"`
	Jumpbox                  *node.Node   `json:"jumpbox"`
	PostgreSQLNode           *node.Node   `json:"postgres_node"`
	ControlPlaneNodes        []*node.Node `json:"control_plane_nodes"`
	CephNodes                []*node.Node `json:"ceph_nodes"`
	ContainerRegistryURL     string       `json:"-"`
	ExistingConfigUsed       bool         `json:"-"`
	InstallVersion           string       `json:"install_version"`
	InstallHash              string       `json:"install_hash"`
	InstallSkipSteps         []string     `json:"install_skip_steps"`
	Preemptible              bool         `json:"preemptible"`
	WriteConfig              bool         `json:"-"`
	GatewayIP                string       `json:"gateway_ip"`
	PublicGatewayIP          string       `json:"public_gateway_ip"`
	RegistryType             RegistryType `json:"registry_type"`
	GitHubPAT                string       `json:"-"`
	RegistryUser             string       `json:"-"`
	Experiments              []string     `json:"experiments"`
	FeatureFlags             []string     `json:"feature_flags"`

	// Config
	InstallConfigPath string              `json:"-"`
	SecretsFilePath   string              `json:"-"`
	InstallConfig     *files.RootConfig   `json:"-"`
	Secrets           *files.InstallVault `json:"-"`

	// GCP Specific
	ProjectDisplayName    string `json:"project_display_name"`
	BillingAccount        string `json:"billing_account"`
	BaseDomain            string `json:"base_domain"`
	GithubAppClientID     string `json:"-"`
	GithubAppClientSecret string `json:"-"`
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
	}, nil
}

func GetInfraFilePath() string {
	workdir := env.NewEnv().GetOmsWorkdir()
	return fmt.Sprintf("%s/gcp-infra.json", workdir)
}

func (b *GCPBootstrapper) Bootstrap() error {
	if b.Env.InstallVersion != "" {
		err := b.stlog.Step("Validate package to install", b.ValidatePackageName)
		if err != nil {
			return fmt.Errorf("invalid package name: %w", err)
		}

	}
	err := b.stlog.Step("Ensure install config", b.EnsureInstallConfig)
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

	if b.Env.InstallVersion != "" {
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

func (b *GCPBootstrapper) ValidatePackageName() error {
	build, err := b.PortalClient.GetBuild(portal.CodesphereProduct, b.Env.InstallVersion, b.Env.InstallHash)
	if err != nil {
		return fmt.Errorf("failed to get codesphere package: %w", err)
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

func (b *GCPBootstrapper) EnsureInstallConfig() error {
	if b.fw.Exists(b.Env.InstallConfigPath) {
		err := b.icg.LoadInstallConfigFromFile(b.Env.InstallConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}

		b.Env.ExistingConfigUsed = true
	} else {
		err := b.icg.ApplyProfile("dev")
		if err != nil {
			return fmt.Errorf("failed to apply profile: %w", err)
		}
	}

	b.Env.InstallConfig = b.icg.GetInstallConfig()

	return nil
}

func (b *GCPBootstrapper) EnsureSecrets() error {
	if b.fw.Exists(b.Env.SecretsFilePath) {
		err := b.icg.LoadVaultFromFile(b.Env.SecretsFilePath)
		if err != nil {
			return fmt.Errorf("failed to load vault file: %w", err)
		}
		err = b.icg.MergeVaultIntoConfig()
		if err != nil {
			return fmt.Errorf("failed to merge vault into config: %w", err)
		}
	}

	b.Env.Secrets = b.icg.GetVault()

	return nil
}

func (b *GCPBootstrapper) EnsureProject() error {
	parent := ""
	if b.Env.FolderID != "" {
		parent = fmt.Sprintf("folders/%s", b.Env.FolderID)
	}

	existingProject, err := b.GCPClient.GetProjectByName(b.Env.FolderID, b.Env.ProjectName)
	if err == nil {
		b.Env.ProjectID = existingProject.ProjectId
		b.Env.ProjectName = existingProject.Name
		return nil
	}
	if err.Error() == fmt.Sprintf("project not found: %s", b.Env.ProjectName) {
		projectId := b.GCPClient.CreateProjectID(b.Env.ProjectName)
		_, err = b.GCPClient.CreateProject(parent, projectId, b.Env.ProjectName)
		if err != nil {
			return fmt.Errorf("failed to create project: %w", err)
		}

		b.Env.ProjectID = projectId
		return nil
	}

	return fmt.Errorf("failed to get project: %w", err)
}

func (b *GCPBootstrapper) EnsureBilling() error {
	bi, err := b.GCPClient.GetBillingInfo(b.Env.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get billing info: %w", err)
	}
	if bi.BillingEnabled && bi.BillingAccountName == b.Env.BillingAccount {
		return nil
	}

	err = b.GCPClient.EnableBilling(b.Env.ProjectID, b.Env.BillingAccount)
	if err != nil {
		return fmt.Errorf("failed to enable billing: %w", err)
	}

	return nil
}

func (b *GCPBootstrapper) EnsureAPIsEnabled() error {
	apis := []string{
		"compute.googleapis.com",
		"serviceusage.googleapis.com",
		"artifactregistry.googleapis.com",
		"dns.googleapis.com",
	}

	err := b.GCPClient.EnableAPIs(b.Env.ProjectID, apis)
	if err != nil {
		return fmt.Errorf("failed to enable APIs: %w", err)
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

func (b *GCPBootstrapper) EnsureServiceAccounts() error {
	_, _, err := b.GCPClient.CreateServiceAccount(b.Env.ProjectID, "cloud-controller", "cloud-controller")
	if err != nil {
		return err
	}

	if b.Env.RegistryType == RegistryTypeArtifactRegistry {
		sa, newSa, err := b.GCPClient.CreateServiceAccount(b.Env.ProjectID, "artifact-registry-writer", "artifact-registry-writer")
		if err != nil {
			return err
		}

		if !newSa && b.Env.InstallConfig.Registry.Password != "" {
			return nil
		}

		for retries := range 5 {
			privateKey, err := b.GCPClient.CreateServiceAccountKey(b.Env.ProjectID, sa)

			if err != nil && status.Code(err) != codes.AlreadyExists {
				if retries > 3 {
					return fmt.Errorf("failed to create service account key: %w", err)
				}
				b.stlog.LogRetry()
				time.Sleep(5 * time.Second)
				continue
			}

			b.Env.InstallConfig.Registry.Password = string(privateKey)
			b.Env.InstallConfig.Registry.Username = "_json_key_base64"

			break
		}
	}

	return nil
}

func (b *GCPBootstrapper) EnsureIAMRoles() error {
	err := b.ensureIAMRoleWithRetry("cloud-controller", []string{"roles/compute.admin"})
	if err != nil {
		return err
	}

	err = b.ensureDnsPermissions()
	if err != nil {
		return err
	}

	if b.Env.RegistryType != RegistryTypeArtifactRegistry {
		return nil
	}

	err = b.ensureIAMRoleWithRetry("artifact-registry-writer", []string{"roles/artifactregistry.writer"})
	return err
}

func (b *GCPBootstrapper) ensureIAMRoleWithRetry(serviceAccount string, roles []string) error {
	var err error
	for retries := range 5 {
		err = b.GCPClient.AssignIAMRole(b.Env.ProjectID, serviceAccount, roles)
		if err == nil {
			return nil
		}
		if retries < 4 {
			b.stlog.LogRetry()
			time.Sleep(5 * time.Second)
		}
	}
	return fmt.Errorf("failed to assign roles %v to service account %s: %w", roles, serviceAccount, err)
}

func (b *GCPBootstrapper) ensureDnsPermissions() error {
	if b.Env.DNSProjectID != "" {
		if b.Env.DNSProjectServiceAccount == "" {
			return errors.New("dns project service account with role roles/dns.admin must be provided when dns project id is set")
		}
		err := b.GCPClient.GrantImpersonation("cloud-controller", b.Env.ProjectID, b.Env.DNSProjectServiceAccount, b.Env.DNSProjectID)
		if err != nil {
			return fmt.Errorf("failed to grant impersonization on dns project %s to cloud-controller service account: %w", b.Env.DNSProjectID, err)
		}
		return nil
	}
	err := b.ensureIAMRoleWithRetry("cloud-controller", []string{"roles/dns.admin"})
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

type vmResult struct {
	vmType     string // jumpbox, postgres, ceph, k0s
	name       string
	externalIP string
	internalIP string
}

func (b *GCPBootstrapper) EnsureComputeInstances() error {
	projectID := b.Env.ProjectID
	region := b.Env.Region
	zone := b.Env.Zone

	network := fmt.Sprintf("projects/%s/global/networks/%s-vpc", projectID, projectID)
	subnetwork := fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s-%s-subnet", projectID, region, projectID, region)
	diskType := fmt.Sprintf("projects/%s/zones/%s/diskTypes/pd-ssd", projectID, zone)

	// Create VMs in parallel
	wg := sync.WaitGroup{}
	errCh := make(chan error, len(vmDefs))
	resultCh := make(chan vmResult, len(vmDefs))
	rootDiskSize := int64(200)
	if b.Env.RegistryType == RegistryTypeGitHub {
		rootDiskSize = 50
	}
	for _, vm := range vmDefs {
		wg.Add(1)
		go func(vm VMDef) {
			defer wg.Done()
			disks := []*computepb.AttachedDisk{
				{
					Boot:       protoBool(true),
					AutoDelete: protoBool(true),
					Type:       protoString("PERSISTENT"),
					InitializeParams: &computepb.AttachedDiskInitializeParams{
						DiskType:    &diskType,
						DiskSizeGb:  protoInt64(rootDiskSize),
						SourceImage: protoString("projects/ubuntu-os-cloud/global/images/family/ubuntu-2204-lts"),
					},
				},
			}
			for _, diskSize := range vm.AdditionalDisks {
				disks = append(disks, &computepb.AttachedDisk{
					Boot:       protoBool(false),
					AutoDelete: protoBool(true),
					Type:       protoString("PERSISTENT"),
					InitializeParams: &computepb.AttachedDiskInitializeParams{
						DiskSizeGb: protoInt64(diskSize),
						DiskType:   &diskType,
					},
				})
			}

			pubKey, err := b.readSSHKey(b.Env.SSHPublicKeyPath)
			if err != nil {
				errCh <- fmt.Errorf("failed to read SSH public key: %w", err)
				return
			}

			serviceAccount := fmt.Sprintf("cloud-controller@%s.iam.gserviceaccount.com", projectID)
			instance := &computepb.Instance{
				Name: protoString(vm.Name),
				ServiceAccounts: []*computepb.ServiceAccount{
					{
						Email:  protoString(serviceAccount),
						Scopes: []string{"https://www.googleapis.com/auth/cloud-platform"},
					},
				},
				MachineType: protoString(fmt.Sprintf("zones/%s/machineTypes/%s", zone, vm.MachineType)),
				Tags: &computepb.Tags{
					Items: vm.Tags,
				},
				Scheduling: &computepb.Scheduling{
					Preemptible: &b.Env.Preemptible,
				},
				NetworkInterfaces: []*computepb.NetworkInterface{
					{
						Network:    protoString(network),
						Subnetwork: protoString(subnetwork),
					},
				},
				Disks: disks,
				Metadata: &computepb.Metadata{
					Items: []*computepb.Items{
						{
							Key:   protoString("ssh-keys"),
							Value: protoString(fmt.Sprintf("root:%s\nubuntu:%s", pubKey+"root", pubKey+"ubuntu")),
						},
					},
				},
			}

			// Configure external IP if needed
			if vm.ExternalIP {
				instance.NetworkInterfaces[0].AccessConfigs = []*computepb.AccessConfig{
					{
						Name: protoString("External NAT"),
						Type: protoString("ONE_TO_ONE_NAT"),
					},
				}
			}

			err = b.GCPClient.CreateInstance(projectID, zone, instance)
			if err != nil && !isAlreadyExistsError(err) {
				errCh <- fmt.Errorf("failed to create instance %s: %w", vm.Name, err)
				return
			}

			// Find out the IP addresses of the created instance
			resp, err := b.GCPClient.GetInstance(projectID, zone, vm.Name)
			if err != nil {
				errCh <- fmt.Errorf("failed to get instance %s: %w", vm.Name, err)
				return
			}

			externalIP := ""
			internalIP := ""
			if len(resp.GetNetworkInterfaces()) > 0 {
				internalIP = resp.GetNetworkInterfaces()[0].GetNetworkIP()
				if len(resp.GetNetworkInterfaces()[0].GetAccessConfigs()) > 0 {
					externalIP = resp.GetNetworkInterfaces()[0].GetAccessConfigs()[0].GetNatIP()
				}
			}

			// Send result through channel instead of creating nodes in goroutine
			resultCh <- vmResult{
				vmType:     vm.Tags[0],
				name:       vm.Name,
				externalIP: externalIP,
				internalIP: internalIP,
			}
		}(vm)
	}
	wg.Wait()

	close(errCh)
	close(resultCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("error ensuring compute instances: %w", errors.Join(errs...))
	}

	// Create nodes from results (in main goroutine, not in spawned goroutines)
	b.Env.Jumpbox = &node.Node{
		NodeClient: b.NodeClient,
		FileIO:     b.fw,
	}
	for result := range resultCh {
		switch result.vmType {
		case "jumpbox":
			b.Env.Jumpbox.UpdateNode(result.name, result.externalIP, result.internalIP)
		case "postgres":
			b.Env.PostgreSQLNode = b.Env.Jumpbox.CreateSubNode(result.name, result.externalIP, result.internalIP)
		case "ceph":
			node := b.Env.Jumpbox.CreateSubNode(result.name, result.externalIP, result.internalIP)
			b.Env.CephNodes = append(b.Env.CephNodes, node)
		case "k0s":
			node := b.Env.Jumpbox.CreateSubNode(result.name, result.externalIP, result.internalIP)
			b.Env.ControlPlaneNodes = append(b.Env.ControlPlaneNodes, node)
		}
	}

	//sort ceph nodes by name to ensure consistent ordering
	sort.Slice(b.Env.CephNodes, func(i, j int) bool {
		return b.Env.CephNodes[i].GetName() < b.Env.CephNodes[j].GetName()
	})
	//sort control plane nodes by name to ensure consistent ordering
	sort.Slice(b.Env.ControlPlaneNodes, func(i, j int) bool {
		return b.Env.ControlPlaneNodes[i].GetName() < b.Env.ControlPlaneNodes[j].GetName()
	})

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
	if err != nil && !isAlreadyExistsError(err) {
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
		time.Sleep(10 * time.Second)
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

	hasOms := b.Env.Jumpbox.HasCommand("oms-cli")
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

func (b *GCPBootstrapper) UpdateInstallConfig() error {
	// Update install config with necessary values
	b.Env.InstallConfig.Datacenter.ID = b.Env.DatacenterID
	b.Env.InstallConfig.Datacenter.City = "Karlsruhe"
	b.Env.InstallConfig.Datacenter.CountryCode = "DE"
	b.Env.InstallConfig.Secrets.BaseDir = b.Env.SecretsDir
	if b.Env.RegistryType != RegistryTypeGitHub {
		b.Env.InstallConfig.Registry.ReplaceImagesInBom = true
		b.Env.InstallConfig.Registry.LoadContainerImages = true
	}

	if b.Env.InstallConfig.Postgres.Primary == nil {
		b.Env.InstallConfig.Postgres.Primary = &files.PostgresPrimaryConfig{
			Hostname: b.Env.PostgreSQLNode.GetName(),
		}
	}
	b.Env.InstallConfig.Postgres.Primary.IP = b.Env.PostgreSQLNode.GetInternalIP()

	b.Env.InstallConfig.Ceph.CsiKubeletDir = "/var/lib/k0s/kubelet"
	b.Env.InstallConfig.Ceph.NodesSubnet = "10.10.0.0/20"
	b.Env.InstallConfig.Ceph.Hosts = []files.CephHost{
		{
			Hostname:  b.Env.CephNodes[0].GetName(),
			IsMaster:  true,
			IPAddress: b.Env.CephNodes[0].GetInternalIP(),
		},
		{
			Hostname:  b.Env.CephNodes[1].GetName(),
			IPAddress: b.Env.CephNodes[1].GetInternalIP(),
		},
		{
			Hostname:  b.Env.CephNodes[2].GetName(),
			IPAddress: b.Env.CephNodes[2].GetInternalIP(),
		},
		{
			Hostname:  b.Env.CephNodes[3].GetName(),
			IPAddress: b.Env.CephNodes[3].GetInternalIP(),
		},
	}
	b.Env.InstallConfig.Ceph.OSDs = []files.CephOSD{
		{
			SpecID: "default",
			Placement: files.CephPlacement{
				HostPattern: "*",
			},
			DataDevices: files.CephDataDevices{
				Size:  "100G:",
				Limit: 1,
			},
			DBDevices: files.CephDBDevices{
				Size:  "10G:500G",
				Limit: 1,
			},
		},
	}

	b.Env.InstallConfig.Kubernetes = files.KubernetesConfig{
		ManagedByCodesphere: true,
		APIServerHost:       b.Env.ControlPlaneNodes[0].GetInternalIP(),
		ControlPlanes: []files.K8sNode{
			{
				IPAddress: b.Env.ControlPlaneNodes[0].GetInternalIP(),
			},
		},
		Workers: []files.K8sNode{
			{
				IPAddress: b.Env.ControlPlaneNodes[0].GetInternalIP(),
			},

			{
				IPAddress: b.Env.ControlPlaneNodes[1].GetInternalIP(),
			},
			{
				IPAddress: b.Env.ControlPlaneNodes[2].GetInternalIP(),
			},
		},
	}
	b.Env.InstallConfig.Cluster.Monitoring = &files.MonitoringConfig{
		Prometheus: &files.PrometheusConfig{
			RemoteWrite: &files.RemoteWriteConfig{
				Enabled:     false,
				ClusterName: "GCP-test",
			},
		},
	}
	b.Env.InstallConfig.Cluster.Gateway = files.GatewayConfig{
		ServiceType: "LoadBalancer",
		//IPAddresses: []string{b.Env.ControlPlaneNodes[0].ExternalIP},
		Annotations: map[string]string{
			"cloud.google.com/load-balancer-ipv4": b.Env.GatewayIP,
		},
	}
	b.Env.InstallConfig.Cluster.PublicGateway = files.GatewayConfig{
		ServiceType: "LoadBalancer",
		Annotations: map[string]string{
			"cloud.google.com/load-balancer-ipv4": b.Env.PublicGatewayIP,
		},
	}

	dnsProject := b.Env.DNSProjectID
	if b.Env.DNSProjectID == "" {
		dnsProject = b.Env.ProjectID
	}
	b.Env.InstallConfig.Cluster.Certificates.Override = map[string]interface{}{
		"issuers": map[string]interface{}{
			"letsEncryptHttp": map[string]interface{}{
				"enabled": true,
			},
			"acme": map[string]interface{}{
				"dnsSolver": map[string]interface{}{
					"config": map[string]interface{}{
						"cloudDNS": map[string]interface{}{
							"project": dnsProject,
						},
					},
				},
			},
		},
	}
	b.Env.InstallConfig.Codesphere.CertIssuer = files.CertIssuerConfig{
		Type: "acme",
		Acme: &files.ACMEConfig{
			Email:  "oms-testing@" + b.Env.BaseDomain,
			Server: "https://acme-v02.api.letsencrypt.org/directory",
		},
	}

	b.Env.InstallConfig.Codesphere.Domain = "cs." + b.Env.BaseDomain
	b.Env.InstallConfig.Codesphere.WorkspaceHostingBaseDomain = "ws." + b.Env.BaseDomain
	b.Env.InstallConfig.Codesphere.PublicIP = b.Env.ControlPlaneNodes[1].GetExternalIP()
	b.Env.InstallConfig.Codesphere.CustomDomains = files.CustomDomainsConfig{
		CNameBaseDomain: "ws." + b.Env.BaseDomain,
	}
	b.Env.InstallConfig.Codesphere.DNSServers = []string{"8.8.8.8"}
	b.Env.InstallConfig.Codesphere.DeployConfig = files.DeployConfig{
		Images: map[string]files.ImageConfig{
			"ubuntu-24.04": {
				Name:           "Ubuntu 24.04",
				SupportedUntil: "2028-05-31",
				Flavors: map[string]files.FlavorConfig{
					"default": {
						Image: files.ImageRef{
							BomRef: "workspace-agent-24.04",
						},
						Pool: map[int]int{
							1: 1,
							2: 1,
							3: 0,
						},
					},
				},
			},
		},
	}
	b.Env.InstallConfig.Codesphere.Plans = files.PlansConfig{
		HostingPlans: map[int]files.HostingPlan{
			1: {
				CPUTenth:      20,
				GPUParts:      0,
				MemoryMb:      4096,
				StorageMb:     20480,
				TempStorageMb: 1024,
			},
			2: {
				CPUTenth:      40,
				GPUParts:      0,
				MemoryMb:      8192,
				StorageMb:     40960,
				TempStorageMb: 1024,
			},
			3: {
				CPUTenth:      80,
				GPUParts:      0,
				MemoryMb:      16384,
				StorageMb:     40960,
				TempStorageMb: 1024,
			},
		},
		WorkspacePlans: map[int]files.WorkspacePlan{
			1: {
				Name:          "Standard",
				HostingPlanID: 1,
				MaxReplicas:   3,
				OnDemand:      true,
			},
			2: {
				Name:          "Big",
				HostingPlanID: 2,
				MaxReplicas:   3,
				OnDemand:      true,
			},
			3: {
				Name:          "Pro",
				HostingPlanID: 3,
				MaxReplicas:   3,
				OnDemand:      true,
			},
		},
	}
	b.Env.InstallConfig.Codesphere.GitProviders = &files.GitProvidersConfig{
		GitHub: &files.GitProviderConfig{
			Enabled: true,
			URL:     "https://github.com",
			API: files.APIConfig{
				BaseURL: "https://api.github.com",
			},
			OAuth: files.OAuthConfig{
				Issuer:                "https://github.com",
				AuthorizationEndpoint: "https://github.com/login/oauth/authorize",
				TokenEndpoint:         "https://github.com/login/oauth/access_token",

				ClientID:     b.Env.GithubAppClientID,
				ClientSecret: b.Env.GithubAppClientSecret,
			},
		},
	}
	b.Env.InstallConfig.Codesphere.Experiments = b.Env.Experiments
	b.Env.InstallConfig.Codesphere.Features = b.Env.FeatureFlags
	b.Env.InstallConfig.Codesphere.ManagedServices = []files.ManagedServiceConfig{}

	if !b.Env.ExistingConfigUsed {
		err := b.icg.GenerateSecrets()
		if err != nil {
			return fmt.Errorf("failed to generate secrets: %w", err)
		}
	} else {
		var err error
		b.Env.InstallConfig.Postgres.Primary.PrivateKey, b.Env.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem, err = installer.GenerateServerCertificate(
			b.Env.InstallConfig.Postgres.CaCertPrivateKey,
			b.Env.InstallConfig.Postgres.CACertPem,
			b.Env.InstallConfig.Postgres.Primary.Hostname,
			[]string{b.Env.InstallConfig.Postgres.Primary.IP})
		if err != nil {
			return fmt.Errorf("failed to generate primary server certificate: %w", err)
		}
		if b.Env.InstallConfig.Postgres.Replica != nil {
			b.Env.InstallConfig.Postgres.ReplicaPrivateKey, b.Env.InstallConfig.Postgres.Replica.SSLConfig.ServerCertPem, err = installer.GenerateServerCertificate(
				b.Env.InstallConfig.Postgres.CaCertPrivateKey,
				b.Env.InstallConfig.Postgres.CACertPem,
				b.Env.InstallConfig.Postgres.Replica.Name,
				[]string{b.Env.InstallConfig.Postgres.Replica.IP})
			if err != nil {
				return fmt.Errorf("failed to generate replica server certificate: %w", err)
			}
		}
	}

	if err := b.icg.WriteInstallConfig(b.Env.InstallConfigPath, true); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if err := b.icg.WriteVault(b.Env.SecretsFilePath, true); err != nil {
		return fmt.Errorf("failed to write vault file: %w", err)
	}

	err := b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, b.Env.InstallConfigPath, "/etc/codesphere/config.yaml")
	if err != nil {
		return fmt.Errorf("failed to copy install config to jumpbox: %w", err)
	}

	err = b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, b.Env.SecretsFilePath, b.Env.SecretsDir+"/prod.vault.yaml")
	if err != nil {
		return fmt.Errorf("failed to copy secrets file to jumpbox: %w", err)
	}
	return nil
}

func (b *GCPBootstrapper) EnsureAgeKey() error {
	hasKey := b.Env.Jumpbox.NodeClient.HasFile(b.Env.Jumpbox, b.Env.SecretsDir+"/age_key.txt")
	if hasKey {
		return nil
	}

	err := b.Env.Jumpbox.RunSSHCommand("root", fmt.Sprintf("mkdir -p %s; age-keygen -o %s/age_key.txt", b.Env.SecretsDir, b.Env.SecretsDir))
	if err != nil {
		return fmt.Errorf("failed to generate age key on jumpbox: %w", err)
	}

	return nil
}

func (b *GCPBootstrapper) EncryptVault() error {
	err := b.Env.Jumpbox.RunSSHCommand("root", "cp "+b.Env.SecretsDir+"/prod.vault.yaml{,.bak}")
	if err != nil {
		return fmt.Errorf("failed backup vault on jumpbox: %w", err)
	}

	err = b.Env.Jumpbox.RunSSHCommand("root", "sops --encrypt --in-place --age $(age-keygen -y "+b.Env.SecretsDir+"/age_key.txt) "+b.Env.SecretsDir+"/prod.vault.yaml")
	if err != nil {
		return fmt.Errorf("failed to encrypt vault on jumpbox: %w", err)
	}

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
	packageFile := "installer.tar.gz"
	skipSteps := b.Env.InstallSkipSteps
	if b.Env.RegistryType == RegistryTypeGitHub {
		skipSteps = append(skipSteps, "load-container-images")
		packageFile = "installer-lite.tar.gz"
	}
	skipStepsArg := ""
	if len(skipSteps) > 0 {
		skipStepsArg = " -s " + strings.Join(skipSteps, ",")
	}

	downloadCmd := "oms-cli download package -f " + packageFile + " " + b.Env.InstallVersion
	err := b.Env.Jumpbox.RunSSHCommand("root", downloadCmd)
	if err != nil {
		return fmt.Errorf("failed to download Codesphere package from jumpbox: %w", err)
	}

	installCmd := fmt.Sprintf("oms-cli install codesphere -c /etc/codesphere/config.yaml -k %s/age_key.txt -p %s-%s%s",
		b.Env.SecretsDir, b.Env.InstallVersion, packageFile, skipStepsArg)
	err = b.Env.Jumpbox.RunSSHCommand("root", installCmd)
	if err != nil {
		return fmt.Errorf("failed to install Codesphere from jumpbox: %w", err)
	}

	return nil
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

// Helper functions
func isAlreadyExistsError(err error) bool {
	return status.Code(err) == codes.AlreadyExists || strings.Contains(err.Error(), "already exists")
}

// readSSHKey reads an SSH key file, expanding ~ in the path
func (b *GCPBootstrapper) readSSHKey(path string) (string, error) {
	realPath := util.ExpandPath(path)
	data, err := b.fw.ReadFile(realPath)
	if err != nil {
		return "", fmt.Errorf("error reading SSH key from %s: %w", realPath, err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		return "", fmt.Errorf("SSH key at %s is empty", realPath)
	}
	return key, nil
}
