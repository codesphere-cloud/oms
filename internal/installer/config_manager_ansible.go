// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"os"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"gopkg.in/yaml.v3"
)

type ansibleInventory map[string]map[string]map[string]any

func (g *InstallConfig) FetchFromAnsibleInventory(inventoryPath string) error {
	if g.Config == nil {
		g.Config = &files.RootConfig{}
	}

	// Read Ansible inventory file
	data, err := os.ReadFile(inventoryPath)
	if err != nil {
		return fmt.Errorf("failed to read Ansible inventory file: %w", err)
	}

	var inventory ansibleInventory

	err = yaml.Unmarshal([]byte(data), &inventory)
	if err != nil {
		return fmt.Errorf("failed to unmarshal Ansible inventory file: %w", err)
	}

	if inventory == nil {
		return fmt.Errorf("empty Ansible inventory file")
	}

	cephHosts, err := fetchCephHostsFromInventory(inventory)
	if err != nil {
		return fmt.Errorf("failed to fetch ceph hosts from inventory: %w", err)
	}

	if len(cephHosts) > 0 {
		g.Config.Ceph.Hosts = cephHosts
	}

	k8sCPHosts := fetchK8sControlPlaneHostsFromInventory(inventory)
	if len(k8sCPHosts) > 0 {
		g.Config.Kubernetes.ControlPlanes = k8sCPHosts
	}

	k8sWorkerHosts := fetchK8sWorkerHostsFromInventory(inventory)
	if len(k8sWorkerHosts) > 0 {
		g.Config.Kubernetes.Workers = k8sWorkerHosts
	}

	return nil
}

// fetchCephHostsFromInventory extracts Ceph host information from the Ansible inventory.
func fetchCephHostsFromInventory(inventory ansibleInventory) ([]files.CephHost, error) {
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
	for hostName, hostVars := range hostsGroup {
		privateIP := ""
		if vars, ok := hostVars.(map[string]any); ok {
			privateIP = vars["private_ip"].(string)
		}

		if privateIP == "" {
			return nil, fmt.Errorf("missing private_ip for ceph host '%s'", hostName)
		}

		host := files.CephHost{
			Hostname:  hostName,
			IPAddress: privateIP,
			IsMaster:  count == 0,
		}
		hosts = append(hosts, host)

		count++
	}

	return hosts, nil
}

func fetchK8sControlPlaneHostsFromInventory(inventory ansibleInventory) []files.K8sNode {
	return fetchKubernetesHostsFromInventory("k8s-cp", inventory)
}

func fetchK8sWorkerHostsFromInventory(inventory ansibleInventory) []files.K8sNode {
	return fetchKubernetesHostsFromInventory("k8s-workers", inventory)
}

func fetchKubernetesHostsFromInventory(parentTag string, inventory ansibleInventory) []files.K8sNode {
	hosts := []files.K8sNode{}

	// check if parentTag exists in inventory
	k8sGroup, ok := inventory[parentTag]
	if !ok {
		return hosts
	}

	// check if hosts exists in inventory
	hostsGroup, ok := k8sGroup["hosts"]
	if !ok {
		return hosts
	}

	for _, hostVars := range hostsGroup {
		privateIP := ""
		if vars, ok := hostVars.(map[string]any); ok {
			privateIP = vars["private_ip"].(string)
		}

		host := files.K8sNode{
			IPAddress: privateIP,
		}
		hosts = append(hosts, host)
	}

	return hosts
}
