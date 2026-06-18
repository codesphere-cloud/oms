// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// package installer
// This file provides functions to read an ansible inventory during config init to fetch host information.
package installer

import (
	"fmt"
	"os"
	"sort"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"gopkg.in/yaml.v3"
)

// ansibleInventory holds all supported yaml tags to be parsed
type ansibleInventory struct {
	Ceph       *group `yaml:"ceph,omitempty"`
	K8sCP      *group `yaml:"k8s-cp,omitempty"`
	K8sWorkers *group `yaml:"k8s-workers,omitempty"`
}

type group struct {
	Hosts map[string]hostInfo `yaml:"hosts,omitempty"`
}

type hostInfo struct {
	PrivateIP string `yaml:"private_ip"`
}

// FetchFromAnsibleInventory parses the ansible inventory file and tries to fetch ceph and k8s host from it.
// Host info are added to the current install config.
// Returns an error if inventory file can't be read or is invalid.
func (g *InstallConfig) FetchFromAnsibleInventory(inventoryPath string) error {
	if g.Config == nil {
		g.Config = &files.RootConfig{}
	}

	data, err := os.ReadFile(inventoryPath)
	if err != nil {
		return fmt.Errorf("failed to read Ansible inventory file: %w", err)
	}

	var inventory *ansibleInventory

	err = yaml.Unmarshal(data, &inventory)
	if err != nil {
		return fmt.Errorf("failed to unmarshal Ansible inventory file: %w", err)
	}

	if inventory == nil {
		return fmt.Errorf("empty Ansible inventory file")
	}

	cephHosts, err := inventory.fetchCephHosts()
	if err != nil {
		return fmt.Errorf("failed to fetch ceph hosts from inventory: %w", err)
	}

	if len(cephHosts) > 0 {
		g.Config.Ceph.Hosts = cephHosts
	}

	k8sCPHosts, err := inventory.fetchK8sControlPlaneHosts()
	if err != nil {
		return fmt.Errorf("failed to fetch k8s control plane hosts from inventory: %w", err)
	}

	if len(k8sCPHosts) > 0 {
		g.Config.Kubernetes.ControlPlanes = k8sCPHosts
	}

	k8sWorkerHosts, err := inventory.fetchK8sWorkerHosts()
	if err != nil {
		return fmt.Errorf("failed to fetch k8s worker node hosts from inventory: %w", err)
	}

	if len(k8sWorkerHosts) > 0 {
		g.Config.Kubernetes.Workers = k8sWorkerHosts
	}

	return nil
}

// fetchCephHosts extracts Ceph hosts from the ansible inventory.
// Hosts are sorted alphabetically; the first host in the list is designated as the master.
func (i *ansibleInventory) fetchCephHosts() ([]files.CephHost, error) {
	hosts := []files.CephHost{}

	if i.Ceph == nil {
		return hosts, nil
	}

	if i.Ceph.Hosts == nil {
		return hosts, fmt.Errorf("no hosts block defined")
	}

	count := 0
	for _, key := range getSortedHostsGroupKeys(i.Ceph.Hosts) {
		hostVars := i.Ceph.Hosts[key]

		if hostVars.PrivateIP == "" {
			return nil, fmt.Errorf("missing private_ip for ceph host '%s'", key)
		}

		host := files.CephHost{
			Hostname:  key,
			IPAddress: hostVars.PrivateIP,
			IsMaster:  count == 0,
		}
		hosts = append(hosts, host)

		count++
	}

	return hosts, nil
}

// fetchK8sControlPlaneHosts extracts K8s control plane hosts from the ansible inventory
func (i *ansibleInventory) fetchK8sControlPlaneHosts() ([]files.K8sNode, error) {
	if i.K8sCP == nil {
		return []files.K8sNode{}, nil
	}

	if i.K8sCP.Hosts == nil {
		return []files.K8sNode{}, fmt.Errorf("no hosts block defined")
	}

	return fetchKubernetesHosts("k8s-cp", i.K8sCP.Hosts)
}

// fetchK8sWorkerHosts extracts K8s worker hosts from the ansible inventory
func (i *ansibleInventory) fetchK8sWorkerHosts() ([]files.K8sNode, error) {
	if i.K8sWorkers == nil {
		return []files.K8sNode{}, nil
	}

	if i.K8sWorkers.Hosts == nil {
		return []files.K8sNode{}, fmt.Errorf("no hosts block defined")
	}

	return fetchKubernetesHosts("k8s-workers", i.K8sWorkers.Hosts)
}

// fetchKubernetesHosts extract hosts from the given parentTag
func fetchKubernetesHosts(parentTag string, inventoryHosts map[string]hostInfo) ([]files.K8sNode, error) {
	hosts := []files.K8sNode{}

	for _, key := range getSortedHostsGroupKeys(inventoryHosts) {
		hostVars := inventoryHosts[key]

		if hostVars.PrivateIP == "" {
			return nil, fmt.Errorf("missing private_ip for k8s host '%s'", key)
		}

		host := files.K8sNode{
			IPAddress: hostVars.PrivateIP,
		}
		hosts = append(hosts, host)
	}

	return hosts, nil
}

func getSortedHostsGroupKeys(group map[string]hostInfo) []string {
	keys := make([]string, 0, len(group))
	for k := range group {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}
