package bootstrap

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	artifact "cloud.google.com/go/artifactregistry/apiv1"
	artifactpb "cloud.google.com/go/artifactregistry/apiv1/artifactregistrypb"
	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"cloud.google.com/go/iam/apiv1/iampb"
	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	"cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"
	serviceusage "cloud.google.com/go/serviceusage/apiv1"
	"cloud.google.com/go/serviceusage/apiv1/serviceusagepb"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/cloudbilling/v1"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GCPBootstrapper struct {
	ctx           context.Context
	env           *CodesphereEnvironemnt
	InstallConfig *files.RootConfig
	Secrets       *files.InstallVault
	icg           installer.InstallConfigManager
	NodeManager   *node.NodeManager
}

type CodesphereEnvironemnt struct {
	ProjectID            string      `json:"project_id"`
	ProjectName          string      `json:"project_name"`
	DNSProjectID         string      `json:"dns_project_id"`
	PostgreSQLNode       node.Node   `json:"postgresql_node"`
	ControlPlaneNodes    []node.Node `json:"control_plane_nodes"`
	CephNodes            []node.Node `json:"ceph_nodes"`
	Jumpbox              node.Node   `json:"jumpbox"`
	ContainerRegistryURL string      `json:"container_registry_url"`

	ProjectDisplayName    string
	BillingAccount        string
	BaseDomain            string
	GithubAppClientID     string
	GithubAppClientSecret string
	SecretsDir            string
	FolderID              string
	SSHPublicKeyPath      string
	SSHPrivateKeyPath     string
	SchedulingType        string
	DatacenterID          int
	CustomPgIP            string
	InstallConfig         string
	SecretsFile           string
	Region                string
	Zone                  string
	DNSZoneName           string
}

func NewGCPBootstrapper(env env.Env, CodesphereEnv *CodesphereEnvironemnt) (*GCPBootstrapper, error) {
	ctx := context.Background()
	fw := util.NewFilesystemWriter()
	icg := installer.NewInstallConfigManager()
	nm := &node.NodeManager{
		FileIO:  fw,
		KeyPath: CodesphereEnv.SSHPrivateKeyPath,
	}
	if fw.Exists(CodesphereEnv.InstallConfig) {
		fmt.Printf("Reading install config file: %s\n", CodesphereEnv.InstallConfig)
		err := icg.LoadInstallConfigFromFile(CodesphereEnv.InstallConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	} else {
		icg.ApplyProfile("dev")
	}

	if fw.Exists(CodesphereEnv.SecretsFile) {
		fmt.Printf("Reading vault file: %s\n", CodesphereEnv.SecretsFile)
		err := icg.LoadVaultFromFile(CodesphereEnv.SecretsFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load vault file: %w", err)
		}
	}
	return &GCPBootstrapper{
		env:           CodesphereEnv,
		InstallConfig: icg.GetInstallConfig(),
		NodeManager:   nm,
		Secrets:       icg.GetVault(),
		ctx:           ctx,
		icg:           icg,
	}, nil
}

func (b *GCPBootstrapper) Bootstrap() (*CodesphereEnvironemnt, error) {
	err := b.EnsureProject()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure GCP project: %w", err)
	}

	err = b.EnsureBilling()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure billing is enabled: %w", err)
	}

	err = b.EnsureAPIsEnabled()
	if err != nil {
		return b.env, fmt.Errorf("failed to enable required APIs: %w", err)
	}

	err = b.EnsureArtifactRegistry()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure artifact registry: %w", err)
	}

	err = b.EnsureServiceAccounts()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure service accounts: %w", err)
	}

	err = b.EnsureIAMRoles()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure IAM roles: %w", err)
	}

	err = b.EnsureVPC()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure VPC: %w", err)
	}

	err = b.EnsureFirewallRules()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure firewall rules: %w", err)
	}

	err = b.EnsureComputeInstances()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure compute instances: %w", err)
	}

	err = b.EnsureRootLoginEnabled()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure root login is enabled: %w", err)
	}

	err = b.EnsureJumpboxConfigured()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure jumpbox is configured: %w", err)
	}

	err = b.UpdateInstallConfig()
	if err != nil {
		return b.env, fmt.Errorf("failed to update install config: %w", err)
	}

	err = b.EnsureAgeKey()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure age key: %w", err)
	}

	err = b.EncryptVault()
	if err != nil {
		return b.env, fmt.Errorf("failed to encrypt vault: %w", err)
	}

	err = b.EnsureDNSRecords()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure DNS records: %w", err)
	}
	return b.env, nil
}

func (b *GCPBootstrapper) EnsureProject() error {
	client, err := resourcemanager.NewProjectsClient(b.ctx, option.WithCredentialsFile(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")))
	if err != nil {
		return fmt.Errorf("failed to create resource manager client: %w", err)
	}
	defer client.Close()

	parent := ""
	if b.env.FolderID != "" {
		parent = fmt.Sprintf("folders/%s", b.env.FolderID)
	}

	// Generate a unique project ID
	projectGuid := strings.ToLower(shortuuid.New()[:8])
	projectId := b.env.ProjectName + "-" + projectGuid
	project := &resourcemanagerpb.Project{
		ProjectId:   projectId,
		DisplayName: b.env.ProjectName,
		Parent:      parent,
	}

	existingProject, err := b.getProjectByName(b.ctx, client, b.env.ProjectName)
	if err == nil {
		b.env.ProjectID = existingProject.ProjectId
		b.env.ProjectName = existingProject.Name
		return nil
	}
	if err.Error() == fmt.Sprintf("project not found: %s", b.env.ProjectName) {
		_, err = client.CreateProject(b.ctx, &resourcemanagerpb.CreateProjectRequest{
			Project: project,
		})
		if err != nil {
			return fmt.Errorf("failed to create project: %w", err)
		}
		b.env.ProjectID = project.ProjectId
		b.env.ProjectName = project.Name
		return nil
	}
	return fmt.Errorf("failed to get project: %w", err)
}

func (b *GCPBootstrapper) EnsureBilling() error {
	projectID := b.env.ProjectID
	billingAccount := b.env.BillingAccount

	ctx := b.ctx
	billingService, err := cloudbilling.NewService(ctx, option.WithCredentialsFile(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")))
	if err != nil {
		return fmt.Errorf("failed to create Cloud Billing service: %w", err)
	}

	projectName := fmt.Sprintf("projects/%s", projectID)
	billingInfo := &cloudbilling.ProjectBillingInfo{
		BillingAccountName: fmt.Sprintf("billingAccounts/%s", billingAccount),
	}

	bi, err := billingService.Projects.GetBillingInfo(projectName).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to get billing info: %w", err)
	}
	if bi.BillingEnabled && bi.BillingAccountName == billingInfo.BillingAccountName {
		return nil
	}

	_, err = billingService.Projects.UpdateBillingInfo(projectName, billingInfo).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to enable billing: %w", err)
	}

	fmt.Printf("Billing enabled for project %s with account %s\n", projectID, billingAccount)
	return nil
}

func (b *GCPBootstrapper) getProjectByName(ctx context.Context, client *resourcemanager.ProjectsClient, displayName string) (*resourcemanagerpb.Project, error) {
	req := &resourcemanagerpb.ListProjectsRequest{
		Parent:      fmt.Sprintf("folders/%s", b.env.FolderID),
		ShowDeleted: false,
	}

	it := client.ListProjects(ctx, req)

	for {
		project, err := it.Next()
		if err == iterator.Done {
			// No more results found
			return nil, fmt.Errorf("project not found: %s", displayName)
		}
		if err != nil {
			return nil, fmt.Errorf("error iterating projects: %w", err)
		}

		// Because the filter is a prefix search on the display name,
		// we should perform an exact match check here to be sure.
		if project.GetDisplayName() == displayName {
			return project, nil
		}
	}
}

func (b *GCPBootstrapper) EnsureAPIsEnabled() error {
	client, err := serviceusage.NewClient(b.ctx, option.WithCredentialsFile(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")))
	if err != nil {
		return fmt.Errorf("failed to create serviceusage client: %w", err)
	}
	defer client.Close()

	apis := []string{
		"compute.googleapis.com",
		"serviceusage.googleapis.com",
		"artifactregistry.googleapis.com",
	}

	for _, api := range apis {
		log.Printf("Enabling API: %s", api)
		serviceName := fmt.Sprintf("%s/services/%s", b.env.ProjectName, api)
		// Figure out if API is already enabled
		svc, err := client.GetService(b.ctx, &serviceusagepb.GetServiceRequest{Name: serviceName})
		if err == nil && svc.State == serviceusagepb.State_ENABLED {
			fmt.Printf("API %s already enabled\n", api)
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to get service %s: %w", api, err)
		}

		// Enable the API
		op, err := client.EnableService(b.ctx, &serviceusagepb.EnableServiceRequest{Name: serviceName})
		if err != nil {
			// If already enabled, ignore error
			if status.Code(err) == codes.AlreadyExists {
				fmt.Printf("API %s already enabled\n", api)
				continue
			}
			return fmt.Errorf("failed to enable API %s: %w", api, err)
		}
		if _, err := op.Wait(b.ctx); err != nil {
			return fmt.Errorf("failed to enable API %s: %w", api, err)
		}
		fmt.Printf("Enabled API: %s\n", api)

	}

	return nil
}

func (b *GCPBootstrapper) EnsureArtifactRegistry() error {
	projectID := b.env.ProjectID
	location := b.env.Region // You may want to make this configurable
	repoName := "codesphere-registry"
	fullRepoName := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", projectID, location, repoName)

	ctx := b.ctx

	artifactClient, err := artifact.NewClient(ctx, option.WithCredentialsFile(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")))
	if err != nil {
		return fmt.Errorf("failed to create artifact registry client: %w", err)
	}
	defer artifactClient.Close()

	parent := fmt.Sprintf("projects/%s/locations/%s", projectID, location)
	repoReq := &artifactpb.CreateRepositoryRequest{
		Parent:       parent,
		RepositoryId: repoName,
		Repository: &artifactpb.Repository{
			Format:      artifactpb.Repository_DOCKER,
			Description: "Codesphere managed registry",
			Name:        fullRepoName,
		},
	}

	op, err := artifactClient.CreateRepository(ctx, repoReq)
	if err != nil && status.Code(err) != codes.AlreadyExists {
		return fmt.Errorf("failed to create artifact registry repository: %w", err)
	}
	var repo *artifactpb.Repository
	if err == nil {
		repo, err = op.Wait(ctx)
		if err != nil {
			return fmt.Errorf("failed to wait for artifact registry repository creation: %w", err)
		}
	}

	if repo == nil {
		repo, err = artifactClient.GetRepository(b.ctx, &artifactpb.GetRepositoryRequest{
			Name: fullRepoName,
		})
		if err != nil {
			return fmt.Errorf("failed to get artifact registry repository: %w", err)
		}
	}

	b.InstallConfig.Registry.Server = repo.GetRegistryUri()
	fmt.Printf("Artifact Registry repository %s ensured\n", repoName)

	return nil
}

func (b *GCPBootstrapper) EnsureServiceAccounts() error {
	projectID := b.env.ProjectID

	saID := "artifact-registry-writer"
	saEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", saID, projectID)
	iamService, err := iam.NewService(b.ctx)
	if err != nil {
		log.Fatal(err)
	}
	saReq := &iam.CreateServiceAccountRequest{
		AccountId: saID,
		ServiceAccount: &iam.ServiceAccount{
			DisplayName: "Artifact Registry Writer",
		},
	}

	newSA := false
	_, err = iamService.Projects.ServiceAccounts.Create(fmt.Sprintf("projects/%s", projectID), saReq).Context(b.ctx).Do()
	if err != nil && !strings.HasPrefix(err.Error(), "googleapi: Error 409: Service account") {
		return fmt.Errorf("failed to create service account: %w", err)
	}
	if err == nil {
		newSA = true
	}
	fmt.Printf("Service account %s ensured\n", saEmail)

	if !newSA && b.InstallConfig.GetRegistryPassword(b.Secrets) != "" {
		return nil
	}

	key, err := iamService.Projects.ServiceAccounts.Keys.Get(fmt.Sprintf("projects/%s/serviceAccounts/%s", projectID, saEmail)).Context(b.ctx).Do()
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("failed to get service account key: %w", err)
	}
	if err == nil {
		b.InstallConfig.SetRegistryPassword(b.Secrets, string(key.PublicKeyData))
		b.InstallConfig.SetRegistryUsername(b.Secrets, "_json_key_base64")
		return nil
	}
	for retries := range 5 {
		// Create Service Account Key
		keyReq := &iam.CreateServiceAccountKeyRequest{}
		key, err := iamService.Projects.ServiceAccounts.Keys.Create(fmt.Sprintf("projects/%s/serviceAccounts/%s", projectID, saEmail), keyReq).Context(b.ctx).Do()

		if err != nil && status.Code(err) != codes.AlreadyExists {
			if retries > 3 {
				return fmt.Errorf("failed to create service account key: %w", err)
			}
			time.Sleep(5 * time.Second)
			continue
		}
		fmt.Printf("Service account key for %s ensured\n", saEmail)
		b.InstallConfig.SetRegistryPassword(b.Secrets, string(key.PublicKeyData))
		b.InstallConfig.SetRegistryUsername(b.Secrets, "_json_key_base64")
		break
	}

	return nil
}

func (b *GCPBootstrapper) EnsureIAMRoles() error {
	projectID := b.env.ProjectID
	ctx := b.ctx

	saID := "artifact-registry-writer"
	saEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", saID, projectID)

	c, err := resourcemanager.NewProjectsClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create resource manager client: %w", err)
	}
	defer c.Close()

	member := fmt.Sprintf("serviceAccount:%s", saEmail)
	role := "roles/artifactregistry.writer"

	getReq := &iampb.GetIamPolicyRequest{
		Resource: b.env.ProjectName,
	}

	policy, err := c.GetIamPolicy(ctx, getReq)
	if err != nil {
		return fmt.Errorf("failed to get IAM policy: %w", err)
	}

	// Check if the binding already exists
	for _, binding := range policy.Bindings {
		if binding.Role == role {
			for _, m := range binding.Members {
				if m == member {
					fmt.Printf("IAM role %s already assigned to %s\n", role, member)
					return nil
				}
			}
		}
	}

	policy.Bindings = append(policy.Bindings, &iampb.Binding{
		Role:    role,
		Members: []string{member},
	})
	req := &iampb.SetIamPolicyRequest{
		Resource: b.env.ProjectName,
		Policy:   policy,
	}

	_, err = c.SetIamPolicy(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to set IAM policy: %w", err)
	}

	fmt.Printf("Assigned IAM role %s to %s\n", role, member)
	return nil
}

func (b *GCPBootstrapper) EnsureVPC() error {
	projectID := b.env.ProjectID
	networkName := fmt.Sprintf("%s-vpc", projectID)
	subnetName := fmt.Sprintf("%s-%s-subnet", projectID, b.env.Region)
	routerName := fmt.Sprintf("%s-router", projectID)
	natName := fmt.Sprintf("%s-nat-gateway", projectID)

	ctx := b.ctx

	// Create VPC
	networksClient, err := compute.NewNetworksRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create networks client: %w", err)
	}
	defer networksClient.Close()

	network := &computepb.Network{
		Name:                  &networkName,
		AutoCreateSubnetworks: protoBool(false),
	}
	op, err := networksClient.Insert(ctx, &computepb.InsertNetworkRequest{
		Project:         projectID,
		NetworkResource: network,
	})
	if err != nil && !isAlreadyExistsError(err) {
		return fmt.Errorf("failed to create VPC: %w", err)
	}
	if err == nil {
		if err := op.Wait(ctx); err != nil {
			return fmt.Errorf("failed to wait for VPC creation: %w", err)
		}
	}
	fmt.Printf("VPC %s ensured\n", networkName)

	// Create Subnet
	subnetsClient, err := compute.NewSubnetworksRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create subnetworks client: %w", err)
	}
	defer subnetsClient.Close()

	subnet := &computepb.Subnetwork{
		Name:        &subnetName,
		IpCidrRange: protoString("10.10.0.0/20"),
		Region:      &b.env.Region,
		Network:     protoString(fmt.Sprintf("projects/%s/global/networks/%s", projectID, networkName)),
	}
	op, err = subnetsClient.Insert(ctx, &computepb.InsertSubnetworkRequest{
		Project:            projectID,
		Region:             b.env.Region,
		SubnetworkResource: subnet,
	})
	if err != nil && !isAlreadyExistsError(err) {
		return fmt.Errorf("failed to create subnet: %w", err)
	}
	if err == nil {
		if err := op.Wait(ctx); err != nil {
			return fmt.Errorf("failed to wait for subnet creation: %w", err)
		}
	}
	fmt.Printf("Subnet %s ensured\n", subnetName)

	// Create Router
	routersClient, err := compute.NewRoutersRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create routers client: %w", err)
	}
	defer routersClient.Close()

	router := &computepb.Router{
		Name:    &routerName,
		Region:  &b.env.Region,
		Network: protoString(fmt.Sprintf("projects/%s/global/networks/%s", projectID, networkName)),
	}
	op, err = routersClient.Insert(ctx, &computepb.InsertRouterRequest{
		Project:        projectID,
		Region:         b.env.Region,
		RouterResource: router,
	})
	if err != nil && !isAlreadyExistsError(err) {
		return fmt.Errorf("failed to create router: %w", err)
	}
	if err == nil {
		if err := op.Wait(ctx); err != nil {
			return fmt.Errorf("failed to wait for router creation: %w", err)
		}
	}
	fmt.Printf("Router %s ensured\n", routerName)

	// Create NAT Gateway
	natsClient, err := compute.NewRoutersRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create routers client for NAT: %w", err)
	}
	defer natsClient.Close()

	nat := &computepb.RouterNat{
		Name:                          &natName,
		SourceSubnetworkIpRangesToNat: protoString("ALL_SUBNETWORKS_ALL_IP_RANGES"),
		NatIpAllocateOption:           protoString("AUTO_ONLY"),
		LogConfig: &computepb.RouterNatLogConfig{
			Enable: protoBool(false),
			Filter: protoString("ERRORS_ONLY"),
		},
	}
	// Patch NAT config to router
	_, err = routersClient.Patch(ctx, &computepb.PatchRouterRequest{
		Project: projectID,
		Region:  b.env.Region,
		Router:  routerName,
		RouterResource: &computepb.Router{
			Name: &routerName,
			Nats: []*computepb.RouterNat{nat},
		},
	})
	if err != nil && !isAlreadyExistsError(err) {
		return fmt.Errorf("failed to create NAT gateway: %w", err)
	}
	fmt.Printf("NAT gateway %s ensured\n", natName)

	return nil
}

func (b *GCPBootstrapper) EnsureFirewallRules() error {
	projectID := b.env.ProjectID
	networkName := fmt.Sprintf("%s-vpc", projectID)
	ctx := b.ctx

	firewallsClient, err := compute.NewFirewallsRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create firewalls client: %w", err)
	}
	defer firewallsClient.Close()

	// Allow external SSH to Jumpbox
	sshRule := &computepb.Firewall{
		Name:      protoString("allow-ssh-ext"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", projectID, networkName)),
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
	_, err = firewallsClient.Insert(ctx, &computepb.InsertFirewallRequest{
		Project:          projectID,
		FirewallResource: sshRule,
	})
	if err != nil && !isAlreadyExistsError(err) {
		return fmt.Errorf("failed to create SSH firewall rule: %w", err)
	}

	// Allow all internal traffic
	internalRule := &computepb.Firewall{
		Name:      protoString("allow-internal"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", projectID, networkName)),
		Direction: protoString("INGRESS"),
		Priority:  protoInt32(1000),
		Allowed: []*computepb.Allowed{
			{IPProtocol: protoString("all")},
		},
		SourceRanges: []string{"10.10.0.0/20"},
		Description:  protoString("Allow all internal traffic"),
	}
	_, err = firewallsClient.Insert(ctx, &computepb.InsertFirewallRequest{
		Project:          projectID,
		FirewallResource: internalRule,
	})
	if err != nil && !isAlreadyExistsError(err) {
		return fmt.Errorf("failed to create internal firewall rule: %w", err)
	}

	// Allow all egress
	egressRule := &computepb.Firewall{
		Name:      protoString("allow-all-egress"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", projectID, networkName)),
		Direction: protoString("EGRESS"),
		Priority:  protoInt32(1000),
		Allowed: []*computepb.Allowed{
			{IPProtocol: protoString("all")},
		},
		DestinationRanges: []string{"0.0.0.0/0"},
		Description:       protoString("Allow all egress"),
	}
	_, err = firewallsClient.Insert(ctx, &computepb.InsertFirewallRequest{
		Project:          projectID,
		FirewallResource: egressRule,
	})
	if err != nil && !isAlreadyExistsError(err) {
		return fmt.Errorf("failed to create egress firewall rule: %w", err)
	}

	// Allow ingress for web (HTTP/HTTPS)
	webRule := &computepb.Firewall{
		Name:      protoString("allow-ingress-web"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", projectID, networkName)),
		Direction: protoString("INGRESS"),
		Priority:  protoInt32(1000),
		Allowed: []*computepb.Allowed{
			{IPProtocol: protoString("tcp"), Ports: []string{"80", "443"}},
		},
		SourceRanges: []string{"0.0.0.0/0"},
		Description:  protoString("Allow HTTP/HTTPS ingress"),
	}
	_, err = firewallsClient.Insert(ctx, &computepb.InsertFirewallRequest{
		Project:          projectID,
		FirewallResource: webRule,
	})
	if err != nil && !isAlreadyExistsError(err) {
		return fmt.Errorf("failed to create web ingress firewall rule: %w", err)
	}

	// Allow ingress for PostgreSQL
	postgresRule := &computepb.Firewall{
		Name:      protoString("allow-ingress-postgres"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", projectID, networkName)),
		Direction: protoString("INGRESS"),
		Priority:  protoInt32(1000),
		Allowed: []*computepb.Allowed{
			{IPProtocol: protoString("tcp"), Ports: []string{"5432"}},
		},
		SourceRanges: []string{"0.0.0.0/0"},
		TargetTags:   []string{"postgres"},
		Description:  protoString("Allow external access to PostgreSQL"),
	}
	_, err = firewallsClient.Insert(ctx, &computepb.InsertFirewallRequest{
		Project:          projectID,
		FirewallResource: postgresRule,
	})
	if err != nil && !isAlreadyExistsError(err) {
		return fmt.Errorf("failed to create postgres ingress firewall rule: %w", err)
	}

	fmt.Println("Firewall rules ensured")
	return nil
}

type VMDef struct {
	Name            string
	MachineType     string
	Tags            []string
	AdditionalDisks []int64
}

func (b *GCPBootstrapper) EnsureComputeInstances() error {
	projectID := b.env.ProjectID
	region := b.env.Region
	zone := b.env.Zone
	ctx := b.ctx

	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create instances client: %w", err)
	}
	defer instancesClient.Close()

	// Example VM definitions (expand as needed)

	vmDefs := []VMDef{
		{"jumpbox", "e2-medium", []string{"jumpbox", "ssh"}, []int64{}},
		{"postgres", "e2-medium", []string{"postgres"}, []int64{50}},
		{"ceph-1", "e2-standard-4", []string{"ceph"}, []int64{10, 100}},
		{"ceph-2", "e2-standard-4", []string{"ceph"}, []int64{10, 100}},
		{"ceph-3", "e2-standard-4", []string{"ceph"}, []int64{10, 100}},
		{"ceph-4", "e2-standard-4", []string{"ceph"}, []int64{10, 100}},
		{"k0s-1", "e2-standard-4", []string{"k0s"}, []int64{}},
		{"k0s-2", "e2-standard-4", []string{"k0s"}, []int64{}},
		{"k0s-3", "e2-standard-4", []string{"k0s"}, []int64{}},
	}

	network := fmt.Sprintf("projects/%s/global/networks/%s-vpc", projectID, projectID)
	subnetwork := fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s-%s-subnet", projectID, region, projectID, region)

	for _, vm := range vmDefs {
		disks := []*computepb.AttachedDisk{
			{
				Boot:       protoBool(true),
				AutoDelete: protoBool(true),
				Type:       protoString("PERSISTENT"),
				InitializeParams: &computepb.AttachedDiskInitializeParams{
					DiskSizeGb:  protoInt64(50),
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
				},
			})
		}

		instance := &computepb.Instance{
			Name:        protoString(vm.Name),
			MachineType: protoString(fmt.Sprintf("zones/%s/machineTypes/%s", zone, vm.MachineType)),
			Tags: &computepb.Tags{
				Items: vm.Tags,
			},
			NetworkInterfaces: []*computepb.NetworkInterface{
				{
					Network:    protoString(network),
					Subnetwork: protoString(subnetwork),
					AccessConfigs: []*computepb.AccessConfig{
						{
							Name: protoString("External NAT"),
							Type: protoString("ONE_TO_ONE_NAT"),
						},
					},
				},
			},
			Disks: disks,
			Metadata: &computepb.Metadata{
				Items: []*computepb.Items{
					{
						Key:   protoString("ssh-keys"),
						Value: protoString(fmt.Sprintf("root:%s\nubuntu:%s", readSSHKey(b.env.SSHPublicKeyPath), readSSHKey(b.env.SSHPublicKeyPath))),
					},
				},
			},
		}

		op, err := instancesClient.Insert(ctx, &computepb.InsertInstanceRequest{
			Project:          projectID,
			Zone:             zone,
			InstanceResource: instance,
		})
		if err != nil && !isAlreadyExistsError(err) {
			return fmt.Errorf("failed to create instance %s: %w", vm.Name, err)
		}
		if err == nil {
			if err := op.Wait(ctx); err != nil {
				return fmt.Errorf("failed to wait for instance %s creation: %w", vm.Name, err)
			}
		}
		fmt.Printf("Instance %s ensured\n", vm.Name)

		//find out the IP addresses of the created instance
		resp, err := instancesClient.Get(ctx, &computepb.GetInstanceRequest{
			Project:  projectID,
			Zone:     zone,
			Instance: vm.Name,
		})
		if err != nil {
			return fmt.Errorf("failed to get instance %s: %w", vm.Name, err)
		}
		externalIP := ""
		internalIP := ""
		if len(resp.GetNetworkInterfaces()) > 0 {
			internalIP = resp.GetNetworkInterfaces()[0].GetNetworkIP()
			if len(resp.GetNetworkInterfaces()[0].GetAccessConfigs()) > 0 {
				externalIP = resp.GetNetworkInterfaces()[0].GetAccessConfigs()[0].GetNatIP()
			}
		}

		node := node.Node{
			ExternalIP: externalIP,
			InternalIP: internalIP,
			Name:       vm.Name,
		}

		switch vm.Tags[0] {
		case "jumpbox":
			b.env.Jumpbox = node
		case "postgres":
			b.env.PostgreSQLNode = node
		case "ceph":
			b.env.CephNodes = append(b.env.CephNodes, node)
		case "k0s":
			b.env.ControlPlaneNodes = append(b.env.ControlPlaneNodes, node)
		}
	}

	return nil
}

func (b *GCPBootstrapper) EnsureRootLoginEnabled() error {
	hasRootLogin := b.env.Jumpbox.HasRootLoginEnabled(nil, b.NodeManager)
	if !hasRootLogin {
		err := b.env.Jumpbox.EnableRootLogin(nil, b.NodeManager)
		if err != nil {
			return fmt.Errorf("failed to enable root login on %s: %w", b.env.Jumpbox.Name, err)
		}
		fmt.Printf("Root login enabled on %s\n", b.env.Jumpbox.Name)
	}

	allNodes := append(b.env.ControlPlaneNodes, b.env.PostgreSQLNode)
	allNodes = append(allNodes, b.env.CephNodes...)

	for _, node := range allNodes {
		hasRootLogin := node.HasRootLoginEnabled(&b.env.Jumpbox, b.NodeManager)
		if hasRootLogin {
			fmt.Printf("Root login already enabled on %s\n", node.Name)
			continue
		}
		err := node.EnableRootLogin(&b.env.Jumpbox, b.NodeManager)
		if err != nil {
			return fmt.Errorf("failed to enable root login on %s: %w", node.Name, err)
		}

		fmt.Printf("Root login enabled on %s\n", node.Name)
	}
	return nil
}

func (b *GCPBootstrapper) EnsureJumpboxConfigured() error {
	if !b.env.Jumpbox.HasAcceptEnvConfigured(nil, b.NodeManager) {
		err := b.env.Jumpbox.ConfigureAcceptEnv(nil, b.NodeManager)
		if err != nil {
			return fmt.Errorf("failed to configure AcceptEnv on jumpbox: %w", err)
		}
	}
	hasOms := b.env.Jumpbox.HasCommand(b.NodeManager, "oms-cli")
	if hasOms {
		fmt.Println("OMS already installed on jumpbox")
		return nil
	}
	err := b.env.Jumpbox.InstallOms(b.NodeManager)
	if err != nil {
		return fmt.Errorf("failed to install OMS on jumpbox: %w", err)
	}

	fmt.Println("OMS installed on jumpbox")
	return nil
}

func (b *GCPBootstrapper) UpdateInstallConfig() error {
	// Update install config with necessary values
	b.InstallConfig.Datacenter.ID = b.env.DatacenterID
	b.InstallConfig.Datacenter.City = "Karlsruhe"
	b.InstallConfig.Datacenter.CountryCode = "DE"
	b.InstallConfig.Secrets.BaseDir = b.env.SecretsDir
	b.InstallConfig.Registry.ReplaceImagesInBom = true
	b.InstallConfig.Registry.LoadContainerImages = true

	b.InstallConfig.Postgres = files.PostgresConfig{
		Primary: &files.PostgresPrimaryConfig{
			IP:       b.env.PostgreSQLNode.InternalIP,
			Hostname: "postgres",
		},
	}

	b.InstallConfig.Ceph = files.CephConfig{
		CsiKubeletDir: "/var/lib/k0s/kubelet",
		NodesSubnet:   "10.10.0.0/20",
		Hosts: []files.CephHost{
			{
				Hostname:  "ceph-1",
				IsMaster:  true,
				IPAddress: b.env.CephNodes[0].InternalIP,
			},
			{
				Hostname:  "ceph-2",
				IPAddress: b.env.CephNodes[1].InternalIP,
			},
			{
				Hostname:  "ceph-3",
				IPAddress: b.env.CephNodes[2].InternalIP,
			},
			{
				Hostname:  "ceph-4",
				IPAddress: b.env.CephNodes[3].InternalIP,
			},
		},
		OSDs: []files.CephOSD{
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
		},
	}

	b.InstallConfig.Kubernetes = files.KubernetesConfig{
		ManagedByCodesphere: true,
		APIServerHost:       b.env.ControlPlaneNodes[0].InternalIP,
		ControlPlanes: []files.K8sNode{
			{
				IPAddress: b.env.ControlPlaneNodes[0].InternalIP,
			},
		},
		Workers: []files.K8sNode{
			{
				IPAddress: b.env.ControlPlaneNodes[0].InternalIP,
			},

			{
				IPAddress: b.env.ControlPlaneNodes[1].InternalIP,
			},
			{
				IPAddress: b.env.ControlPlaneNodes[2].InternalIP,
			},
		},
	}
	b.InstallConfig.Cluster.Monitoring = &files.MonitoringConfig{
		Prometheus: &files.PrometheusConfig{
			RemoteWrite: &files.RemoteWriteConfig{
				Enabled:     false,
				ClusterName: "GCP-test",
			},
		},
	}
	b.InstallConfig.Cluster.Gateway = files.GatewayConfig{
		ServiceType: "ClusterIP",
		IPAddresses: []string{b.env.ControlPlaneNodes[0].ExternalIP},
	}
	b.InstallConfig.Cluster.PublicGateway = files.GatewayConfig{
		ServiceType: "ClusterIP",
		IPAddresses: []string{b.env.ControlPlaneNodes[1].ExternalIP},
	}

	b.InstallConfig.Codesphere = files.CodesphereConfig{
		Domain:                     "cs." + b.env.BaseDomain,
		WorkspaceHostingBaseDomain: "ws." + b.env.BaseDomain,
		PublicIP:                   b.env.ControlPlaneNodes[1].ExternalIP,
		CustomDomains: files.CustomDomainsConfig{
			CNameBaseDomain: "ws." + b.env.BaseDomain,
		},
		DNSServers:  []string{"8.8.8.8"},
		Experiments: []string{},
		DeployConfig: files.DeployConfig{
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
							},
						},
					},
				},
			},
		},
		Plans: files.PlansConfig{
			HostingPlans: map[int]files.HostingPlan{
				1: {
					CPUTenth:      10,
					GPUParts:      0,
					MemoryMb:      2048,
					StorageMb:     20480,
					TempStorageMb: 1024,
				},
			},
			WorkspacePlans: map[int]files.WorkspacePlan{
				1: {
					Name:          "Standard Developer",
					HostingPlanID: 1,
					MaxReplicas:   3,
					OnDemand:      true,
				},
			},
		},
		GitProviders: &files.GitProvidersConfig{
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
				},
			},
		},
		ManagedServices: []files.ManagedServiceConfig{},
	}

	if err := b.icg.GenerateSecrets(); err != nil {
		return fmt.Errorf("failed to generate secrets: %w", err)
	}

	if err := b.icg.WriteInstallConfig(b.env.InstallConfig, true); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if err := b.icg.WriteVault(b.env.SecretsFile, true); err != nil {
		return fmt.Errorf("failed to write vault file: %w", err)
	}

	err := b.env.Jumpbox.CopyFile(b.NodeManager, b.env.InstallConfig, "/etc/codesphere/config.yaml")
	if err != nil {
		return fmt.Errorf("failed to copy install config to jumpbox: %w", err)
	}

	err = b.env.Jumpbox.CopyFile(b.NodeManager, b.env.SecretsFile, "/etc/codesphere/secrets/prod-vault.yaml")
	if err != nil {
		return fmt.Errorf("failed to copy secrets file to jumpbox: %w", err)
	}
	return nil
}

func (b *GCPBootstrapper) EnsureAgeKey() error {
	hasKey := b.env.Jumpbox.HasFile(nil, b.NodeManager, b.env.SecretsDir+"/age_key.txt")
	if hasKey {
		fmt.Println("Age key already present on jumpbox")
		return nil
	}

	err := b.env.Jumpbox.RunSSHCommand(nil, b.NodeManager, "root", fmt.Sprintf("mkdir -p %s; age-keygen -o %s/age_key.txt", b.env.SecretsDir, b.env.SecretsDir))
	if err != nil {
		return fmt.Errorf("failed to generate age key on jumpbox: %w", err)
	}

	fmt.Println("Age key generated on jumpbox")
	return nil
}

func (b *GCPBootstrapper) EncryptVault() error {
	err := b.env.Jumpbox.RunSSHCommand(nil, b.NodeManager, "root", "sops --encrypt --in-place --age $(age-keygen -y "+b.env.SecretsDir+"/age_key.txt) "+b.env.SecretsDir+"/prod-vault.yaml")
	if err != nil {
		return fmt.Errorf("failed to encrypt vault on jumpbox: %w", err)
	}

	fmt.Println("Vault encrypted on jumpbox")
	return nil
}

func (b *GCPBootstrapper) EnsureDNSRecords() error {
	ctx := context.Background()
	gcpProject := b.env.DNSProjectID
	if b.env.DNSProjectID == "" {
		gcpProject = b.env.ProjectID
	}

	dnsService, err := dns.NewService(ctx, option.WithCredentialsFile(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")))
	if err != nil {
		return fmt.Errorf("failed to create DNS service: %w", err)
	}

	zoneName := b.env.DNSZoneName
	// Check if zone exists, otherwise create
	_, err = dnsService.ManagedZones.Get(gcpProject, zoneName).Context(ctx).Do()
	if err != nil {
		zone := &dns.ManagedZone{
			Name:        zoneName,
			DnsName:     b.env.BaseDomain + ".",
			Description: "Codesphere DNS zone",
		}
		_, err = dnsService.ManagedZones.Create(gcpProject, zone).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("failed to create DNS zone: %w", err)
		}
	}

	records := []*dns.ResourceRecordSet{
		{
			Name:    fmt.Sprintf("cs.%s.", b.env.BaseDomain),
			Type:    "A",
			Ttl:     300,
			Rrdatas: []string{b.env.ControlPlaneNodes[0].ExternalIP},
		},
		{
			Name:    fmt.Sprintf("*.cs.%s.", b.env.BaseDomain),
			Type:    "A",
			Ttl:     300,
			Rrdatas: []string{b.env.ControlPlaneNodes[0].ExternalIP},
		},
		{
			Name:    fmt.Sprintf("*.ws.%s.", b.env.BaseDomain),
			Type:    "A",
			Ttl:     300,
			Rrdatas: []string{b.env.ControlPlaneNodes[0].ExternalIP},
		},
	}

	deletions := []*dns.ResourceRecordSet{}
	// Clean up existing records
	for _, record := range records {
		existingRecord, err := dnsService.ResourceRecordSets.Get(gcpProject, zoneName, record.Name, record.Type).Context(ctx).Do()
		if err == nil && existingRecord != nil {
			deletions = append(deletions, existingRecord)
		}
	}

	if len(deletions) > 0 {
		delChange := &dns.Change{
			Deletions: deletions,
		}
		_, err = dnsService.Changes.Create(gcpProject, zoneName, delChange).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("failed to delete DNS records: %w", err)
		}
	}

	change := &dns.Change{
		Additions: records,
	}

	_, err = dnsService.Changes.Create(gcpProject, zoneName, change).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to create DNS records: %w", err)
	}

	fmt.Printf("DNS records created in project %s zone %s\n", gcpProject, zoneName)
	return nil
}

// Helper functions
func protoString(s string) *string { return &s }
func protoBool(b bool) *bool       { return &b }
func protoInt32(i int32) *int32    { return &i }
func protoInt64(i int64) *int64    { return &i }
func isAlreadyExistsError(err error) bool {
	return status.Code(err) == codes.AlreadyExists || strings.Contains(err.Error(), "already exists")
}

// Helper to read SSH key file
func readSSHKey(path string) string {
	data, err := os.ReadFile(os.ExpandEnv(path))
	if err != nil {
		return ""
	}
	return string(data)
}
