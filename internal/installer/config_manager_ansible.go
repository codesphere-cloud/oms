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

type ansibleInventory map[string]map[string]map[string]any

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

	var inventory ansibleInventory

	err = yaml.Unmarshal(data, &inventory)
	if err != nil {
		return fmt.Errorf("failed to unmarshal Ansible inventory file: %w", err)
	}

	if inventory == nil {
		return fmt.Errorf("empty Ansible inventory file")
	}

	cephHosts, err := fetchCephHosts(inventory)
	if err != nil {
		return fmt.Errorf("failed to fetch ceph hosts from inventory: %w", err)
	}

	if len(cephHosts) > 0 {
		g.Config.Ceph.Hosts = cephHosts
	}

	k8sCPHosts, err := fetchK8sControlPlaneHosts(inventory)
	if err != nil {
		return fmt.Errorf("failed to fetch k8s control plane hosts from inventory: %w", err)
	}

	if len(k8sCPHosts) > 0 {
		if len(g.Config.Kubernetes.ControlPlanes) > 0 {
			return fmt.Errorf("k8s control plane nodes are already set. Adjust flags or inventory")
		}
		g.Config.Kubernetes.ControlPlanes = k8sCPHosts
	}

	k8sWorkerHosts, err := fetchK8sWorkerHosts(inventory)
	if err != nil {
		return fmt.Errorf("failed to fetch k8s worker node hosts from inventory: %w", err)
	}

	if len(k8sWorkerHosts) > 0 {
		if len(g.Config.Kubernetes.Workers) > 0 {
			return fmt.Errorf("k8s worker nodes are already set. Adjust flags or inventory")
		}
		g.Config.Kubernetes.Workers = k8sWorkerHosts
	}

	return nil
}

// fetchCephHosts extracts Ceph hosts from the ansible inventory.
// The first ceph host parsed is considered as the master.
// Supported YAML format:
// ceph:
//
//	hosts:
//	  host-name-1:
//	    private_ip: "10.0.0.1"
//	  host-name-2:
//	    private_ip: "10.0.0.2"
func fetchCephHosts(inventory ansibleInventory) ([]files.CephHost, error) {
	hosts := []files.CephHost{}

	// check if ceph exists in inventory
	cephGroup, ok := inventory["ceph"]
	if !ok {
		return hosts, nil // No ceph group, return empty host list
	}

	// check if ceph.hosts exists in inventory
	hostsGroup, ok := cephGroup["hosts"]
	if !ok {
		return hosts, nil // No hosts under ceph group, return empty host list
	}

	count := 0
	for _, key := range getSortedHostsGroupKeys(hostsGroup) {
		hostVars := hostsGroup[key]

		privateIP := fetchHostVarsValue("private_ip", hostVars)
		if privateIP == "" {
			return nil, fmt.Errorf("missing private_ip for ceph host '%s'", key)
		}

		host := files.CephHost{
			Hostname:  key,
			IPAddress: privateIP,
			IsMaster:  count == 0,
		}
		hosts = append(hosts, host)

		count++
	}

	return hosts, nil
}

// fetchK8sControlPlaneHosts extracts K8s control plane hosts from the ansible inventory
// Supported YAML format:
// k8s-cp:
//
//	hosts:
//	  my-host-name:
//	    private_ip: "10.0.0.1"
func fetchK8sControlPlaneHosts(inventory ansibleInventory) ([]files.K8sNode, error) {
	return fetchKubernetesHosts("k8s-cp", inventory)
}

// fetchK8sWorkerHosts extracts K8s worker hosts from the ansible inventory
// Supported YAML format:
// k8s-workers:
//
//	hosts:
//	  my-host-name:
//	    private_ip: "10.0.0.1"
func fetchK8sWorkerHosts(inventory ansibleInventory) ([]files.K8sNode, error) {
	return fetchKubernetesHosts("k8s-workers", inventory)
}

// fetchKubernetesHosts extract hosts from the given parentTag
func fetchKubernetesHosts(parentTag string, inventory ansibleInventory) ([]files.K8sNode, error) {
	hosts := []files.K8sNode{}

	// check if parentTag exists in inventory
	k8sGroup, ok := inventory[parentTag]
	if !ok {
		return hosts, nil
	}

	// check if hosts exists in inventory
	hostsGroup, ok := k8sGroup["hosts"]
	if !ok {
		return hosts, nil
	}

	for _, key := range getSortedHostsGroupKeys(hostsGroup) {
		privateIP := fetchHostVarsValue("private_ip", hostsGroup[key])
		if privateIP == "" {
			return nil, fmt.Errorf("missing private_ip for k8s host '%s'", key)
		}

		host := files.K8sNode{
			IPAddress: privateIP,
		}
		hosts = append(hosts, host)
	}

	return hosts, nil
}

func getSortedHostsGroupKeys(group map[string]any) []string {
	keys := make([]string, 0, len(group))
	for k := range group {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

func fetchHostVarsValue(key string, hostVars any) string {
	value := ""

	if vars, ok := hostVars.(map[string]any); ok {
		anyValue, exists := vars[key]
		if exists && anyValue != nil {
			value = fmt.Sprintf("%v", anyValue)
		}
	}

	return value
}
