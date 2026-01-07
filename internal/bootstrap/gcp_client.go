package bootstrap

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"slices"

	artifact "cloud.google.com/go/artifactregistry/apiv1"
	artifactpb "cloud.google.com/go/artifactregistry/apiv1/artifactregistrypb"
	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"cloud.google.com/go/iam/apiv1/iampb"
	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	"cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"
	serviceusage "cloud.google.com/go/serviceusage/apiv1"
	"cloud.google.com/go/serviceusage/apiv1/serviceusagepb"
	"github.com/codesphere-cloud/oms/internal/util"
	"google.golang.org/api/cloudbilling/v1"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Interface for high-level GCP operations
type GCPClient interface {
	GetProjectByName(ctx context.Context, folderID string, displayName string) (*resourcemanagerpb.Project, error)
	CreateProject(ctx context.Context, parent, projectName, displayName string) (string, error)
	GetBillingInfo(projectID string) (*cloudbilling.ProjectBillingInfo, error)
	EnableBilling(ctx context.Context, projectID, billingAccount string) error
	EnableAPIs(ctx context.Context, projectID string, apis []string) error
	GetArtifactRegistry(ctx context.Context, projectID, region, repoName string) (*artifactpb.Repository, error)
	CreateArtifactRegistry(ctx context.Context, projectID, region, repoName string) (*artifactpb.Repository, error)
	CreateServiceAccount(ctx context.Context, projectID, name, displayName string) (string, bool, error)
	CreateServiceAccountKey(ctx context.Context, projectID, saEmail string) (string, error)
	AssignIAMRole(ctx context.Context, projectID, saEmail, role string) error
	CreateVPC(ctx context.Context, projectID, region, networkName, subnetName, routerName, natName string) error
	CreateFirewallRule(ctx context.Context, projectID string, rule *computepb.Firewall) error
}

// Concrete implementation
type RealGCPClient struct {
	CredentialsFile string
}

func NewGCPClient(credentialsFile string) *RealGCPClient {
	return &RealGCPClient{
		CredentialsFile: credentialsFile,
	}
}

func (c *RealGCPClient) GetProjectByName(ctx context.Context, folderID string, displayName string) (*resourcemanagerpb.Project, error) {
	client, err := resourcemanager.NewProjectsClient(ctx, option.WithCredentialsFile(c.CredentialsFile))
	if err != nil {
		return nil, err
	}
	defer util.IgnoreError(client.Close)
	req := &resourcemanagerpb.ListProjectsRequest{
		Parent:      fmt.Sprintf("folders/%s", folderID),
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

func (c *RealGCPClient) CreateProject(ctx context.Context, parent, projectID, displayName string) (string, error) {
	client, err := resourcemanager.NewProjectsClient(ctx, option.WithCredentialsFile(c.CredentialsFile))
	if err != nil {
		return "", err
	}
	defer util.IgnoreError(client.Close)
	project := &resourcemanagerpb.Project{
		ProjectId:   projectID,
		DisplayName: displayName,
		Parent:      parent,
	}
	op, err := client.CreateProject(ctx, &resourcemanagerpb.CreateProjectRequest{Project: project})
	if err != nil {
		return "", err
	}
	resp, err := op.Wait(ctx)
	if err != nil {
		return "", err
	}
	return resp.ProjectId, nil
}

func getProjectResourceName(projectID string) string {
	return fmt.Sprintf("projects/%s", projectID)
}

func (c *RealGCPClient) GetBillingInfo(projectID string) (*cloudbilling.ProjectBillingInfo, error) {
	projectName := getProjectResourceName(projectID)
	billingService, err := cloudbilling.NewService(context.Background(), option.WithCredentialsFile(c.CredentialsFile))
	if err != nil {
		return nil, err
	}
	billingInfo, err := billingService.Projects.GetBillingInfo(projectName).Do()
	if err != nil {
		return nil, err
	}
	return billingInfo, nil
}

func (c *RealGCPClient) EnableBilling(ctx context.Context, projectID, billingAccount string) error {
	billingService, err := cloudbilling.NewService(ctx, option.WithCredentialsFile(c.CredentialsFile))
	if err != nil {
		return err
	}
	projectName := getProjectResourceName(projectID)
	billingInfo := &cloudbilling.ProjectBillingInfo{
		BillingAccountName: fmt.Sprintf("billingAccounts/%s", billingAccount),
	}
	_, err = billingService.Projects.UpdateBillingInfo(projectName, billingInfo).Context(ctx).Do()
	return err
}

func (c *RealGCPClient) EnableAPIs(ctx context.Context, projectID string, apis []string) error {
	client, err := serviceusage.NewClient(ctx, option.WithCredentialsFile(c.CredentialsFile))
	if err != nil {
		return err
	}
	defer util.IgnoreError(client.Close)
	// enable APIs in parallel
	wg := sync.WaitGroup{}
	errCh := make(chan error, len(apis))
	for _, api := range apis {
		serviceName := fmt.Sprintf("projects/%s/services/%s", projectID, api)
		wg.Add(1)
		go func(serviceName, api string) {
			defer wg.Done()
			log.Printf("Enabling API %s\n", api)
			op, err := client.EnableService(ctx, &serviceusagepb.EnableServiceRequest{Name: serviceName})
			if status.Code(err) == codes.AlreadyExists {
				log.Printf("API %s already enabled\n", api)
				return
			}
			if err != nil {
				errCh <- fmt.Errorf("failed to enable API %s: %w", api, err)
			}
			if _, err := op.Wait(ctx); err != nil {
				errCh <- fmt.Errorf("failed to enable API %s: %w", api, err)
			}
			log.Printf("API %s enabled\n", api)
		}(serviceName, api)
	}
	wg.Wait()
	close(errCh)
	errStr := ""
	for err := range errCh {
		errStr += err.Error() + "; "
	}
	if len(errStr) > 0 {
		return fmt.Errorf("errors occurred while enabling APIs: %s", errStr)
	}
	return nil
}

func (c *RealGCPClient) CreateArtifactRegistry(ctx context.Context, projectID, region, repoName string) (*artifactpb.Repository, error) {
	client, err := artifact.NewClient(ctx, option.WithCredentialsFile(c.CredentialsFile))
	if err != nil {
		return nil, err
	}
	defer util.IgnoreError(client.Close)
	parent := fmt.Sprintf("projects/%s/locations/%s", projectID, region)
	repoReq := &artifactpb.CreateRepositoryRequest{
		Parent:       parent,
		RepositoryId: repoName,
		Repository: &artifactpb.Repository{
			Format:      artifactpb.Repository_DOCKER,
			Description: "Codesphere managed registry",
		},
	}
	op, err := client.CreateRepository(ctx, repoReq)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return nil, err
	}
	var repo *artifactpb.Repository
	if err == nil {
		repo, err = op.Wait(ctx)
		if err != nil {
			return nil, err
		}
	}
	return repo, nil
}

func (c *RealGCPClient) GetArtifactRegistry(ctx context.Context, projectID, region, repoName string) (*artifactpb.Repository, error) {
	fullRepoName := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", projectID, region, repoName)
	client, err := artifact.NewClient(ctx, option.WithCredentialsFile(c.CredentialsFile))
	if err != nil {
		return nil, err
	}
	defer util.IgnoreError(client.Close)
	repo, err := client.GetRepository(ctx, &artifactpb.GetRepositoryRequest{
		Name: fullRepoName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get artifact registry repository: %w", err)
	}
	return repo, nil
}

func (c *RealGCPClient) CreateServiceAccount(ctx context.Context, projectID, name, displayName string) (string, bool, error) {
	saMail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", name, projectID)
	iamService, err := iam.NewService(ctx, option.WithCredentialsFile(c.CredentialsFile))
	if err != nil {
		return saMail, false, err
	}
	saReq := &iam.CreateServiceAccountRequest{
		AccountId: name,
		ServiceAccount: &iam.ServiceAccount{
			DisplayName: displayName,
		},
	}
	_, err = iamService.Projects.ServiceAccounts.Create(fmt.Sprintf("projects/%s", projectID), saReq).Context(ctx).Do()
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return saMail, false, err
	}
	if err != nil && strings.Contains(err.Error(), "already exists") {
		return saMail, false, nil
	}
	return saMail, true, nil
}

func (c *RealGCPClient) CreateServiceAccountKey(ctx context.Context, projectID, saEmail string) (string, error) {
	iamService, err := iam.NewService(ctx, option.WithCredentialsFile(c.CredentialsFile))
	if err != nil {
		return "", err
	}
	keyReq := &iam.CreateServiceAccountKeyRequest{}
	saName := fmt.Sprintf("projects/%s/serviceAccounts/%s", projectID, saEmail)
	key, err := iamService.Projects.ServiceAccounts.Keys.Create(saName, keyReq).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	return string(key.PrivateKeyData), nil
}

func (c *RealGCPClient) AssignIAMRole(ctx context.Context, projectID, saName, role string) error {
	saEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", saName, projectID)
	client, err := resourcemanager.NewProjectsClient(ctx, option.WithCredentialsFile(c.CredentialsFile))
	if err != nil {
		return err
	}
	defer util.IgnoreError(client.Close)
	getReq := &iampb.GetIamPolicyRequest{
		Resource: fmt.Sprintf("projects/%s", projectID),
	}
	policy, err := client.GetIamPolicy(ctx, getReq)
	if err != nil {
		return err
	}
	member := fmt.Sprintf("serviceAccount:%s", saEmail)
	// Check if already assigned
	for _, binding := range policy.Bindings {
		if binding.Role == role {
			if slices.Contains(binding.Members, member) {
				return nil
			}
		}
	}
	policy.Bindings = append(policy.Bindings, &iampb.Binding{
		Role:    role,
		Members: []string{member},
	})
	setReq := &iampb.SetIamPolicyRequest{
		Resource: fmt.Sprintf("projects/%s", projectID),
		Policy:   policy,
	}
	_, err = client.SetIamPolicy(ctx, setReq)
	return err
}

func (c *RealGCPClient) CreateVPC(ctx context.Context, projectID, region, networkName, subnetName, routerName, natName string) error {
	networksClient, err := compute.NewNetworksRESTClient(ctx, option.WithCredentialsFile(c.CredentialsFile))
	if err != nil {
		return err
	}
	defer util.IgnoreError(networksClient.Close)
	network := &computepb.Network{
		Name:                  &networkName,
		AutoCreateSubnetworks: protoBool(false),
	}
	op, err := networksClient.Insert(ctx, &computepb.InsertNetworkRequest{
		Project:         projectID,
		NetworkResource: network,
	})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}
	if err == nil {
		if err := op.Wait(ctx); err != nil {
			return err
		}
	}
	subnetsClient, err := compute.NewSubnetworksRESTClient(ctx, option.WithCredentialsFile(c.CredentialsFile))
	if err != nil {
		return err
	}
	defer util.IgnoreError(subnetsClient.Close)
	subnet := &computepb.Subnetwork{
		Name:        &subnetName,
		IpCidrRange: protoString("10.10.0.0/20"),
		Region:      &region,
		Network:     protoString(fmt.Sprintf("projects/%s/global/networks/%s", projectID, networkName)),
	}
	op, err = subnetsClient.Insert(ctx, &computepb.InsertSubnetworkRequest{
		Project:            projectID,
		Region:             region,
		SubnetworkResource: subnet,
	})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}
	if err == nil {
		if err := op.Wait(ctx); err != nil {
			return err
		}
	}

	// Create Router
	routersClient, err := compute.NewRoutersRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create routers client: %w", err)
	}
	defer util.IgnoreError(routersClient.Close)

	router := &computepb.Router{
		Name:    &routerName,
		Region:  &region,
		Network: protoString(fmt.Sprintf("projects/%s/global/networks/%s", projectID, networkName)),
	}
	op, err = routersClient.Insert(ctx, &computepb.InsertRouterRequest{
		Project:        projectID,
		Region:         region,
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
	defer util.IgnoreError(natsClient.Close)

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
		Region:  region,
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

func (c *RealGCPClient) CreateFirewallRule(ctx context.Context, projectID string, rule *computepb.Firewall) error {
	firewallsClient, err := compute.NewFirewallsRESTClient(ctx, option.WithCredentialsFile(c.CredentialsFile))
	if err != nil {
		return err
	}
	defer util.IgnoreError(firewallsClient.Close)
	_, err = firewallsClient.Insert(ctx, &computepb.InsertFirewallRequest{
		Project:          projectID,
		FirewallResource: rule,
	})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}
	return nil
}

// Helper functions
func protoString(s string) *string { return &s }
func protoBool(b bool) *bool       { return &b }
