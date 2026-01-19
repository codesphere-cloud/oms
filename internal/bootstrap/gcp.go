// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/dns/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GCPBootstrapper struct {
	ctx           context.Context
	env           *CodesphereEnvironment
	InstallConfig *files.RootConfig
	Secrets       *files.InstallVault
	icg           installer.InstallConfigManager
	NodeManager   *node.NodeManager
	GCPClient     GCPClient
}

type CodesphereEnvironment struct {
	ProjectID                string      `json:"project_id"`
	ProjectName              string      `json:"project_name"`
	DNSProjectID             string      `json:"dns_project_id"`
	PostgreSQLNode           node.Node   `json:"postgresql_node"`
	ControlPlaneNodes        []node.Node `json:"control_plane_nodes"`
	CephNodes                []node.Node `json:"ceph_nodes"`
	Jumpbox                  node.Node   `json:"jumpbox"`
	ContainerRegistryURL     string      `json:"container_registry_url"`
	ExistingConfigUsed       bool        `json:"existing_config_used"`
	InstallCodesphereVersion string      `json:"install_codesphere_version"`
	Preemptible              bool        `json:"preemptible"`
	WriteConfig              bool        `json:"write_config"`
	GatewayIP                string      `json:"gateway_ip"`
	PublicGatewayIP          string      `json:"public_gateway_ip"`

	ProjectDisplayName    string
	BillingAccount        string
	BaseDomain            string
	GithubAppClientID     string
	GithubAppClientSecret string
	SecretsDir            string
	FolderID              string
	SSHPublicKeyPath      string
	SSHPrivateKeyPath     string
	DatacenterID          int
	CustomPgIP            string
	InstallConfig         string
	SecretsFile           string
	Region                string
	Zone                  string
	DNSZoneName           string
}

func NewGCPBootstrapper(env env.Env, CodesphereEnv *CodesphereEnvironment, gcpClient GCPClient) (*GCPBootstrapper, error) {
	ctx := context.Background()
	fw := util.NewFilesystemWriter()
	icg := installer.NewInstallConfigManager()
	nm := &node.NodeManager{
		FileIO:  fw,
		KeyPath: CodesphereEnv.SSHPrivateKeyPath,
	}

	if fw.Exists(CodesphereEnv.InstallConfig) {
		log.Printf("Reading install config file: %s", CodesphereEnv.InstallConfig)
		err := icg.LoadInstallConfigFromFile(CodesphereEnv.InstallConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}

		CodesphereEnv.ExistingConfigUsed = true
	} else {
		err := icg.ApplyProfile("dev")
		if err != nil {
			return nil, fmt.Errorf("failed to apply profile: %w", err)
		}
	}

	if fw.Exists(CodesphereEnv.SecretsFile) {
		log.Printf("Reading vault file: %s", CodesphereEnv.SecretsFile)
		err := icg.LoadVaultFromFile(CodesphereEnv.SecretsFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load vault file: %w", err)
		}

		log.Println("Merging vault secrets into configuration...")
		err = icg.MergeVaultIntoConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to merge vault into config: %w", err)
		}
	}

	return &GCPBootstrapper{
		env:           CodesphereEnv,
		InstallConfig: icg.GetInstallConfig(),
		NodeManager:   nm,
		Secrets:       icg.GetVault(),
		ctx:           ctx,
		icg:           icg,
		GCPClient:     gcpClient,
	}, nil
}

func (b *GCPBootstrapper) Bootstrap() (*CodesphereEnvironment, error) {
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

	err = b.EnsureGatewayIPAddresses()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure external IP addresses: %w", err)
	}

	err = b.EnsureRootLoginEnabled()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure root login is enabled: %w", err)
	}

	err = b.EnsureJumpboxConfigured()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure jumpbox is configured: %w", err)
	}

	err = b.EnsureHostsConfigured()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure hosts are configured: %w", err)
	}

	if b.env.WriteConfig {
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
	}

	err = b.EnsureDNSRecords()
	if err != nil {
		return b.env, fmt.Errorf("failed to ensure DNS records: %w", err)
	}

	if b.env.InstallCodesphereVersion != "" {
		err = b.InstallCodesphere()
		if err != nil {
			return b.env, fmt.Errorf("failed to install Codesphere: %w", err)
		}
	}

	err = b.GenerateK0sConfigScript()
	if err != nil {
		return b.env, fmt.Errorf("failed to generate k0s config script: %w", err)
	}

	return b.env, nil
}

func (b *GCPBootstrapper) EnsureProject() error {
	parent := ""
	if b.env.FolderID != "" {
		parent = fmt.Sprintf("folders/%s", b.env.FolderID)
	}

	// Generate a unique project ID
	projectGuid := strings.ToLower(shortuuid.New()[:8])
	projectId := b.env.ProjectName + "-" + projectGuid

	existingProject, err := b.GCPClient.GetProjectByName(b.ctx, b.env.FolderID, b.env.ProjectName)
	if err == nil {
		b.env.ProjectID = existingProject.ProjectId
		b.env.ProjectName = existingProject.Name
		return nil
	}
	if err.Error() == fmt.Sprintf("project not found: %s", b.env.ProjectName) {
		_, err := b.GCPClient.CreateProject(b.ctx, parent, projectId, b.env.ProjectName)
		if err != nil {
			return fmt.Errorf("failed to create project: %w", err)
		}
		b.env.ProjectID = projectId
		return nil
	}
	return fmt.Errorf("failed to get project: %w", err)
}

func (b *GCPBootstrapper) EnsureBilling() error {
	bi, err := b.GCPClient.GetBillingInfo(b.env.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get billing info: %w", err)
	}
	if bi.BillingEnabled && bi.BillingAccountName == b.env.BillingAccount {
		return nil
	}

	err = b.GCPClient.EnableBilling(b.ctx, b.env.ProjectID, b.env.BillingAccount)
	if err != nil {
		return fmt.Errorf("failed to enable billing: %w", err)
	}
	log.Printf("Billing enabled for project %s with account %s", b.env.ProjectID, b.env.BillingAccount)

	return nil
}

func (b *GCPBootstrapper) EnsureAPIsEnabled() error {
	apis := []string{
		"compute.googleapis.com",
		"serviceusage.googleapis.com",
		"artifactregistry.googleapis.com",
		"dns.googleapis.com",
	}

	err := b.GCPClient.EnableAPIs(b.ctx, b.env.ProjectID, apis)
	if err != nil {
		return fmt.Errorf("failed to enable APIs: %w", err)
	}

	log.Printf("Required APIs enabled for project %s", b.env.ProjectID)

	return nil
}

func (b *GCPBootstrapper) EnsureArtifactRegistry() error {
	repoName := "codesphere-registry"

	repo, err := b.GCPClient.GetArtifactRegistry(b.ctx, b.env.ProjectID, b.env.Region, repoName)
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("failed to get artifact registry: %w", err)
	}

	// Create the repository if it doesn't exist
	if repo == nil {
		repo, err = b.GCPClient.CreateArtifactRegistry(b.ctx, b.env.ProjectID, b.env.Region, repoName)
		if err != nil || repo == nil {
			return fmt.Errorf("failed to create artifact registry: %w, repo: %v", err, repo)
		}
	}

	b.InstallConfig.Registry.Server = repo.GetRegistryUri()

	log.Printf("Artifact Registry repository %s ensured", b.InstallConfig.Registry.Server)

	return nil
}

func (b *GCPBootstrapper) EnsureServiceAccounts() error {
	_, _, err := b.EnsureServiceAccount("cloud-controller")
	if err != nil {
		return err
	}
	sa, newSa, err := b.EnsureServiceAccount("artifact-registry-writer")
	if err != nil {
		return err
	}

	if !newSa && b.InstallConfig.Registry.Password != "" {
		return nil
	}

	for retries := range 5 {
		privateKey, err := b.GCPClient.CreateServiceAccountKey(b.ctx, b.env.ProjectID, sa)

		if err != nil && status.Code(err) != codes.AlreadyExists {
			if retries > 3 {
				return fmt.Errorf("failed to create service account key: %w", err)
			}

			log.Printf("got response %d trying to create service account key for %s, retrying...", status.Code(err), sa)

			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("Service account key for %s ensured", sa)
		b.InstallConfig.Registry.Password = string(privateKey)
		b.InstallConfig.Registry.Username = "_json_key_base64"

		break
	}

	return nil
}

func (b *GCPBootstrapper) EnsureServiceAccount(name string) (string, bool, error) {
	return b.GCPClient.CreateServiceAccount(b.ctx, b.env.ProjectID, name, name)
}

func (b *GCPBootstrapper) EnsureIAMRoles() error {
	err := b.GCPClient.AssignIAMRole(b.ctx, b.env.ProjectID, "artifact-registry-writer", "roles/artifactregistry.writer")
	if err != nil {
		return err
	}
	err = b.GCPClient.AssignIAMRole(b.ctx, b.env.ProjectID, "cloud-controller", "roles/compute.admin")
	return err
}

func (b *GCPBootstrapper) EnsureVPC() error {
	networkName := fmt.Sprintf("%s-vpc", b.env.ProjectID)
	subnetName := fmt.Sprintf("%s-%s-subnet", b.env.ProjectID, b.env.Region)
	routerName := fmt.Sprintf("%s-router", b.env.ProjectID)
	natName := fmt.Sprintf("%s-nat-gateway", b.env.ProjectID)

	// Create VPC
	err := b.GCPClient.CreateVPC(b.ctx, b.env.ProjectID, b.env.Region, networkName, subnetName, routerName, natName)
	if err != nil {
		return fmt.Errorf("failed to ensure VPC: %w", err)
	}

	log.Printf("VPC %s ensured", networkName)

	return nil
}

func (b *GCPBootstrapper) EnsureFirewallRules() error {
	networkName := fmt.Sprintf("%s-vpc", b.env.ProjectID)

	// Allow external SSH to Jumpbox
	sshRule := &computepb.Firewall{
		Name:      protoString("allow-ssh-ext"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", b.env.ProjectID, networkName)),
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
	err := b.GCPClient.CreateFirewallRule(b.ctx, b.env.ProjectID, sshRule)
	if err != nil {
		return fmt.Errorf("failed to create jumpbox ssh firewall rule: %w", err)
	}

	// Allow all internal traffic
	internalRule := &computepb.Firewall{
		Name:      protoString("allow-internal"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", b.env.ProjectID, networkName)),
		Direction: protoString("INGRESS"),
		Priority:  protoInt32(1000),
		Allowed: []*computepb.Allowed{
			{IPProtocol: protoString("all")},
		},
		SourceRanges: []string{"10.10.0.0/20"},
		Description:  protoString("Allow all internal traffic"),
	}
	err = b.GCPClient.CreateFirewallRule(b.ctx, b.env.ProjectID, internalRule)
	if err != nil {
		return fmt.Errorf("failed to create internal firewall rule: %w", err)
	}

	// Allow all egress
	egressRule := &computepb.Firewall{
		Name:      protoString("allow-all-egress"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", b.env.ProjectID, networkName)),
		Direction: protoString("EGRESS"),
		Priority:  protoInt32(1000),
		Allowed: []*computepb.Allowed{
			{IPProtocol: protoString("all")},
		},
		DestinationRanges: []string{"0.0.0.0/0"},
		Description:       protoString("Allow all egress"),
	}
	err = b.GCPClient.CreateFirewallRule(b.ctx, b.env.ProjectID, egressRule)
	if err != nil {
		return fmt.Errorf("failed to create egress firewall rule: %w", err)
	}

	// Allow ingress for web (HTTP/HTTPS)
	webRule := &computepb.Firewall{
		Name:      protoString("allow-ingress-web"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", b.env.ProjectID, networkName)),
		Direction: protoString("INGRESS"),
		Priority:  protoInt32(1000),
		Allowed: []*computepb.Allowed{
			{IPProtocol: protoString("tcp"), Ports: []string{"80", "443"}},
		},
		SourceRanges: []string{"0.0.0.0/0"},
		Description:  protoString("Allow HTTP/HTTPS ingress"),
	}
	err = b.GCPClient.CreateFirewallRule(b.ctx, b.env.ProjectID, webRule)
	if err != nil {
		return fmt.Errorf("failed to create web firewall rule: %w", err)
	}

	// Allow ingress for PostgreSQL
	postgresRule := &computepb.Firewall{
		Name:      protoString("allow-ingress-postgres"),
		Network:   protoString(fmt.Sprintf("projects/%s/global/networks/%s", b.env.ProjectID, networkName)),
		Direction: protoString("INGRESS"),
		Priority:  protoInt32(1000),
		Allowed: []*computepb.Allowed{
			{IPProtocol: protoString("tcp"), Ports: []string{"5432"}},
		},
		SourceRanges: []string{"0.0.0.0/0"},
		TargetTags:   []string{"postgres"},
		Description:  protoString("Allow external access to PostgreSQL"),
	}
	err = b.GCPClient.CreateFirewallRule(b.ctx, b.env.ProjectID, postgresRule)
	if err != nil {
		return fmt.Errorf("failed to create postgres firewall rule: %w", err)
	}

	log.Println("Firewall rules ensured")
	return nil
}

type VMDef struct {
	Name            string
	MachineType     string
	Tags            []string
	AdditionalDisks []int64
	ExternalIP      bool
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
	defer util.IgnoreError(instancesClient.Close)

	// Example VM definitions (expand as needed)

	vmDefs := []VMDef{
		{"jumpbox", "e2-medium", []string{"jumpbox", "ssh"}, []int64{}, true},
		{"postgres", "e2-medium", []string{"postgres"}, []int64{50}, true},
		{"ceph-1", "e2-standard-8", []string{"ceph"}, []int64{20, 200}, false},
		{"ceph-2", "e2-standard-8", []string{"ceph"}, []int64{20, 200}, false},
		{"ceph-3", "e2-standard-8", []string{"ceph"}, []int64{20, 200}, false},
		{"ceph-4", "e2-standard-8", []string{"ceph"}, []int64{20, 200}, false},
		{"k0s-1", "e2-standard-16", []string{"k0s"}, []int64{}, false},
		{"k0s-2", "e2-standard-16", []string{"k0s"}, []int64{}, false},
		{"k0s-3", "e2-standard-16", []string{"k0s"}, []int64{}, false},
	}

	network := fmt.Sprintf("projects/%s/global/networks/%s-vpc", projectID, projectID)
	subnetwork := fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s-%s-subnet", projectID, region, projectID, region)
	diskType := fmt.Sprintf("projects/%s/zones/%s/diskTypes/pd-ssd", projectID, zone)

	// Create VMs in parallel
	wg := sync.WaitGroup{}
	errCh := make(chan error, len(vmDefs))
	mu := sync.Mutex{}
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
						DiskSizeGb:  protoInt64(200),
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
					Preemptible: &b.env.Preemptible,
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
							Value: protoString(fmt.Sprintf("root:%s\nubuntu:%s", readSSHKey(b.env.SSHPublicKeyPath)+"root", readSSHKey(b.env.SSHPublicKeyPath)) + "ubuntu"),
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

			op, err := instancesClient.Insert(ctx, &computepb.InsertInstanceRequest{
				Project:          projectID,
				Zone:             zone,
				InstanceResource: instance,
			})
			if err != nil && !isAlreadyExistsError(err) {
				errCh <- fmt.Errorf("failed to create instance %s: %w", vm.Name, err)
			}
			if err == nil {
				if err := op.Wait(ctx); err != nil {
					errCh <- fmt.Errorf("failed to wait for instance %s creation: %w", vm.Name, err)
				}
			}
			log.Printf("Instance %s ensured", vm.Name)

			//find out the IP addresses of the created instance
			resp, err := instancesClient.Get(ctx, &computepb.GetInstanceRequest{
				Project:  projectID,
				Zone:     zone,
				Instance: vm.Name,
			})
			if err != nil {
				errCh <- fmt.Errorf("failed to get instance %s: %w", vm.Name, err)
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

			mu.Lock()
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
			mu.Unlock()
		}(vm)
	}
	wg.Wait()

	close(errCh)
	errStr := ""
	for err := range errCh {
		errStr += err.Error() + "; "
	}
	if errStr != "" {
		return fmt.Errorf("error ensuring compute instances: %s", errStr)
	}

	//sort ceph nodes by name to ensure consistent ordering
	sort.Slice(b.env.CephNodes, func(i, j int) bool {
		return b.env.CephNodes[i].Name < b.env.CephNodes[j].Name
	})

	//sort control plane nodes by name to ensure consistent ordering
	sort.Slice(b.env.ControlPlaneNodes, func(i, j int) bool {
		return b.env.ControlPlaneNodes[i].Name < b.env.ControlPlaneNodes[j].Name
	})
	return nil
}

// EnsureGatewayIPAddresses reserves 2 static external IP addresses for the ingress
// controllers of the cluster.
func (b *GCPBootstrapper) EnsureGatewayIPAddresses() error {
	var err error
	b.env.GatewayIP, err = b.EnsureExternalIP("gateway")
	if err != nil {
		return fmt.Errorf("failed to ensure gateway IP: %w", err)
	}
	b.env.PublicGatewayIP, err = b.EnsureExternalIP("public-gateway")
	if err != nil {
		return fmt.Errorf("failed to ensure public gateway IP: %w", err)
	}
	return nil
}

func (b *GCPBootstrapper) EnsureExternalIP(name string) (string, error) {
	addressesClient, err := compute.NewAddressesRESTClient(b.ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create addresses client: %w", err)
	}
	defer util.IgnoreError(addressesClient.Close)

	desiredAddress := &computepb.Address{
		Name:        &name,
		AddressType: protoString("EXTERNAL"),
		Region:      &b.env.Region,
	}

	// Figure out if address already exists and get IP
	req := &computepb.GetAddressRequest{
		Project: b.env.ProjectID,
		Region:  b.env.Region,
		Address: *desiredAddress.Name,
	}

	address, err := addressesClient.Get(b.ctx, req)

	if err == nil && address != nil {
		log.Printf("Address %s already exists", name)

		return address.GetAddress(), nil
	}

	op, err := addressesClient.Insert(b.ctx, &computepb.InsertAddressRequest{
		Project:         b.env.ProjectID,
		Region:          b.env.Region,
		AddressResource: desiredAddress,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create address %s: %w", name, err)
	}
	if err := op.Wait(b.ctx); err != nil {
		return "", fmt.Errorf("failed to wait for address %s creation: %w", name, err)
	}
	log.Printf("Address %s ensured", name)

	address, err = addressesClient.Get(b.ctx, req)

	if err == nil && address != nil {
		return address.GetAddress(), nil
	}
	return "", fmt.Errorf("failed to get address %s after creation", name)
}

func (b *GCPBootstrapper) EnsureRootLoginEnabled() error {
	// wait for SSH service to be available on jumpbox
	err := b.env.Jumpbox.WaitForSSH(nil, b.NodeManager, 30*time.Second)
	if err != nil {
		return fmt.Errorf("timed out waiting for SSH service to start on jumpbox: %w", err)
	}

	hasRootLogin := b.env.Jumpbox.HasRootLoginEnabled(nil, b.NodeManager)
	if !hasRootLogin {
		err := b.env.Jumpbox.EnableRootLogin(nil, b.NodeManager)
		if err != nil {
			return fmt.Errorf("failed to enable root login on %s: %w", b.env.Jumpbox.Name, err)
		}
		log.Printf("Root login enabled on %s", b.env.Jumpbox.Name)
	}

	allNodes := append(b.env.ControlPlaneNodes, b.env.PostgreSQLNode)
	allNodes = append(allNodes, b.env.CephNodes...)

	for _, node := range allNodes {
		err = node.WaitForSSH(&b.env.Jumpbox, b.NodeManager, 30*time.Second)
		if err != nil {
			return fmt.Errorf("timed out waiting for SSH service to start on %s: %w", node.Name, err)
		}
		hasRootLogin := node.HasRootLoginEnabled(&b.env.Jumpbox, b.NodeManager)
		if hasRootLogin {
			log.Printf("Root login already enabled on %s", node.Name)

			continue
		}
		for i := range 3 {
			err := node.EnableRootLogin(&b.env.Jumpbox, b.NodeManager)
			if err == nil {
				break
			}
			if i == 2 {
				return fmt.Errorf("failed to enable root login on %s: %w", node.Name, err)
			}
			log.Printf("cannot enable root login on %s yet, retrying in 10 seconds: %v", node.Name, err)
			time.Sleep(10 * time.Second)
		}

		log.Printf("Root login enabled on %s", node.Name)
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
		log.Println("OMS already installed on jumpbox")
		return nil
	}
	err := b.env.Jumpbox.InstallOms(b.NodeManager)
	if err != nil {
		return fmt.Errorf("failed to install OMS on jumpbox: %w", err)
	}

	log.Println("OMS installed on jumpbox")
	return nil
}

func (b *GCPBootstrapper) EnsureHostsConfigured() error {
	allNodes := append(b.env.ControlPlaneNodes, b.env.PostgreSQLNode)
	allNodes = append(allNodes, b.env.CephNodes...)

	for _, node := range allNodes {
		if !node.HasInotifyWatchesConfigured(&b.env.Jumpbox, b.NodeManager) {
			err := node.ConfigureInotifyWatches(&b.env.Jumpbox, b.NodeManager)
			if err != nil {
				return fmt.Errorf("failed to configure inotify watches on %s: %w", node.Name, err)
			}
		}

		if !node.HasMemoryMapConfigured(&b.env.Jumpbox, b.NodeManager) {
			err := node.ConfigureMemoryMap(&b.env.Jumpbox, b.NodeManager)
			if err != nil {
				return fmt.Errorf("failed to configure memory map on %s: %w", node.Name, err)
			}
		}
		log.Printf("Host %s configured", node.Name)
	}
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

	if b.InstallConfig.Postgres.Primary == nil {
		b.InstallConfig.Postgres.Primary = &files.PostgresPrimaryConfig{
			Hostname: b.env.PostgreSQLNode.Name,
		}
	}
	b.InstallConfig.Postgres.Primary.IP = b.env.PostgreSQLNode.InternalIP

	b.InstallConfig.Ceph.CsiKubeletDir = "/var/lib/k0s/kubelet"
	b.InstallConfig.Ceph.NodesSubnet = "10.10.0.0/20"
	b.InstallConfig.Ceph.Hosts = []files.CephHost{
		{
			Hostname:  b.env.CephNodes[0].Name,
			IsMaster:  true,
			IPAddress: b.env.CephNodes[0].InternalIP,
		},
		{
			Hostname:  b.env.CephNodes[1].Name,
			IPAddress: b.env.CephNodes[1].InternalIP,
		},
		{
			Hostname:  b.env.CephNodes[2].Name,
			IPAddress: b.env.CephNodes[2].InternalIP,
		},
		{
			Hostname:  b.env.CephNodes[3].Name,
			IPAddress: b.env.CephNodes[3].InternalIP,
		},
	}
	b.InstallConfig.Ceph.OSDs = []files.CephOSD{
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
		ServiceType: "LoadBalancer",
		//IPAddresses: []string{b.env.ControlPlaneNodes[0].ExternalIP},
		Annotations: map[string]string{
			"cloud.google.com/load-balancer-ipv4": b.env.GatewayIP,
		},
	}
	b.InstallConfig.Cluster.PublicGateway = files.GatewayConfig{
		ServiceType: "LoadBalancer",
		Annotations: map[string]string{
			"cloud.google.com/load-balancer-ipv4": b.env.PublicGatewayIP,
		},
		//IPAddresses: []string{b.env.ControlPlaneNodes[1].ExternalIP},
	}

	b.InstallConfig.Codesphere.Domain = "cs." + b.env.BaseDomain
	b.InstallConfig.Codesphere.WorkspaceHostingBaseDomain = "ws." + b.env.BaseDomain
	b.InstallConfig.Codesphere.PublicIP = b.env.ControlPlaneNodes[1].ExternalIP
	b.InstallConfig.Codesphere.CustomDomains = files.CustomDomainsConfig{
		CNameBaseDomain: "ws." + b.env.BaseDomain,
	}
	b.InstallConfig.Codesphere.DNSServers = []string{"8.8.8.8"}
	b.InstallConfig.Codesphere.Experiments = []string{}
	b.InstallConfig.Codesphere.DeployConfig = files.DeployConfig{
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
							4: 0,
						},
					},
				},
			},
		},
	}
	b.InstallConfig.Codesphere.Plans = files.PlansConfig{
		HostingPlans: map[int]files.HostingPlan{
			1: {
				CPUTenth:      10,
				GPUParts:      0,
				MemoryMb:      2048,
				StorageMb:     20480,
				TempStorageMb: 1024,
			},
			2: {
				CPUTenth:      20,
				GPUParts:      0,
				MemoryMb:      4096,
				StorageMb:     20480,
				TempStorageMb: 1024,
			},
			3: {
				CPUTenth:      40,
				GPUParts:      0,
				MemoryMb:      8192,
				StorageMb:     40960,
				TempStorageMb: 1024,
			},
			4: {
				CPUTenth:      80,
				GPUParts:      0,
				MemoryMb:      16384,
				StorageMb:     40960,
				TempStorageMb: 1024,
			},
		},
		WorkspacePlans: map[int]files.WorkspacePlan{
			1: {
				Name:          "Micro",
				HostingPlanID: 1,
				MaxReplicas:   3,
				OnDemand:      true,
			},
			2: {
				Name:          "Standard",
				HostingPlanID: 2,
				MaxReplicas:   3,
				OnDemand:      true,
			},
			3: {
				Name:          "Big",
				HostingPlanID: 3,
				MaxReplicas:   3,
				OnDemand:      true,
			},
			4: {
				Name:          "Pro",
				HostingPlanID: 4,
				MaxReplicas:   3,
				OnDemand:      true,
			},
		},
	}
	b.InstallConfig.Codesphere.GitProviders = &files.GitProvidersConfig{
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

				ClientID:     b.env.GithubAppClientID,
				ClientSecret: b.env.GithubAppClientSecret,
			},
		},
	}
	b.InstallConfig.Codesphere.ManagedServices = []files.ManagedServiceConfig{}

	if !b.env.ExistingConfigUsed {
		err := b.icg.GenerateSecrets()
		if err != nil {
			return fmt.Errorf("failed to generate secrets: %w", err)
		}
	} else {
		var err error
		b.InstallConfig.Postgres.Primary.PrivateKey, b.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem, err = installer.GenerateServerCertificate(
			b.InstallConfig.Postgres.CaCertPrivateKey,
			b.InstallConfig.Postgres.CACertPem,
			b.InstallConfig.Postgres.Primary.Hostname,
			[]string{b.InstallConfig.Postgres.Primary.IP})
		if err != nil {
			return fmt.Errorf("failed to generate primary server certificate: %w", err)
		}
		if b.InstallConfig.Postgres.Replica != nil {
			b.InstallConfig.Postgres.ReplicaPrivateKey, b.InstallConfig.Postgres.Replica.SSLConfig.ServerCertPem, err = installer.GenerateServerCertificate(
				b.InstallConfig.Postgres.CaCertPrivateKey,
				b.InstallConfig.Postgres.CACertPem,
				b.InstallConfig.Postgres.Replica.Name,
				[]string{b.InstallConfig.Postgres.Replica.IP})
			if err != nil {
				return fmt.Errorf("failed to generate replica server certificate: %w", err)
			}
		}
	}

	if err := b.icg.WriteInstallConfig(b.env.InstallConfig, true); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if err := b.icg.WriteVault(b.env.SecretsFile, true); err != nil {
		return fmt.Errorf("failed to write vault file: %w", err)
	}

	err := b.env.Jumpbox.CopyFile(nil, b.NodeManager, b.env.InstallConfig, "/etc/codesphere/config.yaml")
	if err != nil {
		return fmt.Errorf("failed to copy install config to jumpbox: %w", err)
	}

	err = b.env.Jumpbox.CopyFile(nil, b.NodeManager, b.env.SecretsFile, b.env.SecretsDir+"/prod.vault.yaml")
	if err != nil {
		return fmt.Errorf("failed to copy secrets file to jumpbox: %w", err)
	}
	return nil
}

func (b *GCPBootstrapper) EnsureAgeKey() error {
	hasKey := b.env.Jumpbox.HasFile(nil, b.NodeManager, b.env.SecretsDir+"/age_key.txt")
	if hasKey {
		log.Println("Age key already present on jumpbox")
		return nil
	}

	err := b.env.Jumpbox.RunSSHCommand(nil, b.NodeManager, "root", fmt.Sprintf("mkdir -p %s; age-keygen -o %s/age_key.txt", b.env.SecretsDir, b.env.SecretsDir))
	if err != nil {
		return fmt.Errorf("failed to generate age key on jumpbox: %w", err)
	}

	log.Println("Age key generated on jumpbox")
	return nil
}

func (b *GCPBootstrapper) EncryptVault() error {
	err := b.env.Jumpbox.RunSSHCommand(nil, b.NodeManager, "root", "cp "+b.env.SecretsDir+"/prod.vault.yaml{,.bak}")
	if err != nil {
		return fmt.Errorf("failed backup vault on jumpbox: %w", err)
	}

	err = b.env.Jumpbox.RunSSHCommand(nil, b.NodeManager, "root", "sops --encrypt --in-place --age $(age-keygen -y "+b.env.SecretsDir+"/age_key.txt) "+b.env.SecretsDir+"/prod.vault.yaml")
	if err != nil {
		return fmt.Errorf("failed to encrypt vault on jumpbox: %w", err)
	}

	log.Println("Vault encrypted on jumpbox")
	return nil
}

func (b *GCPBootstrapper) EnsureDNSRecords() error {
	ctx := context.Background()
	gcpProject := b.env.DNSProjectID
	if b.env.DNSProjectID == "" {
		gcpProject = b.env.ProjectID
	}

	dnsService, err := dns.NewService(ctx)
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
			Rrdatas: []string{b.env.GatewayIP},
		},
		{
			Name:    fmt.Sprintf("*.cs.%s.", b.env.BaseDomain),
			Type:    "A",
			Ttl:     300,
			Rrdatas: []string{b.env.GatewayIP},
		},
		{
			Name:    fmt.Sprintf("*.ws.%s.", b.env.BaseDomain),
			Type:    "A",
			Ttl:     300,
			Rrdatas: []string{b.env.PublicGatewayIP},
		},
		{
			Name:    fmt.Sprintf("ws.%s.", b.env.BaseDomain),
			Type:    "A",
			Ttl:     300,
			Rrdatas: []string{b.env.PublicGatewayIP},
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

	log.Printf("DNS records created in project %s zone %s", gcpProject, zoneName)
	return nil
}

func (b *GCPBootstrapper) InstallCodesphere() error {
	err := b.env.Jumpbox.RunSSHCommand(nil, b.NodeManager, "root", "oms-cli download package "+b.env.InstallCodesphereVersion)
	if err != nil {
		return fmt.Errorf("failed to download Codesphere package from jumpbox: %w", err)
	}

	err = b.env.Jumpbox.RunSSHCommand(nil, b.NodeManager, "root", "oms-cli install codesphere -c /etc/codesphere/config.yaml -k "+b.env.SecretsDir+"/age_key.txt -p "+b.env.InstallCodesphereVersion+".tar.gz")
	if err != nil {
		return fmt.Errorf("failed to install Codesphere from jumpbox: %w", err)
	}

	log.Println("Codesphere installed from jumpbox")
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
$KUBECTL patch svc public-gateway-controller -n codesphere -p '{"spec": {"loadBalancerIP": "'` + b.env.PublicGatewayIP + `'"}}'
$KUBECTL patch svc gateway-controller -n codesphere -p '{"spec": {"loadBalancerIP": "'` + b.env.GatewayIP + `'"}}'

sed -i 's/k0scontroller/k0scontroller --enable-cloud-provider/g' /etc/systemd/system/k0scontroller.service

ssh root@` + b.env.ControlPlaneNodes[1].InternalIP + ` "sed -i 's/k0sworker/k0sworker --enable-cloud-provider/g' /etc/systemd/system/k0sworker.service; systemctl daemon-reload; systemctl restart k0sworker"

ssh root@` + b.env.ControlPlaneNodes[2].InternalIP + ` "sed -i 's/k0sworker/k0sworker --enable-cloud-provider/g' /etc/systemd/system/k0sworker.service; systemctl daemon-reload; systemctl restart k0sworker"

systemctl daemon-reload
systemctl restart k0scontroller
`
	// Probably we need to enable the cloud provider plugin in k0s configuration.
	// --enable-cloud-provider on worker nodes systemd file /etc/systemd/system/k0sworker.service
	// in addition on the first node: /etc/systemd/system/k0scontroller.service the flag --enable-cloud-provider

	err := os.WriteFile("configure-k0s.sh", []byte(script), 0755)
	if err != nil {
		return fmt.Errorf("failed to write configure-k0s.sh: %w", err)
	}
	err = b.env.ControlPlaneNodes[0].CopyFile(&b.env.Jumpbox, b.NodeManager, "configure-k0s.sh", "/root/configure-k0s.sh")
	if err != nil {
		return fmt.Errorf("failed to copy configure-k0s.sh to control plane node: %w", err)
	}
	err = b.env.ControlPlaneNodes[0].RunSSHCommand(&b.env.Jumpbox, b.NodeManager, "root", "chmod +x /root/configure-k0s.sh")
	if err != nil {
		return fmt.Errorf("failed to make configure-k0s.sh executable on control plane node: %w", err)
	}
	return nil
}

// Helper functions
func protoInt32(i int32) *int32 { return &i }
func protoInt64(i int64) *int64 { return &i }
func isAlreadyExistsError(err error) bool {
	return status.Code(err) == codes.AlreadyExists || strings.Contains(err.Error(), "already exists")
}

// Helper to read SSH key file
func readSSHKey(path string) string {
	data, err := os.ReadFile(os.ExpandEnv(path))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
