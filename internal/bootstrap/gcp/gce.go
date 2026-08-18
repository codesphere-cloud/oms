// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/codesphere-cloud/oms/internal/bootstrap/datacenter"
	"github.com/codesphere-cloud/oms/internal/github"
	"github.com/codesphere-cloud/oms/internal/installer/node"
	"github.com/codesphere-cloud/oms/internal/util"
)

type VMDef struct {
	Name            string
	MachineType     string
	Tags            []string
	AdditionalDisks []int64
	ExternalIP      bool
	// DataCenterID is the data center the VM belongs to, or 0 for the project-shared VMs
	// (jumpbox and postgres) that every data center uses.
	DataCenterID int
}

// cephNodesPerDataCenter and k0sNodesPerDataCenter are the per-data-center node counts. Three
// Ceph nodes are the minimum for replication; three k0s nodes give one control plane and three
// workers, as written into the install config.
const (
	cephNodesPerDataCenter = 3
	k0sNodesPerDataCenter  = 3
)

// sharedVMDefs returns the VMs that exist once per project, regardless of how many data centers
// are bootstrapped. The postgres node hosts the database both data centers share.
func sharedVMDefs() []VMDef {
	return []VMDef{
		{Name: "jumpbox", MachineType: "e2-medium", Tags: []string{"jumpbox", "ssh"}, AdditionalDisks: []int64{}, ExternalIP: true},
		{Name: "postgres", MachineType: "e2-standard-2", Tags: []string{"postgres"}, AdditionalDisks: []int64{}, ExternalIP: true},
	}
}

// dataCenterVMDefs returns the Ceph and k0s VMs of one data center. The suffix is empty for the
// primary data center, so single-DC bootstraps keep the names ceph-1..3 and k0s-1..3.
func dataCenterVMDefs(dcID int, suffix string) []VMDef {
	defs := make([]VMDef, 0, cephNodesPerDataCenter+k0sNodesPerDataCenter)
	for i := 1; i <= cephNodesPerDataCenter; i++ {
		defs = append(defs, VMDef{
			Name:            fmt.Sprintf("ceph-%d%s", i, suffix),
			MachineType:     "e2-standard-8",
			Tags:            []string{"ceph"},
			AdditionalDisks: []int64{10, 100},
			DataCenterID:    dcID,
		})
	}

	for i := 1; i <= k0sNodesPerDataCenter; i++ {
		defs = append(defs, VMDef{
			Name:            fmt.Sprintf("k0s-%d%s", i, suffix),
			MachineType:     "e2-standard-8",
			Tags:            []string{"k0s"},
			AdditionalDisks: []int64{},
			DataCenterID:    dcID,
		})
	}

	return defs
}

// VMDefsForEnv returns every VM definition of the environment: the project-shared VMs plus the
// Ceph and k0s VMs of each data center. When the environment carries no data centers — as with
// an infra file written before multi-DC support — it falls back to a single unsuffixed one.
func VMDefsForEnv(env *CodesphereEnvironment) []VMDef {
	defs := sharedVMDefs()

	dcs := env.DataCenters
	if len(dcs) == 0 {
		dcs = []*datacenter.DataCenter{{ID: datacenter.PrimaryID}}
	}

	for _, dc := range dcs {
		defs = append(defs, dataCenterVMDefs(dc.ID, dc.Suffix)...)
	}

	return defs
}

// validateVMProvisioningOptions checks that spot and preemptible options are not both set
func (b *GCPBootstrapper) validateVMProvisioningOptions() error {
	if b.Env.SpotVMs && b.Env.Preemptible {
		return fmt.Errorf("cannot specify both --spot-vms and --preemptible flags; use --spot-vms for the newer spot VM model")
	}
	return nil
}

type vmResult struct {
	vmType     string // jumpbox, postgres, ceph, k0s
	name       string
	externalIP string
	internalIP string
	dcID       int
}

// EnsureComputeInstances ensures that all required compute instances are present and running.
func (b *GCPBootstrapper) EnsureComputeInstances() error {
	if err := b.ensureDataCenters(); err != nil {
		return err
	}

	sshKeys, err := b.getSSHKeys()
	if err != nil {
		return fmt.Errorf("failed to determine SSH keys: %w", err)
	}

	vms := VMDefsForEnv(b.Env)
	wg := sync.WaitGroup{}
	errCh := make(chan error, len(vms))
	resultCh := make(chan vmResult, len(vms))
	logCh := make(chan string, len(vms))

	for _, vm := range vms {
		wg.Add(1)
		go func(vm VMDef) {
			defer wg.Done()
			result, err := b.ensureVM(vm, b.Env.RootDiskSize, sshKeys, logCh)
			if err != nil {
				errCh <- err
				return
			}
			resultCh <- result
		}(vm)
	}
	wg.Wait()

	close(errCh)
	close(resultCh)
	close(logCh)

	for msg := range logCh {
		b.stlog.Logf("%s", msg)
	}

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
	dcByID := map[int]*datacenter.DataCenter{}

	for _, dc := range b.Env.DataCenters {
		dc.CephNodes = nil
		dc.ControlPlaneNodes = nil
		dcByID[dc.ID] = dc
	}
	for result := range resultCh {
		switch result.vmType {
		case "jumpbox":
			b.Env.Jumpbox.UpdateNode(result.name, result.externalIP, result.internalIP)
		case "postgres":
			b.Env.PostgreSQLNode = b.Env.Jumpbox.CreateSubNode(result.name, result.externalIP, result.internalIP)
		case "ceph":
			dc, ok := dcByID[result.dcID]
			if !ok {
				return fmt.Errorf("instance %s belongs to unknown data center %d", result.name, result.dcID)
			}

			dc.CephNodes = append(dc.CephNodes, b.Env.Jumpbox.CreateSubNode(result.name, result.externalIP, result.internalIP))
		case "k0s":
			dc, ok := dcByID[result.dcID]
			if !ok {
				return fmt.Errorf("instance %s belongs to unknown data center %d", result.name, result.dcID)
			}

			dc.ControlPlaneNodes = append(dc.ControlPlaneNodes, b.Env.Jumpbox.CreateSubNode(result.name, result.externalIP, result.internalIP))
		}
	}

	// Sort each data center's nodes by name to ensure consistent ordering, since the install
	// config assigns roles by index.
	for _, dc := range b.Env.DataCenters {
		sort.Slice(dc.CephNodes, func(i, j int) bool {
			return dc.CephNodes[i].GetName() < dc.CephNodes[j].GetName()
		})
		sort.Slice(dc.ControlPlaneNodes, func(i, j int) bool {
			return dc.ControlPlaneNodes[i].GetName() < dc.ControlPlaneNodes[j].GetName()
		})
	}

	b.mirrorPrimaryDataCenter()

	return nil
}

func (b *GCPBootstrapper) getSSHKeys() (string, error) {
	sshKeys := ""
	if b.Env.GitHubPAT != "" && b.Env.GitHubTeamOrg != "" && b.Env.GitHubTeamSlug != "" {
		var err error
		sshKeys, err = github.GetSSHKeysFromGitHubTeam(b.GitHubClient, b.Env.GitHubTeamOrg, b.Env.GitHubTeamSlug)
		if err != nil {
			return "", fmt.Errorf("failed to get SSH keys from GitHub team: %w", err)
		}
	}

	pubKey, err := b.ReadSSHKey(b.Env.SSHPublicKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read SSH public key: %w", err)
	}

	sshKeys += fmt.Sprintf("root:%s\nubuntu:%s", pubKey+"root", pubKey+"ubuntu")
	return sshKeys, nil
}

// ensureVM handles the full lifecycle of a single VM: check existence, restart if stopped,
// or create a new instance with spot fallback. Returns the VM result with assigned IPs.
func (b *GCPBootstrapper) ensureVM(vm VMDef, rootDiskSize int64, sshKeys string, logCh chan<- string) (vmResult, error) {
	projectID := b.Env.ProjectID
	zone := b.Env.Zone

	existingInstance, err := b.GCPClient.GetInstance(projectID, zone, vm.Name)
	if err != nil && !IsNotFoundError(err) {
		return vmResult{}, fmt.Errorf("failed to get instance %s: %w", vm.Name, err)
	}

	if existingInstance != nil {
		switch s := existingInstance.GetStatus(); s {
		case "TERMINATED", "STOPPED":
			if err := b.GCPClient.StartInstance(projectID, zone, vm.Name); err != nil {
				return vmResult{}, fmt.Errorf("failed to start stopped instance %s: %w", vm.Name, err)
			}
		case "SUSPENDED":
			return vmResult{}, fmt.Errorf("instance %s is SUSPENDED; manual resume is required", vm.Name)
		}
	} else {
		instance, err := b.buildInstanceSpec(vm, rootDiskSize, sshKeys)
		if err != nil {
			return vmResult{}, err
		}
		if err := b.CreateInstanceWithFallback(projectID, zone, instance, vm.Name, logCh); err != nil {
			return vmResult{}, err
		}
	}

	readyInstance, err := b.waitForInstanceRunning(projectID, zone, vm.Name, vm.ExternalIP)
	if err != nil {
		return vmResult{}, fmt.Errorf("instance %s did not become ready: %w", vm.Name, err)
	}

	internalIP, externalIP := ExtractInstanceIPs(readyInstance)
	return vmResult{
		vmType:     vm.Tags[0],
		name:       vm.Name,
		externalIP: externalIP,
		internalIP: internalIP,
		dcID:       vm.DataCenterID,
	}, nil
}

// buildInstanceSpec constructs the full compute instance specification for a VM.
func (b *GCPBootstrapper) buildInstanceSpec(vm VMDef, rootDiskSize int64, sshKeys string) (*computepb.Instance, error) {
	projectID := b.Env.ProjectID
	region := b.Env.Region
	zone := b.Env.Zone

	network := fmt.Sprintf("projects/%s/global/networks/%s-vpc", projectID, projectID)
	subnetwork := fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s-%s-subnet", projectID, region, projectID, region)
	diskType := fmt.Sprintf("projects/%s/zones/%s/diskTypes/pd-ssd", projectID, zone)

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
		Scheduling: b.BuildSchedulingConfig(),
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

	if vm.ExternalIP {
		instance.NetworkInterfaces[0].AccessConfigs = []*computepb.AccessConfig{
			{
				Name: protoString("External NAT"),
				Type: protoString("ONE_TO_ONE_NAT"),
			},
		}
	}

	return instance, nil
}

// ExtractInstanceIPs returns the internal and external IPs from a compute instance.
func ExtractInstanceIPs(inst *computepb.Instance) (internalIP, externalIP string) {
	if len(inst.GetNetworkInterfaces()) > 0 {
		internalIP = inst.GetNetworkInterfaces()[0].GetNetworkIP()
		if len(inst.GetNetworkInterfaces()[0].GetAccessConfigs()) > 0 {
			externalIP = inst.GetNetworkInterfaces()[0].GetAccessConfigs()[0].GetNatIP()
		}
	}
	return
}

// IsInstanceReady checks if an instance is RUNNING with its internal IP assigned,
// and optionally its external IP as well.
func IsInstanceReady(inst *computepb.Instance, needsExternalIP bool) bool {
	if inst.GetStatus() != "RUNNING" || len(inst.GetNetworkInterfaces()) == 0 {
		return false
	}
	ni := inst.GetNetworkInterfaces()[0]
	if ni.GetNetworkIP() == "" {
		return false
	}
	if needsExternalIP && (len(ni.GetAccessConfigs()) == 0 || ni.GetAccessConfigs()[0].GetNatIP() == "") {
		return false
	}
	return true
}

// BuildSchedulingConfig creates the scheduling configuration based on spot/preemptible settings
func (b *GCPBootstrapper) BuildSchedulingConfig() *computepb.Scheduling {
	if b.Env.SpotVMs {
		return &computepb.Scheduling{
			ProvisioningModel:         protoString("SPOT"),
			OnHostMaintenance:         protoString("TERMINATE"),
			AutomaticRestart:          protoBool(false),
			InstanceTerminationAction: protoString("STOP"),
		}
	}
	if b.Env.Preemptible {
		return &computepb.Scheduling{
			Preemptible: protoBool(true),
		}
	}

	return &computepb.Scheduling{}
}

// CreateInstanceWithFallback attempts to create an instance with the configured settings.
// If spot VMs are enabled and creation fails due to capacity issues, it falls back to standard VMs.
func (b *GCPBootstrapper) CreateInstanceWithFallback(projectID, zone string, instance *computepb.Instance, vmName string, logCh chan<- string) error {
	err := b.GCPClient.CreateInstance(projectID, zone, instance)
	if err == nil {
		return nil
	}

	if IsAlreadyExistsError(err) {
		return nil
	}

	if b.Env.SpotVMs && IsSpotCapacityError(err) {
		logCh <- fmt.Sprintf("Spot capacity unavailable for %s, falling back to standard VM", vmName)
		instance.Scheduling = &computepb.Scheduling{}
		err = b.GCPClient.CreateInstance(projectID, zone, instance)
		if err != nil && !IsAlreadyExistsError(err) {
			return fmt.Errorf("failed to create instance %s (fallback to standard VM): %w", vmName, err)
		}
		return nil
	}

	return fmt.Errorf("failed to create instance %s: %w", vmName, err)
}

// waitForInstanceRunning polls GetInstance until the instance status is RUNNING
// and its internal IP (and external IP, when needsExternalIP is true) are populated.
// It returns the ready instance or an error if the deadline is exceeded.
func (b *GCPBootstrapper) waitForInstanceRunning(projectID, zone, name string, needsExternalIP bool) (*computepb.Instance, error) {
	const (
		maxAttempts  = 60
		pollInterval = 5 * time.Second
	)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		inst, err := b.GCPClient.GetInstance(projectID, zone, name)
		if err != nil {
			if IsNotFoundError(err) {
				if attempt < maxAttempts-1 {
					b.Time.Sleep(pollInterval)
				}
				continue
			}
			return nil, fmt.Errorf("failed to poll instance %s: %w", name, err)
		}

		if IsInstanceReady(inst, needsExternalIP) {
			return inst, nil
		}

		if attempt < maxAttempts-1 {
			b.Time.Sleep(pollInterval)
		}
	}
	return nil, fmt.Errorf("timed out waiting for instance %s to be RUNNING with IPs assigned after %s",
		name, pollInterval*time.Duration(maxAttempts))
}

// findVMDef looks up a VM definition by name among the given definitions. Returns nil if not
// found.
func findVMDef(defs []VMDef, name string) *VMDef {
	for i := range defs {
		if defs[i].Name == name {
			return &defs[i]
		}
	}
	return nil
}

// validVMNames returns the names of the given VM definitions.
func validVMNames(defs []VMDef) []string {
	names := make([]string, len(defs))
	for i, vm := range defs {
		names[i] = vm.Name
	}
	return names
}

// RestartVM restarts a single stopped or terminated VM by a name defined for this environment.
func (b *GCPBootstrapper) RestartVM(name string) error {
	defs := VMDefsForEnv(b.Env)

	vm := findVMDef(defs, name)
	if vm == nil {
		return fmt.Errorf("unknown VM name %q; valid names are: %s", name, strings.Join(validVMNames(defs), ", "))
	}

	projectID := b.Env.ProjectID
	zone := b.Env.Zone

	inst, err := b.GCPClient.GetInstance(projectID, zone, name)
	if err != nil {
		if IsNotFoundError(err) {
			return fmt.Errorf("instance %s does not exist in project %s / zone %s; did you run bootstrap first?", name, projectID, zone)
		}
		return fmt.Errorf("failed to get instance %s: %w", name, err)
	}

	switch s := inst.GetStatus(); s {
	case "RUNNING":
		log.Printf("Instance %s is already running", name)
		return nil
	case "TERMINATED", "STOPPED":
		log.Printf("Starting stopped instance %s...", name)
		if err := b.GCPClient.StartInstance(projectID, zone, name); err != nil {
			return fmt.Errorf("failed to start instance %s: %w", name, err)
		}
	case "SUSPENDED":
		return fmt.Errorf("instance %s is SUSPENDED; manual resume is required", name)
	default:
		return fmt.Errorf("instance %s is in unexpected state %q", name, s)
	}

	readyInstance, err := b.waitForInstanceRunning(projectID, zone, name, vm.ExternalIP)
	if err != nil {
		return fmt.Errorf("instance %s did not become ready: %w", name, err)
	}

	internalIP, externalIP := ExtractInstanceIPs(readyInstance)
	log.Printf("Instance %s is now running (internal=%s, external=%s)", name, internalIP, externalIP)
	return nil
}

// RestartVMs restarts all stopped or terminated VMs of the environment, across every data center.
func (b *GCPBootstrapper) RestartVMs() error {
	var errs []error

	for _, vm := range VMDefsForEnv(b.Env) {
		if err := b.RestartVM(vm.Name); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors restarting VMs: %w", errors.Join(errs...))
	}
	return nil
}

// ReadSSHKey reads an SSH key file, expanding ~ in the path
func (b *GCPBootstrapper) ReadSSHKey(path string) (string, error) {
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

// GetNodeByName returns the node by the given name
// Returns an error if gce instance is not found
func (b *GCPBootstrapper) GetNodeByName(name string) (*node.Node, error) {
	existingInstance, err := b.GCPClient.GetInstance(b.Env.ProjectID, b.Env.Zone, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance %s: %w", name, err)
	}

	existingNode := &node.Node{
		NodeClient: b.NodeClient,
		FileIO:     b.fw,
	}

	internalIP, externalIP := ExtractInstanceIPs(existingInstance)
	existingNode.UpdateNode(name, externalIP, internalIP)

	return existingNode, nil
}
