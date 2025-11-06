// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

func collectField[T any](optValue T, isEmpty func(T) bool, promptFunc func() T) T {
	if !isEmpty(optValue) {
		return optValue
	}
	return promptFunc()
}

func isEmptyString(s string) bool  { return s == "" }
func isEmptyInt(i int) bool        { return i == 0 }
func isEmptySlice(s []string) bool { return len(s) == 0 }

func (g *InstallConfig) collectString(prompter *Prompter, optValue, prompt, defaultVal string) string {
	return collectField(optValue, isEmptyString, func() string {
		return prompter.String(prompt, defaultVal)
	})
}

func (g *InstallConfig) collectInt(prompter *Prompter, optValue int, prompt string, defaultVal int) int {
	return collectField(optValue, isEmptyInt, func() int {
		return prompter.Int(prompt, defaultVal)
	})
}

func (g *InstallConfig) collectStringSlice(prompter *Prompter, optValue []string, prompt string, defaultVal []string) []string {
	return collectField(optValue, isEmptySlice, func() []string {
		return prompter.StringSlice(prompt, defaultVal)
	})
}

func (g *InstallConfig) collectChoice(prompter *Prompter, optValue, prompt string, options []string, defaultVal string) string {
	return collectField(optValue, isEmptyString, func() string {
		return prompter.Choice(prompt, options, defaultVal)
	})
}

func (g *InstallConfig) collectConfig() (*files.CollectedConfig, error) {
	prompter := NewPrompter(g.Interactive)
	opts := g.configOpts
	collected := &files.CollectedConfig{}

	// TODO: no sub functions after they are simplifies and interactive is removed and the if else are simplified

	g.collectDatacenterConfig(prompter, opts, collected)
	g.collectRegistryConfig(prompter, opts, collected)
	g.collectPostgresConfig(prompter, opts, collected)
	g.collectCephConfig(prompter, opts, collected)
	g.collectK8sConfig(prompter, opts, collected)
	g.collectGatewayConfig(prompter, opts, collected)
	g.collectMetalLBConfig(prompter, opts, collected)
	g.collectCodesphereConfig(prompter, opts, collected)

	return collected, nil
}

func (g *InstallConfig) collectDatacenterConfig(prompter *Prompter, opts *files.ConfigOptions, collected *files.CollectedConfig) {
	fmt.Println("=== Datacenter Configuration ===")
	collected.DcID = g.collectInt(prompter, opts.DatacenterID, "Datacenter ID", 1)
	collected.DcName = g.collectString(prompter, opts.DatacenterName, "Datacenter name", "main")
	collected.DcCity = g.collectString(prompter, opts.DatacenterCity, "Datacenter city", "Karlsruhe")
	collected.DcCountry = g.collectString(prompter, opts.DatacenterCountryCode, "Country code", "DE")
	collected.SecretsBaseDir = g.collectString(prompter, opts.SecretsBaseDir, "Secrets base directory", "/root/secrets")
}

func (g *InstallConfig) collectRegistryConfig(prompter *Prompter, opts *files.ConfigOptions, collected *files.CollectedConfig) {
	fmt.Println("\n=== Container Registry Configuration ===")
	collected.RegistryServer = g.collectString(prompter, opts.RegistryServer, "Container registry server (e.g., ghcr.io, leave empty to skip)", "")
	if collected.RegistryServer != "" {
		collected.RegistryReplaceImages = opts.RegistryReplaceImages
		collected.RegistryLoadContainerImgs = opts.RegistryLoadContainerImgs
		if g.Interactive {
			collected.RegistryReplaceImages = prompter.Bool("Replace images in BOM", true)
			collected.RegistryLoadContainerImgs = prompter.Bool("Load container images from installer", false)
		}
	}
}

func (g *InstallConfig) collectPostgresConfig(prompter *Prompter, opts *files.ConfigOptions, collected *files.CollectedConfig) {
	fmt.Println("\n=== PostgreSQL Configuration ===")
	collected.PgMode = g.collectChoice(prompter, opts.PostgresMode, "PostgreSQL setup", []string{"install", "external"}, "install")

	if collected.PgMode == "install" {
		collected.PgPrimaryIP = g.collectString(prompter, opts.PostgresPrimaryIP, "Primary PostgreSQL server IP", "10.50.0.2")
		collected.PgPrimaryHost = g.collectString(prompter, opts.PostgresPrimaryHost, "Primary PostgreSQL hostname", "pg-primary-node")

		if g.Interactive {
			hasReplica := prompter.Bool("Configure PostgreSQL replica", true)
			if hasReplica {
				collected.PgReplicaIP = g.collectString(prompter, opts.PostgresReplicaIP, "Replica PostgreSQL server IP", "10.50.0.3")
				collected.PgReplicaName = g.collectString(prompter, opts.PostgresReplicaName, "Replica name (lowercase alphanumeric + underscore only)", "replica1")
			}
		} else {
			collected.PgReplicaIP = opts.PostgresReplicaIP
			collected.PgReplicaName = opts.PostgresReplicaName
		}
	} else {
		collected.PgExternal = g.collectString(prompter, opts.PostgresExternal, "External PostgreSQL server address", "postgres.example.com:5432")
	}
}

func (g *InstallConfig) collectCephConfig(prompter *Prompter, opts *files.ConfigOptions, collected *files.CollectedConfig) {
	fmt.Println("\n=== Ceph Configuration ===")
	collected.CephSubnet = g.collectString(prompter, opts.CephSubnet, "Ceph nodes subnet (CIDR)", "10.53.101.0/24")

	if len(opts.CephHosts) == 0 {
		numHosts := prompter.Int("Number of Ceph hosts", 3)
		collected.CephHosts = make([]files.CephHost, numHosts)
		for i := 0; i < numHosts; i++ {
			fmt.Printf("\nCeph Host %d:\n", i+1)
			collected.CephHosts[i].Hostname = prompter.String("  Hostname (as shown by 'hostname' command)", fmt.Sprintf("ceph-node-%d", i))
			collected.CephHosts[i].IPAddress = prompter.String("  IP address", fmt.Sprintf("10.53.101.%d", i+2))
			collected.CephHosts[i].IsMaster = (i == 0)
		}
	} else {
		collected.CephHosts = make([]files.CephHost, len(opts.CephHosts))
		for i, host := range opts.CephHosts {
			collected.CephHosts[i] = files.CephHost(host)
		}
	}
}

func (g *InstallConfig) collectK8sConfig(prompter *Prompter, opts *files.ConfigOptions, collected *files.CollectedConfig) {
	fmt.Println("\n=== Kubernetes Configuration ===")
	collected.K8sManaged = opts.K8sManaged
	if g.Interactive {
		collected.K8sManaged = prompter.Bool("Use Codesphere-managed Kubernetes (k0s)", true)
	}

	if collected.K8sManaged {
		collected.K8sAPIServer = g.collectString(prompter, opts.K8sAPIServer, "Kubernetes API server host (LB/DNS/IP)", "10.50.0.2")
		collected.K8sControlPlane = g.collectStringSlice(prompter, opts.K8sControlPlane, "Control plane IP addresses (comma-separated)", []string{"10.50.0.2"})
		collected.K8sWorkers = g.collectStringSlice(prompter, opts.K8sWorkers, "Worker node IP addresses (comma-separated)", []string{"10.50.0.2", "10.50.0.3", "10.50.0.4"})
	} else {
		collected.K8sPodCIDR = g.collectString(prompter, opts.K8sPodCIDR, "Pod CIDR of external cluster", "100.96.0.0/11")
		collected.K8sServiceCIDR = g.collectString(prompter, opts.K8sServiceCIDR, "Service CIDR of external cluster", "100.64.0.0/13")
		fmt.Println("Note: You'll need to provide kubeconfig in the vault file for external Kubernetes")
	}
}

func (g *InstallConfig) collectGatewayConfig(prompter *Prompter, opts *files.ConfigOptions, collected *files.CollectedConfig) {
	// TODO: in ifs
	fmt.Println("\n=== Cluster Gateway Configuration ===")
	collected.GatewayType = g.collectChoice(prompter, opts.ClusterGatewayType, "Gateway service type", []string{"LoadBalancer", "ExternalIP"}, "LoadBalancer")
	if collected.GatewayType == "ExternalIP" {
		collected.GatewayIPs = g.collectStringSlice(prompter, opts.ClusterGatewayIPs, "Gateway IP addresses (comma-separated)", []string{"10.51.0.2", "10.51.0.3"})
	} else {
		collected.GatewayIPs = opts.ClusterGatewayIPs
	}

	collected.PublicGatewayType = g.collectChoice(prompter, opts.ClusterPublicGatewayType, "Public gateway service type", []string{"LoadBalancer", "ExternalIP"}, "LoadBalancer")
	if collected.PublicGatewayType == "ExternalIP" {
		collected.PublicGatewayIPs = g.collectStringSlice(prompter, opts.ClusterPublicGatewayIPs, "Public gateway IP addresses (comma-separated)", []string{"10.52.0.2", "10.52.0.3"})
	} else {
		collected.PublicGatewayIPs = opts.ClusterPublicGatewayIPs
	}
}

func (g *InstallConfig) collectMetalLBConfig(prompter *Prompter, opts *files.ConfigOptions, collected *files.CollectedConfig) {
	fmt.Println("\n=== MetalLB Configuration (Optional) ===")
	if g.Interactive {
		collected.MetalLBEnabled = prompter.Bool("Enable MetalLB", false)
		if collected.MetalLBEnabled {
			numPools := prompter.Int("Number of MetalLB IP pools", 1)
			collected.MetalLBPools = make([]files.MetalLBPoolDef, numPools)
			for i := 0; i < numPools; i++ {
				fmt.Printf("\nMetalLB Pool %d:\n", i+1)
				poolName := prompter.String("  Pool name", fmt.Sprintf("pool-%d", i+1))
				poolIPs := prompter.StringSlice("  IP addresses/ranges (comma-separated)", []string{"10.10.10.100-10.10.10.200"})
				collected.MetalLBPools[i] = files.MetalLBPoolDef{
					Name:        poolName,
					IPAddresses: poolIPs,
				}
			}
		}
	} else if opts.MetalLBEnabled {
		collected.MetalLBEnabled = true
		collected.MetalLBPools = make([]files.MetalLBPoolDef, len(opts.MetalLBPools))
		for i, pool := range opts.MetalLBPools {
			collected.MetalLBPools[i] = files.MetalLBPoolDef(pool)
		}
	}
}

func (g *InstallConfig) collectCodesphereConfig(prompter *Prompter, opts *files.ConfigOptions, collected *files.CollectedConfig) {
	fmt.Println("\n=== Codesphere Application Configuration ===")
	collected.CodesphereDomain = g.collectString(prompter, opts.CodesphereDomain, "Main Codesphere domain", "codesphere.yourcompany.com")
	collected.WorkspaceDomain = g.collectString(prompter, opts.CodesphereWorkspaceBaseDomain, "Workspace base domain (*.domain should point to public gateway)", "ws.yourcompany.com")
	collected.PublicIP = g.collectString(prompter, opts.CodespherePublicIP, "Primary public IP for workspaces", "")
	collected.CustomDomain = g.collectString(prompter, opts.CodesphereCustomDomainBaseDomain, "Custom domain CNAME base", "custom.yourcompany.com")
	collected.DnsServers = g.collectStringSlice(prompter, opts.CodesphereDNSServers, "DNS servers (comma-separated)", []string{"1.1.1.1", "8.8.8.8"})

	fmt.Println("\n=== Workspace Plans Configuration ===")
	collected.WorkspaceImageBomRef = g.collectString(prompter, opts.CodesphereWorkspaceImageBomRef, "Workspace agent image BOM reference", "workspace-agent-24.04")
	collected.HostingPlanCPU = g.collectInt(prompter, opts.CodesphereHostingPlanCPU, "Hosting plan CPU (tenths, e.g., 10 = 1 core)", 10)
	collected.HostingPlanMemory = g.collectInt(prompter, opts.CodesphereHostingPlanMemory, "Hosting plan memory (MB)", 2048)
	collected.HostingPlanStorage = g.collectInt(prompter, opts.CodesphereHostingPlanStorage, "Hosting plan storage (MB)", 20480)
	collected.HostingPlanTempStorage = g.collectInt(prompter, opts.CodesphereHostingPlanTempStorage, "Hosting plan temp storage (MB)", 1024)
	collected.WorkspacePlanName = g.collectString(prompter, opts.CodesphereWorkspacePlanName, "Workspace plan name", "Standard Developer")
	collected.WorkspacePlanMaxReplica = g.collectInt(prompter, opts.CodesphereWorkspacePlanMaxReplica, "Max replicas per workspace", 3)
}
