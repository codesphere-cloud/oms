// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"context"
	"fmt"
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
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/cloudbilling/v1"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Interface for high-level GCP operations
type GCPClientManager interface {
	GetProjectByName(folderID string, displayName string) (*resourcemanagerpb.Project, error)
	CreateProjectID(projectName string) string
	CreateProject(parent, projectName, displayName string) (string, error)
	DeleteProject(projectID string) error
	IsOMSManagedProject(projectID string) (bool, error)
	GetBillingInfo(projectID string) (*cloudbilling.ProjectBillingInfo, error)
	EnableBilling(projectID, billingAccount string) error
	EnableAPIs(projectID string, apis []string) error
	GetArtifactRegistry(projectID, region, repoName string) (*artifactpb.Repository, error)
	CreateArtifactRegistry(projectID, region, repoName string) (*artifactpb.Repository, error)
	CreateServiceAccount(projectID, name, displayName string) (string, bool, error)
	CreateServiceAccountKey(projectID, saEmail string) (string, error)
	AssignIAMRole(projectID, saEmail string, saProjectID string, roles []string) error
	GrantImpersonation(impersonatingServiceAccount, impersonatingProjectID, imperonatedServiceAccount, impersonatedProjectID string) error
	RevokeImpersonation(impersonatingServiceAccount, impersonatingProjectID, impersonatedServiceAccount, impersonatedProjectID string) error
	CreateVPC(projectID, region, networkName, subnetName, routerName, natName string) error
	CreateFirewallRule(projectID string, rule *computepb.Firewall) error
	CreateInstance(projectID, zone string, instance *computepb.Instance) error
	GetInstance(projectID, zone, instanceName string) (*computepb.Instance, error)
	CreateAddress(projectID, region string, address *computepb.Address) (string, error)
	GetAddress(projectID, region, addressName string) (*computepb.Address, error)
	EnsureDNSManagedZone(projectID, zoneName, dnsName, description string) error
	EnsureDNSRecordSets(projectID, zoneName string, records []*dns.ResourceRecordSet) error
	DeleteDNSRecordSets(projectID, zoneName, baseDomain string) error
}

// Concrete implementation
type GCPClient struct {
	ctx             context.Context
	st              *bootstrap.StepLogger
	CredentialsFile string
}

func NewGCPClient(ctx context.Context, st *bootstrap.StepLogger, credentialsFile string) *GCPClient {
	return &GCPClient{
		ctx:             ctx,
		st:              st,
		CredentialsFile: credentialsFile,
	}
}

// GetProjectByName retrieves a GCP project by its display name within the specified folder.
func (c *GCPClient) GetProjectByName(folderID string, displayName string) (*resourcemanagerpb.Project, error) {
	client, err := resourcemanager.NewProjectsClient(c.ctx)
	if err != nil {
		return nil, err
	}
	defer util.IgnoreError(client.Close)

	req := &resourcemanagerpb.ListProjectsRequest{
		Parent:      fmt.Sprintf("folders/%s", folderID),
		ShowDeleted: false,
	}

	it := client.ListProjects(c.ctx, req)

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

// CreateProjectID generates a unique project ID based on the given project name.
func (c *GCPClient) CreateProjectID(projectName string) string {
	projectGuid := strings.ToLower(shortuuid.New()[:8])
	return projectName + "-" + projectGuid
}

// CreateProject creates a new GCP project under the specified parent (folder or organization).
// It returns the project ID of the newly created project.
// The project is labeled with 'oms-managed=true' to identify it as created by OMS.
func (c *GCPClient) CreateProject(parent, projectID, displayName string) (string, error) {
	client, err := resourcemanager.NewProjectsClient(c.ctx)
	if err != nil {
		return "", err
	}
	defer util.IgnoreError(client.Close)

	project := &resourcemanagerpb.Project{
		ProjectId:   projectID,
		DisplayName: displayName,
		Parent:      parent,
		Labels: map[string]string{
			OMSManagedLabel: "true",
		},
	}
	op, err := client.CreateProject(c.ctx, &resourcemanagerpb.CreateProjectRequest{Project: project})
	if err != nil {
		return "", err
	}
	resp, err := op.Wait(c.ctx)
	if err != nil {
		return "", err
	}

	return resp.ProjectId, nil
}

// DeleteProject deletes the specified GCP project.
func (c *GCPClient) DeleteProject(projectID string) error {
	client, err := resourcemanager.NewProjectsClient(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to create resource manager client: %w", err)
	}
	defer util.IgnoreError(client.Close)

	op, err := client.DeleteProject(c.ctx, &resourcemanagerpb.DeleteProjectRequest{
		Name: getProjectResourceName(projectID),
	})
	if err != nil {
		return fmt.Errorf("failed to initiate project deletion: %w", err)
	}

	if _, err = op.Wait(c.ctx); err != nil {
		return fmt.Errorf("failed to wait for project deletion: %w", err)
	}

	return nil
}

// IsOMSManagedProject checks if the given project was created by OMS by verifying the 'oms-managed' label.
func (c *GCPClient) IsOMSManagedProject(projectID string) (bool, error) {
	client, err := resourcemanager.NewProjectsClient(c.ctx)
	if err != nil {
		return false, fmt.Errorf("failed to create resource manager client: %w", err)
	}
	defer util.IgnoreError(client.Close)

	project, err := client.GetProject(c.ctx, &resourcemanagerpb.GetProjectRequest{
		Name: getProjectResourceName(projectID),
	})
	if err != nil {
		return false, fmt.Errorf("failed to get project: %w", err)
	}

	return CheckOMSManagedLabel(project.Labels), nil
}

func getProjectResourceName(projectID string) string {
	return fmt.Sprintf("projects/%s", projectID)
}

// GetBillingInfo retrieves the billing information for the given project.
func (c *GCPClient) GetBillingInfo(projectID string) (*cloudbilling.ProjectBillingInfo, error) {
	billingService, err := cloudbilling.NewService(context.Background())
	if err != nil {
		return nil, err
	}

	projectName := getProjectResourceName(projectID)
	billingInfo, err := billingService.Projects.GetBillingInfo(projectName).Do()
	if err != nil {
		return nil, err
	}
	return billingInfo, nil
}

// EnableBilling enables billing for the given project using the specified billing account.
func (c *GCPClient) EnableBilling(projectID, billingAccount string) error {
	billingService, err := cloudbilling.NewService(c.ctx)
	if err != nil {
		return err
	}

	projectName := getProjectResourceName(projectID)
	billingInfo := &cloudbilling.ProjectBillingInfo{
		BillingAccountName: fmt.Sprintf("billingAccounts/%s", billingAccount),
	}
	_, err = billingService.Projects.UpdateBillingInfo(projectName, billingInfo).Context(c.ctx).Do()
	return err
}

// EnableAPIs enables the specified APIs for the given project.
func (c *GCPClient) EnableAPIs(projectID string, apis []string) error {
	client, err := serviceusage.NewClient(c.ctx)
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
			c.st.Logf("Enabling API %s", api)

			op, err := client.EnableService(c.ctx, &serviceusagepb.EnableServiceRequest{Name: serviceName})
			if status.Code(err) == codes.AlreadyExists {
				c.st.Logf("API %s already enabled", api)
				return
			}
			if err != nil {
				errCh <- fmt.Errorf("failed to enable API %s: %w", api, err)
				return
			}
			if _, err := op.Wait(c.ctx); err != nil {
				errCh <- fmt.Errorf("failed to enable API %s: %w", api, err)
				return
			}

			c.st.Logf("API %s enabled", api)
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

// CreateArtifactRegistry creates and returns an Artifact Registry repository by its name.
func (c *GCPClient) CreateArtifactRegistry(projectID, region, repoName string) (*artifactpb.Repository, error) {
	client, err := artifact.NewClient(c.ctx)
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
	op, err := client.CreateRepository(c.ctx, repoReq)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return nil, err
	}
	var repo *artifactpb.Repository
	if err == nil {
		_, err = op.Wait(c.ctx)
		if err != nil {
			return nil, err
		}
	}

	// get repo again to ensure all infos are stored, else e.g. uri would be missing
	repo, err = c.GetArtifactRegistry(projectID, region, repoName)
	if err != nil {
		return nil, fmt.Errorf("failed to get newly created artifact registry: %w", err)
	}

	return repo, nil
}

// GetArtifactRegistry retrieves an existing Artifact Registry repository by its name.
func (c *GCPClient) GetArtifactRegistry(projectID, region, repoName string) (*artifactpb.Repository, error) {
	client, err := artifact.NewClient(c.ctx)
	if err != nil {
		return nil, err
	}
	defer util.IgnoreError(client.Close)

	fullRepoName := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", projectID, region, repoName)
	repo, err := client.GetRepository(c.ctx, &artifactpb.GetRepositoryRequest{
		Name: fullRepoName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get artifact registry repository: %w", err)
	}

	return repo, nil
}

// CreateServiceAccount creates a new service account with the given name and display name.
// It returns the email of the created service account, a boolean indicating whether the account was newly created,
// and an error if any occurred during the process.
func (c *GCPClient) CreateServiceAccount(projectID, name, displayName string) (string, bool, error) {
	saMail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", name, projectID)
	iamService, err := iam.NewService(c.ctx)
	if err != nil {
		return saMail, false, err
	}

	saReq := &iam.CreateServiceAccountRequest{
		AccountId: name,
		ServiceAccount: &iam.ServiceAccount{
			DisplayName: displayName,
		},
	}
	_, err = iamService.Projects.ServiceAccounts.Create(fmt.Sprintf("projects/%s", projectID), saReq).Context(c.ctx).Do()
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return saMail, false, err
	}
	if err != nil && strings.Contains(err.Error(), "already exists") {
		return saMail, false, nil
	}

	return saMail, true, nil
}

// CreateServiceAccountKey creates a new key for the specified service account.
// It returns the private key data in PEM format and an error if any occurred during the process.
func (c *GCPClient) CreateServiceAccountKey(projectID, saEmail string) (string, error) {
	iamService, err := iam.NewService(c.ctx)
	if err != nil {
		return "", err
	}

	keyReq := &iam.CreateServiceAccountKeyRequest{}
	saName := fmt.Sprintf("projects/%s/serviceAccounts/%s", projectID, saEmail)
	key, err := iamService.Projects.ServiceAccounts.Keys.Create(saName, keyReq).Context(c.ctx).Do()
	if err != nil {
		return "", err
	}

	return string(key.PrivateKeyData), nil
}

// AssignIAMRole assigns the specified IAM role to a service account in a project.
func (c *GCPClient) AssignIAMRole(projectID, saName string, saProjectID string, roles []string) error {
	saEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", saName, saProjectID)
	member := fmt.Sprintf("serviceAccount:%s", saEmail)
	resource := fmt.Sprintf("projects/%s", projectID)
	return c.addRoleBindingToProject(member, roles, resource)
}

func (c *GCPClient) addRoleBindingToProject(member string, roles []string, resource string) error {
	client, err := resourcemanager.NewProjectsClient(c.ctx)
	if err != nil {
		return err
	}
	defer util.IgnoreError(client.Close)

	getReq := &iampb.GetIamPolicyRequest{
		Resource: resource,
	}

	policy, err := client.GetIamPolicy(c.ctx, getReq)
	if err != nil {
		return err
	}

	// Add role bindings to policy
	updated := false
	for _, role := range roles {
		bindingExists := false
		for _, binding := range policy.Bindings {
			if binding.Role == role {
				if !slices.Contains(binding.Members, member) {
					binding.Members = append(binding.Members, member)
					updated = true
				}
				bindingExists = true
				break
			}
		}
		if bindingExists {
			continue
		}

		// Assign role
		policy.Bindings = append(policy.Bindings, &iampb.Binding{
			Role:    role,
			Members: []string{member},
		})
		updated = true
	}

	if !updated {
		return nil
	}

	setReq := &iampb.SetIamPolicyRequest{
		Resource: resource,
		Policy:   policy,
	}
	_, err = client.SetIamPolicy(c.ctx, setReq)
	return err
}

// Types between ServiceAccount and Project IAM API differ, so we need a separate function
func (c *GCPClient) addRoleBindingToServiceAccount(member string, roles []string, resource string) error {
	iamService, err := iam.NewService(c.ctx)
	if err != nil {
		return err
	}

	// Get current policy
	policy, err := iamService.Projects.ServiceAccounts.GetIamPolicy(resource).Context(c.ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to get IAM policy for service account: %w", err)
	}

	// Add role bindings directly to iam.Policy
	updated := false
	for _, role := range roles {
		bindingExists := false
		for _, binding := range policy.Bindings {
			if binding.Role == role {
				if !slices.Contains(binding.Members, member) {
					binding.Members = append(binding.Members, member)
					updated = true
				}
				bindingExists = true
				break
			}
		}
		if bindingExists {
			continue
		}

		// Assign role
		policy.Bindings = append(policy.Bindings, &iam.Binding{
			Role:    role,
			Members: []string{member},
		})
		updated = true
	}

	if !updated {
		return nil
	}

	// Set the updated policy
	setReq := &iam.SetIamPolicyRequest{
		Policy: policy,
	}
	_, err = iamService.Projects.ServiceAccounts.SetIamPolicy(resource, setReq).Context(c.ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to set IAM policy for service account: %w", err)
	}

	return nil
}

// GrantImpersonation grants the "roles/iam.serviceAccountTokenCreator" role to the impersonating service account on the impersonated service account,
// allowing the impersonating service account to generate access tokens for the impersonated service account, which is necessary for cross-project impersonation.
func (c *GCPClient) GrantImpersonation(impersonatingServiceAccount, impersonatingProjectID, impersonatedServiceAccount, impersonatedProjectID string) error {
	impersonatingSAEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", impersonatingServiceAccount, impersonatingProjectID)
	impersonatedSAEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", impersonatedServiceAccount, impersonatedProjectID)

	resourceName := fmt.Sprintf("projects/%s/serviceAccounts/%s", impersonatedProjectID, impersonatedSAEmail)
	member := fmt.Sprintf("serviceAccount:%s", impersonatingSAEmail)

	return c.addRoleBindingToServiceAccount(member, []string{"roles/iam.serviceAccountTokenCreator"}, resourceName)
}

// RevokeImpersonation revokes the "roles/iam.serviceAccountTokenCreator" role from the impersonating service account on the impersonated service account.
// This removes the cross-project impersonation permission that was previously granted.
func (c *GCPClient) RevokeImpersonation(impersonatingServiceAccount, impersonatingProjectID, impersonatedServiceAccount, impersonatedProjectID string) error {
	impersonatingSAEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", impersonatingServiceAccount, impersonatingProjectID)
	impersonatedSAEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", impersonatedServiceAccount, impersonatedProjectID)

	resourceName := fmt.Sprintf("projects/%s/serviceAccounts/%s", impersonatedProjectID, impersonatedSAEmail)
	member := fmt.Sprintf("serviceAccount:%s", impersonatingSAEmail)

	return c.removeRoleBindingFromServiceAccount(member, []string{"roles/iam.serviceAccountTokenCreator"}, resourceName)
}

// removeRoleBindingFromServiceAccount removes the specified role bindings for a member from a service account's IAM policy.
func (c *GCPClient) removeRoleBindingFromServiceAccount(member string, roles []string, resource string) error {
	iamService, err := iam.NewService(c.ctx)
	if err != nil {
		return err
	}

	policy, err := iamService.Projects.ServiceAccounts.GetIamPolicy(resource).Context(c.ctx).Do()
	if err != nil {
		if IsNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("failed to get IAM policy for service account: %w", err)
	}

	updated := false
	for _, role := range roles {
		for i, binding := range policy.Bindings {
			if binding.Role == role {
				// Find and remove the member from this binding
				for j, m := range binding.Members {
					if m == member {
						binding.Members = append(binding.Members[:j], binding.Members[j+1:]...)
						updated = true
						break
					}
				}
				// If the binding has no more members, remove it entirely
				if len(binding.Members) == 0 {
					policy.Bindings = append(policy.Bindings[:i], policy.Bindings[i+1:]...)
				}
				break
			}
		}
	}

	if !updated {
		return nil
	}

	setReq := &iam.SetIamPolicyRequest{
		Policy: policy,
	}
	_, err = iamService.Projects.ServiceAccounts.SetIamPolicy(resource, setReq).Context(c.ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to set IAM policy for service account: %w", err)
	}

	return nil
}

// CreateVPC creates a VPC network with the specified subnet, router, and NAT gateway.
func (c *GCPClient) CreateVPC(projectID, region, networkName, subnetName, routerName, natName string) error {
	// Create Network
	networksClient, err := compute.NewNetworksRESTClient(c.ctx)
	if err != nil {
		return err
	}
	defer util.IgnoreError(networksClient.Close)

	network := &computepb.Network{
		Name:                  &networkName,
		AutoCreateSubnetworks: protoBool(false),
	}
	op, err := networksClient.Insert(c.ctx, &computepb.InsertNetworkRequest{
		Project:         projectID,
		NetworkResource: network,
	})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}
	if err == nil {
		if err := op.Wait(c.ctx); err != nil {
			return err
		}
	}

	c.st.Logf("Network %s ensured", networkName)

	// Create Subnet
	subnetsClient, err := compute.NewSubnetworksRESTClient(c.ctx)
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
	op, err = subnetsClient.Insert(c.ctx, &computepb.InsertSubnetworkRequest{
		Project:            projectID,
		Region:             region,
		SubnetworkResource: subnet,
	})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}
	if err == nil {
		if err := op.Wait(c.ctx); err != nil {
			return err
		}
	}

	c.st.Logf("Subnetwork %s ensured", subnetName)

	// Create Router
	routersClient, err := compute.NewRoutersRESTClient(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to create routers client: %w", err)
	}
	defer util.IgnoreError(routersClient.Close)

	router := &computepb.Router{
		Name:    &routerName,
		Region:  &region,
		Network: protoString(fmt.Sprintf("projects/%s/global/networks/%s", projectID, networkName)),
	}
	op, err = routersClient.Insert(c.ctx, &computepb.InsertRouterRequest{
		Project:        projectID,
		Region:         region,
		RouterResource: router,
	})
	if err != nil && !isAlreadyExistsError(err) {
		return fmt.Errorf("failed to create router: %w", err)
	}
	if err == nil {
		if err := op.Wait(c.ctx); err != nil {
			return fmt.Errorf("failed to wait for router creation: %w", err)
		}
	}

	c.st.Logf("Router %s ensured", routerName)

	// Create NAT Gateway
	natsClient, err := compute.NewRoutersRESTClient(c.ctx)
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
	_, err = routersClient.Patch(c.ctx, &computepb.PatchRouterRequest{
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

	c.st.Logf("NAT gateway %s ensured", natName)

	return nil
}

// CreateFirewallRule creates a firewall rule in the specified project.
func (c *GCPClient) CreateFirewallRule(projectID string, rule *computepb.Firewall) error {
	firewallsClient, err := compute.NewFirewallsRESTClient(c.ctx)
	if err != nil {
		return err
	}
	defer util.IgnoreError(firewallsClient.Close)

	_, err = firewallsClient.Insert(c.ctx, &computepb.InsertFirewallRequest{
		Project:          projectID,
		FirewallResource: rule,
	})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}

	return nil
}

// CreateInstance creates a new Compute Engine instance in the specified project and zone.
func (c *GCPClient) CreateInstance(projectID, zone string, instance *computepb.Instance) error {
	client, err := compute.NewInstancesRESTClient(c.ctx)
	if err != nil {
		return err
	}
	defer util.IgnoreError(client.Close)

	op, err := client.Insert(c.ctx, &computepb.InsertInstanceRequest{
		Project:          projectID,
		Zone:             zone,
		InstanceResource: instance,
	})
	if err != nil {
		return err
	}

	return op.Wait(c.ctx)
}

// GetInstance retrieves a Compute Engine instance by its name in the specified project and zone.
func (c *GCPClient) GetInstance(projectID, zone, instanceName string) (*computepb.Instance, error) {
	client, err := compute.NewInstancesRESTClient(c.ctx)
	if err != nil {
		return nil, err
	}
	defer util.IgnoreError(client.Close)

	return client.Get(c.ctx, &computepb.GetInstanceRequest{
		Project:  projectID,
		Zone:     zone,
		Instance: instanceName,
	})
}

// CreateAddress creates a new static IP address in the specified project and region.
func (c *GCPClient) CreateAddress(projectID, region string, address *computepb.Address) (string, error) {
	client, err := compute.NewAddressesRESTClient(c.ctx)
	if err != nil {
		return "", err
	}
	defer util.IgnoreError(client.Close)

	op, err := client.Insert(c.ctx, &computepb.InsertAddressRequest{
		Project:         projectID,
		Region:          region,
		AddressResource: address,
	})
	if err != nil {
		return "", err
	}
	if err = op.Wait(c.ctx); err != nil {
		return "", err
	}

	// Fetch the created address to get the IP
	createdAddress, err := client.Get(c.ctx, &computepb.GetAddressRequest{
		Project: projectID,
		Region:  region,
		Address: *address.Name,
	})
	if err != nil {
		return "", err
	}

	return *createdAddress.Address, nil
}

// GetAddress retrieves a static IP address by its name in the specified project and region.
func (c *GCPClient) GetAddress(projectID, region, addressName string) (*computepb.Address, error) {
	client, err := compute.NewAddressesRESTClient(c.ctx)
	if err != nil {
		return nil, err
	}
	defer util.IgnoreError(client.Close)

	return client.Get(c.ctx, &computepb.GetAddressRequest{
		Project: projectID,
		Region:  region,
		Address: addressName,
	})
}

// EnsureDNSManagedZone ensures that a DNS managed zone exists in the specified project.
func (c *GCPClient) EnsureDNSManagedZone(projectID, zoneName, dnsName, description string) error {
	service, err := dns.NewService(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to create DNS service: %w", err)
	}

	// Check if zone exists
	_, err = service.ManagedZones.Get(projectID, zoneName).Context(c.ctx).Do()
	if err == nil {
		// Zone exists
		return nil
	}

	// Create zone
	zone := &dns.ManagedZone{
		Name:        zoneName,
		DnsName:     dnsName,
		Description: description,
	}
	_, err = service.ManagedZones.Create(projectID, zone).Context(c.ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to create DNS zone: %w", err)
	}

	return nil
}

// EnsureDNSRecordSets ensures that the specified DNS record sets exist in the given managed zone.
func (c *GCPClient) EnsureDNSRecordSets(projectID, zoneName string, records []*dns.ResourceRecordSet) error {
	service, err := dns.NewService(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to create DNS service: %w", err)
	}

	deletions := []*dns.ResourceRecordSet{}
	// Clean up existing records
	for _, record := range records {
		existingRecord, err := service.ResourceRecordSets.Get(projectID, zoneName, record.Name, record.Type).Context(c.ctx).Do()
		if err == nil && existingRecord != nil {
			deletions = append(deletions, existingRecord)
		}
	}

	if len(deletions) > 0 {
		delChange := &dns.Change{
			Deletions: deletions,
		}
		_, err = service.Changes.Create(projectID, zoneName, delChange).Context(c.ctx).Do()
		if err != nil {
			return fmt.Errorf("failed to delete existing DNS records: %w", err)
		}
	}

	change := &dns.Change{
		Additions: records,
	}
	_, err = service.Changes.Create(projectID, zoneName, change).Context(c.ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to create DNS records: %w", err)
	}

	return nil
}

// DeleteDNSRecordSets deletes DNS record sets created by OMS for the given base domain.
func (c *GCPClient) DeleteDNSRecordSets(projectID, zoneName, baseDomain string) error {
	service, err := dns.NewService(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to create DNS service: %w", err)
	}

	var deletions []*dns.ResourceRecordSet
	for _, record := range GetDNSRecordNames(baseDomain) {
		existing, err := service.ResourceRecordSets.Get(projectID, zoneName, record.Name, record.Rtype).Context(c.ctx).Do()
		if IsNotFoundError(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to get DNS record %s: %w", record.Name, err)
		}
		deletions = append(deletions, existing)
	}

	if len(deletions) == 0 {
		return nil
	}

	if _, err = service.Changes.Create(projectID, zoneName, &dns.Change{Deletions: deletions}).Context(c.ctx).Do(); err != nil {
		return fmt.Errorf("failed to delete DNS records: %w", err)
	}
	return nil
}

// Helper functions
func protoString(s string) *string { return &s }
func protoBool(b bool) *bool       { return &b }
func protoInt32(i int32) *int32    { return &i }
func protoInt64(i int64) *int64    { return &i }
