// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
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
}

// Example VM definitions (expand as needed)
var vmDefs = []VMDef{
	{"jumpbox", "e2-medium", []string{"jumpbox", "ssh"}, []int64{}, true},
	{"postgres", "e2-standard-2", []string{"postgres"}, []int64{}, true},
	{"ceph-1", "e2-standard-4", []string{"ceph"}, []int64{10, 100}, false},
	{"ceph-2", "e2-standard-4", []string{"ceph"}, []int64{10, 100}, false},
	{"ceph-3", "e2-standard-4", []string{"ceph"}, []int64{10, 100}, false},
	{"k0s-1", "e2-standard-8", []string{"k0s"}, []int64{}, false},
	{"k0s-2", "e2-standard-8", []string{"k0s"}, []int64{}, false},
	{"k0s-3", "e2-standard-8", []string{"k0s"}, []int64{}, false},
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
}

func (b *GCPBootstrapper) EnsureComputeInstances() error {
	rootDiskSize := int64(200)
	if b.Env.RegistryType == RegistryTypeGitHub {
		rootDiskSize = 50
	}

	wg := sync.WaitGroup{}
	errCh := make(chan error, len(vmDefs))
	resultCh := make(chan vmResult, len(vmDefs))
	logCh := make(chan string, len(vmDefs))

	for _, vm := range vmDefs {
		wg.Add(1)
		go func(vm VMDef) {
			defer wg.Done()
			result, err := b.ensureVM(vm, rootDiskSize, logCh)
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

// ensureVM handles the full lifecycle of a single VM: check existence, restart if stopped,
// or create a new instance with spot fallback. Returns the VM result with assigned IPs.
func (b *GCPBootstrapper) ensureVM(vm VMDef, rootDiskSize int64, logCh chan<- string) (vmResult, error) {
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
		instance, err := b.buildInstanceSpec(vm, rootDiskSize)
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
	}, nil
}

// buildInstanceSpec constructs the full compute instance specification for a VM.
func (b *GCPBootstrapper) buildInstanceSpec(vm VMDef, rootDiskSize int64) (*computepb.Instance, error) {
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

	sshKeys := ""
	if b.Env.GitHubPAT != "" && b.Env.GitHubTeamOrg != "" && b.Env.GitHubTeamSlug != "" {
		var err error
		sshKeys, err = github.GetSSHKeysFromGitHubTeam(b.GitHubClient, b.Env.GitHubTeamOrg, b.Env.GitHubTeamSlug)
		if err != nil {
			return nil, fmt.Errorf("failed to get SSH keys from GitHub team: %w", err)
		}
	}

	pubKey, err := b.ReadSSHKey(b.Env.SSHPublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SSH public key: %w", err)
	}

	sshKeys += fmt.Sprintf("root:%s\nubuntu:%s", pubKey+"root", pubKey+"ubuntu")

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
