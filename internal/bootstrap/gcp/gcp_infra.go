package gcp

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/github"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"google.golang.org/api/dns/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EnsureGCPInfrastructure creates and configures the necessary GCP infrastructure for the Codesphere installation.
func (b *GCPBootstrapper) EnsureGCPInfrastructure() error {
	err := b.stlog.Step("Ensure project", b.EnsureProject)
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

	err = b.stlog.Step("Ensure DNS records", b.EnsureDNSRecords)
	if err != nil {
		return fmt.Errorf("failed to ensure DNS records: %w", err)
	}
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

		projectTTL, err := time.ParseDuration(b.Env.ProjectTTL)
		if err != nil {
			return fmt.Errorf("invalid project TTL format: %w", err)
		}

		_, err = b.GCPClient.CreateProject(parent, projectId, b.Env.ProjectName, projectTTL)
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
				b.Time.Sleep(5 * time.Second)
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
	err := b.ensureIAMRoleWithRetry(b.Env.ProjectID, "cloud-controller", b.Env.ProjectID, []string{"roles/compute.admin"})
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

	err = b.ensureIAMRoleWithRetry(b.Env.ProjectID, "artifact-registry-writer", b.Env.ProjectID, []string{"roles/artifactregistry.writer"})
	return err
}

func (b *GCPBootstrapper) ensureIAMRoleWithRetry(projectID string, serviceAccount string, serviceAccountProjectID string, roles []string) error {
	var err error
	for retries := range 5 {
		err = b.GCPClient.AssignIAMRole(projectID, serviceAccount, serviceAccountProjectID, roles)
		if err == nil {
			return nil
		}
		if retries < 4 {
			b.stlog.LogRetry()
			b.Time.Sleep(5 * time.Second)
		}
	}
	return fmt.Errorf("failed to assign roles %v to service account %s: %w", roles, serviceAccount, err)
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

	rootDiskSize := int64(200)
	if b.Env.RegistryType == RegistryTypeGitHub {
		rootDiskSize = 50
	}
	sshKeys := ""
	var err error
	if b.Env.GitHubPAT != "" && b.Env.GitHubTeamOrg != "" && b.Env.GitHubTeamSlug != "" {
		sshKeys, err = github.GetSSHKeysFromGitHubTeam(b.GitHubClient, b.Env.GitHubTeamOrg, b.Env.GitHubTeamSlug)
		if err != nil {
			return fmt.Errorf("failed to get SSH keys from GitHub team: %w", err)
		}
	}

	pubKey, err := b.readSSHKey(b.Env.SSHPublicKeyPath)
	if err != nil {
		return err
	}

	sshKeys += fmt.Sprintf("root:%s\nubuntu:%s", pubKey+"root", pubKey+"ubuntu")

	// Create VMs in parallel
	wg := sync.WaitGroup{}
	errCh := make(chan error, len(vmDefs))
	resultCh := make(chan vmResult, len(vmDefs))
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
							Value: protoString(sshKeys),
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
