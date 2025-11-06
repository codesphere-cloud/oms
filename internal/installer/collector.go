// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import "fmt"

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

func (g *InstallConfig) collectConfig() (*collectedConfig, error) {
	prompter := NewPrompter(g.Interactive)
	opts := g.configOpts
	collected := &collectedConfig{}

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

func (g *InstallConfig) collectDatacenterConfig(prompter *Prompter, opts *ConfigOptions, collected *collectedConfig) {
	fmt.Println("=== Datacenter Configuration ===")
	collected.dcID = g.collectInt(prompter, opts.DatacenterID, "Datacenter ID", 1)
	collected.dcName = g.collectString(prompter, opts.DatacenterName, "Datacenter name", "main")
	collected.dcCity = g.collectString(prompter, opts.DatacenterCity, "Datacenter city", "Karlsruhe")
	collected.dcCountry = g.collectString(prompter, opts.DatacenterCountryCode, "Country code", "DE")
	collected.secretsBaseDir = g.collectString(prompter, opts.SecretsBaseDir, "Secrets base directory", "/root/secrets")
}

func (g *InstallConfig) collectRegistryConfig(prompter *Prompter, opts *ConfigOptions, collected *collectedConfig) {
	fmt.Println("\n=== Container Registry Configuration ===")
	collected.registryServer = g.collectString(prompter, opts.RegistryServer, "Container registry server (e.g., ghcr.io, leave empty to skip)", "")
	if collected.registryServer != "" {
		collected.registryReplaceImages = opts.RegistryReplaceImages
		collected.registryLoadContainerImgs = opts.RegistryLoadContainerImgs
		if g.Interactive {
			collected.registryReplaceImages = prompter.Bool("Replace images in BOM", true)
			collected.registryLoadContainerImgs = prompter.Bool("Load container images from installer", false)
		}
	}
}

func (g *InstallConfig) collectPostgresConfig(prompter *Prompter, opts *ConfigOptions, collected *collectedConfig) {
	fmt.Println("\n=== PostgreSQL Configuration ===")
	collected.pgMode = g.collectChoice(prompter, opts.PostgresMode, "PostgreSQL setup", []string{"install", "external"}, "install")

	if collected.pgMode == "install" {
		collected.pgPrimaryIP = g.collectString(prompter, opts.PostgresPrimaryIP, "Primary PostgreSQL server IP", "10.50.0.2")
		collected.pgPrimaryHost = g.collectString(prompter, opts.PostgresPrimaryHost, "Primary PostgreSQL hostname", "pg-primary-node")

		if g.Interactive {
			hasReplica := prompter.Bool("Configure PostgreSQL replica", true)
			if hasReplica {
				collected.pgReplicaIP = g.collectString(prompter, opts.PostgresReplicaIP, "Replica PostgreSQL server IP", "10.50.0.3")
				collected.pgReplicaName = g.collectString(prompter, opts.PostgresReplicaName, "Replica name (lowercase alphanumeric + underscore only)", "replica1")
			}
		} else {
			collected.pgReplicaIP = opts.PostgresReplicaIP
			collected.pgReplicaName = opts.PostgresReplicaName
		}
	} else {
		collected.pgExternal = g.collectString(prompter, opts.PostgresExternal, "External PostgreSQL server address", "postgres.example.com:5432")
	}
}

func (g *InstallConfig) collectCephConfig(prompter *Prompter, opts *ConfigOptions, collected *collectedConfig) {
	fmt.Println("\n=== Ceph Configuration ===")
	collected.cephSubnet = g.collectString(prompter, opts.CephSubnet, "Ceph nodes subnet (CIDR)", "10.53.101.0/24")

	if len(opts.CephHosts) == 0 {
		numHosts := prompter.Int("Number of Ceph hosts", 3)
		collected.cephHosts = make([]CephHost, numHosts)
		for i := 0; i < numHosts; i++ {
			fmt.Printf("\nCeph Host %d:\n", i+1)
			collected.cephHosts[i].Hostname = prompter.String("  Hostname (as shown by 'hostname' command)", fmt.Sprintf("ceph-node-%d", i))
			collected.cephHosts[i].IPAddress = prompter.String("  IP address", fmt.Sprintf("10.53.101.%d", i+2))
			collected.cephHosts[i].IsMaster = (i == 0)
		}
	} else {
		collected.cephHosts = make([]CephHost, len(opts.CephHosts))
		for i, host := range opts.CephHosts {
			collected.cephHosts[i] = CephHost(host)
		}
	}
}

func (g *InstallConfig) collectK8sConfig(prompter *Prompter, opts *ConfigOptions, collected *collectedConfig) {
	fmt.Println("\n=== Kubernetes Configuration ===")
	collected.k8sManaged = opts.K8sManaged
	if g.Interactive {
		collected.k8sManaged = prompter.Bool("Use Codesphere-managed Kubernetes (k0s)", true)
	}

	if collected.k8sManaged {
		collected.k8sAPIServer = g.collectString(prompter, opts.K8sAPIServer, "Kubernetes API server host (LB/DNS/IP)", "10.50.0.2")
		collected.k8sControlPlane = g.collectStringSlice(prompter, opts.K8sControlPlane, "Control plane IP addresses (comma-separated)", []string{"10.50.0.2"})
		collected.k8sWorkers = g.collectStringSlice(prompter, opts.K8sWorkers, "Worker node IP addresses (comma-separated)", []string{"10.50.0.2", "10.50.0.3", "10.50.0.4"})
	} else {
		collected.k8sPodCIDR = g.collectString(prompter, opts.K8sPodCIDR, "Pod CIDR of external cluster", "100.96.0.0/11")
		collected.k8sServiceCIDR = g.collectString(prompter, opts.K8sServiceCIDR, "Service CIDR of external cluster", "100.64.0.0/13")
		fmt.Println("Note: You'll need to provide kubeconfig in the vault file for external Kubernetes")
	}
}

func (g *InstallConfig) collectGatewayConfig(prompter *Prompter, opts *ConfigOptions, collected *collectedConfig) {
	fmt.Println("\n=== Cluster Gateway Configuration ===")
	collected.gatewayType = g.collectChoice(prompter, opts.ClusterGatewayType, "Gateway service type", []string{"LoadBalancer", "ExternalIP"}, "LoadBalancer")
	if collected.gatewayType == "ExternalIP" {
		collected.gatewayIPs = g.collectStringSlice(prompter, opts.ClusterGatewayIPs, "Gateway IP addresses (comma-separated)", []string{"10.51.0.2", "10.51.0.3"})
	} else {
		collected.gatewayIPs = opts.ClusterGatewayIPs
	}

	collected.publicGatewayType = g.collectChoice(prompter, opts.ClusterPublicGatewayType, "Public gateway service type", []string{"LoadBalancer", "ExternalIP"}, "LoadBalancer")
	if collected.publicGatewayType == "ExternalIP" {
		collected.publicGatewayIPs = g.collectStringSlice(prompter, opts.ClusterPublicGatewayIPs, "Public gateway IP addresses (comma-separated)", []string{"10.52.0.2", "10.52.0.3"})
	} else {
		collected.publicGatewayIPs = opts.ClusterPublicGatewayIPs
	}
}

func (g *InstallConfig) collectMetalLBConfig(prompter *Prompter, opts *ConfigOptions, collected *collectedConfig) {
	fmt.Println("\n=== MetalLB Configuration (Optional) ===")
	if g.Interactive {
		collected.metalLBEnabled = prompter.Bool("Enable MetalLB", false)
		if collected.metalLBEnabled {
			numPools := prompter.Int("Number of MetalLB IP pools", 1)
			collected.metalLBPools = make([]MetalLBPoolDef, numPools)
			for i := 0; i < numPools; i++ {
				fmt.Printf("\nMetalLB Pool %d:\n", i+1)
				poolName := prompter.String("  Pool name", fmt.Sprintf("pool-%d", i+1))
				poolIPs := prompter.StringSlice("  IP addresses/ranges (comma-separated)", []string{"10.10.10.100-10.10.10.200"})
				collected.metalLBPools[i] = MetalLBPoolDef{
					Name:        poolName,
					IPAddresses: poolIPs,
				}
			}
		}
	} else if opts.MetalLBEnabled {
		collected.metalLBEnabled = true
		collected.metalLBPools = make([]MetalLBPoolDef, len(opts.MetalLBPools))
		for i, pool := range opts.MetalLBPools {
			collected.metalLBPools[i] = MetalLBPoolDef(pool)
		}
	}
}

func (g *InstallConfig) collectCodesphereConfig(prompter *Prompter, opts *ConfigOptions, collected *collectedConfig) {
	fmt.Println("\n=== Codesphere Application Configuration ===")
	collected.codesphereDomain = g.collectString(prompter, opts.CodesphereDomain, "Main Codesphere domain", "codesphere.yourcompany.com")
	collected.workspaceDomain = g.collectString(prompter, opts.CodesphereWorkspaceBaseDomain, "Workspace base domain (*.domain should point to public gateway)", "ws.yourcompany.com")
	collected.publicIP = g.collectString(prompter, opts.CodespherePublicIP, "Primary public IP for workspaces", "")
	collected.customDomain = g.collectString(prompter, opts.CodesphereCustomDomainBaseDomain, "Custom domain CNAME base", "custom.yourcompany.com")
	collected.dnsServers = g.collectStringSlice(prompter, opts.CodesphereDNSServers, "DNS servers (comma-separated)", []string{"1.1.1.1", "8.8.8.8"})

	fmt.Println("\n=== Workspace Plans Configuration ===")
	collected.workspaceImageBomRef = g.collectString(prompter, opts.CodesphereWorkspaceImageBomRef, "Workspace agent image BOM reference", "workspace-agent-24.04")
	collected.hostingPlanCPU = g.collectInt(prompter, opts.CodesphereHostingPlanCPU, "Hosting plan CPU (tenths, e.g., 10 = 1 core)", 10)
	collected.hostingPlanMemory = g.collectInt(prompter, opts.CodesphereHostingPlanMemory, "Hosting plan memory (MB)", 2048)
	collected.hostingPlanStorage = g.collectInt(prompter, opts.CodesphereHostingPlanStorage, "Hosting plan storage (MB)", 20480)
	collected.hostingPlanTempStorage = g.collectInt(prompter, opts.CodesphereHostingPlanTempStorage, "Hosting plan temp storage (MB)", 1024)
	collected.workspacePlanName = g.collectString(prompter, opts.CodesphereWorkspacePlanName, "Workspace plan name", "Standard Developer")
	collected.workspacePlanMaxReplica = g.collectInt(prompter, opts.CodesphereWorkspacePlanMaxReplica, "Max replicas per workspace", 3)
}
