// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

func (g *InstallConfig) CollectInteractively() error {
	prompter := NewPrompter(true)

	g.collectDatacenterConfig(prompter)
	g.collectRegistryConfig(prompter)
	g.collectPostgresConfig(prompter)
	g.collectCephConfig(prompter)
	g.collectK8sConfig(prompter)
	g.collectGatewayConfig(prompter)
	g.collectMetalLBConfig(prompter)
	g.collectCodesphereConfig(prompter)

	return nil
}

func collectField[T any](isEmpty func(T) bool, promptFunc func() T) T {
	return promptFunc()
}

func isEmptyString(s string) bool  { return s == "" }
func isEmptyInt(i int) bool        { return i == 0 }
func isEmptySlice(s []string) bool { return len(s) == 0 }

func (g *InstallConfig) collectString(prompter *Prompter, prompt, defaultVal string) string {
	return collectField(isEmptyString, func() string {
		return prompter.String(prompt, defaultVal)
	})
}

func (g *InstallConfig) collectInt(prompter *Prompter, prompt string, defaultVal int) int {
	return collectField(isEmptyInt, func() int {
		return prompter.Int(prompt, defaultVal)
	})
}

func (g *InstallConfig) collectStringSlice(prompter *Prompter, prompt string, defaultVal []string) []string {
	return collectField(isEmptySlice, func() []string {
		return prompter.StringSlice(prompt, defaultVal)
	})
}

func (g *InstallConfig) collectChoice(prompter *Prompter, prompt string, options []string, defaultVal string) string {
	return collectField(isEmptyString, func() string {
		return prompter.Choice(prompt, options, defaultVal)
	})
}

func (g *InstallConfig) collectDatacenterConfig(prompter *Prompter) {
	fmt.Println("=== Datacenter Configuration ===")
	g.Config.Datacenter.ID = g.collectInt(prompter, "Datacenter ID", g.Config.Datacenter.ID)
	g.Config.Datacenter.Name = g.collectString(prompter, "Datacenter name", g.Config.Datacenter.Name)
	g.Config.Datacenter.City = g.collectString(prompter, "Datacenter city", g.Config.Datacenter.City)
	g.Config.Datacenter.CountryCode = g.collectString(prompter, "Country code", g.Config.Datacenter.CountryCode)
	g.Config.Secrets.BaseDir = g.collectString(prompter, "Secrets base directory", "/root/secrets")
}

func (g *InstallConfig) collectRegistryConfig(prompter *Prompter) {
	fmt.Println("\n=== Container Registry Configuration ===")
	g.Config.Registry.Server = g.collectString(prompter, "Container registry server (e.g., ghcr.io, leave empty to skip)", "")
	if g.Config.Registry.Server != "" {
		g.Config.Registry.ReplaceImagesInBom = prompter.Bool("Replace images in BOM", g.Config.Registry.ReplaceImagesInBom)
		g.Config.Registry.LoadContainerImages = prompter.Bool("Load container images from installer", g.Config.Registry.LoadContainerImages)
	}
}

func (g *InstallConfig) collectPostgresConfig(prompter *Prompter) {
	fmt.Println("\n=== PostgreSQL Configuration ===")
	// // TODO: create mode in generator
	// g.config.Postgres.Mode = g.collectChoice(prompter, "PostgreSQL setup", []string{"install", "external"}, "install")

	// if g.config.Postgres.Mode == "install" {
	// 	g.config.Postgres.Primary.IP = g.collectString(prompter, "Primary PostgreSQL server IP", "10.50.0.2")
	// 	g.config.Postgres.Primary.Hostname = g.collectString(prompter, "Primary PostgreSQL hostname", "pg-primary-node")
	// 	hasReplica := prompter.Bool("Configure PostgreSQL replica", g.config.Postgres.Replica != nil)
	// 	if hasReplica {
	// 		g.config.Postgres.Replica.IP = g.collectString(prompter, "Replica PostgreSQL server IP", "10.50.0.3")
	// 		g.config.Postgres.Replica.Name = g.collectString(prompter, "Replica name (lowercase alphanumeric + underscore only)", "replica1")
	// 	}
	// } else {
	// 	g.config.Postgres.ServerAddress = g.collectString(prompter, "External PostgreSQL server address", "postgres.example.com:5432")
	// }
}

func (g *InstallConfig) collectCephConfig(prompter *Prompter) {
	fmt.Println("\n=== Ceph Configuration ===")
	g.Config.Ceph.NodesSubnet = g.collectString(prompter, "Ceph nodes subnet (CIDR)", "10.53.101.0/24")

	if len(g.Config.Ceph.Hosts) == 0 {
		numHosts := prompter.Int("Number of Ceph hosts", 3)
		g.Config.Ceph.Hosts = make([]files.CephHost, numHosts)
		for i := 0; i < numHosts; i++ {
			fmt.Printf("\nCeph Host %d:\n", i+1)
			g.Config.Ceph.Hosts[i].Hostname = prompter.String("  Hostname (as shown by 'hostname' command)", fmt.Sprintf("ceph-node-%d", i))
			g.Config.Ceph.Hosts[i].IPAddress = prompter.String("  IP address", fmt.Sprintf("10.53.101.%d", i+2))
			g.Config.Ceph.Hosts[i].IsMaster = (i == 0)
		}
	} else {
		g.Config.Ceph.Hosts = make([]files.CephHost, len(g.Config.Ceph.Hosts))
		for i, host := range g.Config.Ceph.Hosts {
			g.Config.Ceph.Hosts[i] = files.CephHost(host)
		}
	}
}

func (g *InstallConfig) collectK8sConfig(prompter *Prompter) {
	fmt.Println("\n=== Kubernetes Configuration ===")
	g.Config.Kubernetes.ManagedByCodesphere = prompter.Bool("Use Codesphere-managed Kubernetes (k0s)", g.Config.Kubernetes.ManagedByCodesphere)

	if g.Config.Kubernetes.ManagedByCodesphere {
		g.Config.Kubernetes.APIServerHost = g.collectString(prompter, "Kubernetes API server host (LB/DNS/IP)", "10.50.0.2")
		// TODO: convert existing config params to string array and after collection convert back
		// g.config.Kubernetes.ControlPlanes = g.collectStringSlice(prompter, "Control plane IP addresses (comma-separated)", []string{"10.50.0.2"})
		// g.config.Kubernetes.Workers = g.collectStringSlice(prompter, "Worker node IP addresses (comma-separated)", []string{"10.50.0.2", "10.50.0.3", "10.50.0.4"})
	} else {
		g.Config.Kubernetes.PodCIDR = g.collectString(prompter, "Pod CIDR of external cluster", "100.96.0.0/11")
		g.Config.Kubernetes.ServiceCIDR = g.collectString(prompter, "Service CIDR of external cluster", "100.64.0.0/13")
		fmt.Println("Note: You'll need to provide kubeconfig in the vault file for external Kubernetes")
	}
}

func (g *InstallConfig) collectGatewayConfig(prompter *Prompter) {
	fmt.Println("\n=== Cluster Gateway Configuration ===")
	g.Config.Cluster.Gateway.ServiceType = g.collectChoice(prompter, "Gateway service type", []string{"LoadBalancer", "ExternalIP"}, "LoadBalancer")
	if g.Config.Cluster.Gateway.ServiceType == "ExternalIP" {
		g.Config.Cluster.Gateway.IPAddresses = g.collectStringSlice(prompter, "Gateway IP addresses (comma-separated)", []string{"10.51.0.2", "10.51.0.3"})
	}

	g.Config.Cluster.PublicGateway.ServiceType = g.collectChoice(prompter, "Public gateway service type", []string{"LoadBalancer", "ExternalIP"}, "LoadBalancer")
	if g.Config.Cluster.PublicGateway.ServiceType == "ExternalIP" {
		g.Config.Cluster.PublicGateway.IPAddresses = g.collectStringSlice(prompter, "Public gateway IP addresses (comma-separated)", []string{"10.52.0.2", "10.52.0.3"})
	}
}

func (g *InstallConfig) collectMetalLBConfig(prompter *Prompter) {
	fmt.Println("\n=== MetalLB Configuration (Optional) ===")

	g.Config.MetalLB.Enabled = prompter.Bool("Enable MetalLB", g.Config.MetalLB.Enabled)

	if g.Config.MetalLB.Enabled {
		defaultNumPools := len(g.Config.MetalLB.Pools)
		if defaultNumPools == 0 {
			defaultNumPools = 1
		}
		numPools := prompter.Int("Number of MetalLB IP pools", defaultNumPools)

		g.Config.MetalLB.Pools = make([]files.MetalLBPoolDef, numPools)
		for i := 0; i < numPools; i++ {
			fmt.Printf("\nMetalLB Pool %d:\n", i+1)

			defaultName := fmt.Sprintf("pool-%d", i+1)
			var defaultIPs []string
			if i < len(g.Config.MetalLB.Pools) {
				defaultName = g.Config.MetalLB.Pools[i].Name
				defaultIPs = g.Config.MetalLB.Pools[i].IPAddresses
			}
			if len(defaultIPs) == 0 {
				defaultIPs = []string{"10.10.10.100-10.10.10.200"}
			}

			poolName := prompter.String("  Pool name", defaultName)
			poolIPs := prompter.StringSlice("  IP addresses/ranges (comma-separated)", defaultIPs)
			g.Config.MetalLB.Pools[i] = files.MetalLBPoolDef{
				Name:        poolName,
				IPAddresses: poolIPs,
			}
		}
	}
}

func (g *InstallConfig) collectCodesphereConfig(prompter *Prompter) {
	fmt.Println("\n=== Codesphere Application Configuration ===")
	g.Config.Codesphere.Domain = g.collectString(prompter, "Main Codesphere domain", "codesphere.yourcompany.com")
	g.Config.Codesphere.WorkspaceHostingBaseDomain = g.collectString(prompter, "Workspace base domain (*.domain should point to public gateway)", "ws.yourcompany.com")
	g.Config.Codesphere.PublicIP = g.collectString(prompter, "Primary public IP for workspaces", "")
	g.Config.Codesphere.CustomDomains.CNameBaseDomain = g.collectString(prompter, "Custom domain CNAME base", "custom.yourcompany.com")
	g.Config.Codesphere.DNSServers = g.collectStringSlice(prompter, "DNS servers (comma-separated)", []string{"1.1.1.1", "8.8.8.8"})

	fmt.Println("\n=== Workspace Plans Configuration ===")
	g.Config.Codesphere.WorkspaceImages.Agent.BomRef = g.collectString(prompter, "Workspace agent image BOM reference", "workspace-agent-24.04")
	hostingPlan := files.HostingPlan{}
	hostingPlan.CPUTenth = g.collectInt(prompter, "Hosting plan CPU (tenths, e.g., 10 = 1 core)", 10)
	hostingPlan.MemoryMb = g.collectInt(prompter, "Hosting plan memory (MB)", 2048)
	hostingPlan.StorageMb = g.collectInt(prompter, "Hosting plan storage (MB)", 20480)
	hostingPlan.TempStorageMb = g.collectInt(prompter, "Hosting plan temp storage (MB)", 1024)

	workspacePlan := files.WorkspacePlan{
		HostingPlanID: 1,
	}
	workspacePlan.Name = g.collectString(prompter, "Workspace plan name", "Standard Developer")
	workspacePlan.MaxReplicas = g.collectInt(prompter, "Max replicas per workspace", 3)

	g.Config.Codesphere.Plans = files.PlansConfig{
		HostingPlans: map[int]files.HostingPlan{
			1: hostingPlan,
		},
		WorkspacePlans: map[int]files.WorkspacePlan{
			1: workspacePlan,
		},
	}
}
