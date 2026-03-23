// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"log"

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
	g.collectACMEConfig(prompter)
	g.collectCodesphereConfig(prompter)

	return nil
}

func (g *InstallConfig) collectString(prompter *Prompter, prompt, defaultVal string) string {
	return prompter.String(prompt, defaultVal)
}

func (g *InstallConfig) collectInt(prompter *Prompter, prompt string, defaultVal int) int {
	return prompter.Int(prompt, defaultVal)
}

func (g *InstallConfig) collectStringSlice(prompter *Prompter, prompt string, defaultVal []string) []string {
	return prompter.StringSlice(prompt, defaultVal)
}

func (g *InstallConfig) collectChoice(prompter *Prompter, prompt string, options []string, defaultVal string) string {
	return prompter.Choice(prompt, options, defaultVal)
}

func k8sNodesToStringSlice(nodes []files.K8sNode) []string {
	ips := make([]string, len(nodes))
	for i, node := range nodes {
		ips[i] = node.IPAddress
	}
	return ips
}

func stringSliceToK8sNodes(ips []string) []files.K8sNode {
	nodes := make([]files.K8sNode, len(ips))
	for i, ip := range ips {
		nodes[i] = files.K8sNode{IPAddress: ip}
	}
	return nodes
}

func (g *InstallConfig) collectDatacenterConfig(prompter *Prompter) {
	log.Println("=== Datacenter Configuration ===")
	g.Config.Datacenter.ID = g.collectInt(prompter, "Datacenter ID", g.Config.Datacenter.ID)
	g.Config.Datacenter.Name = g.collectString(prompter, "Datacenter name", g.Config.Datacenter.Name)
	g.Config.Datacenter.City = g.collectString(prompter, "Datacenter city", g.Config.Datacenter.City)
	g.Config.Datacenter.CountryCode = g.collectString(prompter, "Country code", g.Config.Datacenter.CountryCode)
	g.Config.Secrets.BaseDir = g.collectString(prompter, "Secrets base directory", "/root/secrets")
}

func (g *InstallConfig) collectRegistryConfig(prompter *Prompter) {
	log.Println("\n=== Container Registry Configuration ===")
	g.Config.Registry.Server = g.collectString(prompter, "Container registry server (e.g., ghcr.io, leave empty to skip)", "")
	if g.Config.Registry.Server != "" {
		g.Config.Registry.ReplaceImagesInBom = prompter.Bool("Replace images in BOM", g.Config.Registry.ReplaceImagesInBom)
		g.Config.Registry.LoadContainerImages = prompter.Bool("Load container images from installer", g.Config.Registry.LoadContainerImages)
	}
}

func (g *InstallConfig) collectPostgresConfig(prompter *Prompter) {
	log.Println("\n=== PostgreSQL Configuration ===")
	g.Config.Postgres.Mode = g.collectChoice(prompter, "PostgreSQL setup", []string{"install", "external"}, "install")

	if g.Config.Postgres.Mode == "install" {
		if g.Config.Postgres.Primary == nil {
			g.Config.Postgres.Primary = &files.PostgresPrimaryConfig{}
		}
		defaultPrimaryIP := g.Config.Postgres.Primary.IP
		if defaultPrimaryIP == "" {
			defaultPrimaryIP = "10.50.0.2"
		}
		defaultPrimaryHostname := g.Config.Postgres.Primary.Hostname
		if defaultPrimaryHostname == "" {
			defaultPrimaryHostname = "pg-primary-node"
		}
		g.Config.Postgres.Primary.IP = g.collectString(prompter, "Primary PostgreSQL server IP", defaultPrimaryIP)
		g.Config.Postgres.Primary.Hostname = g.collectString(prompter, "Primary PostgreSQL hostname", defaultPrimaryHostname)

		hasReplica := prompter.Bool("Configure PostgreSQL replica", g.Config.Postgres.Replica != nil)
		if hasReplica {
			if g.Config.Postgres.Replica == nil {
				g.Config.Postgres.Replica = &files.PostgresReplicaConfig{}
			}
			g.Config.Postgres.Replica.IP = g.collectString(prompter, "Replica PostgreSQL server IP", "10.50.0.3")
			g.Config.Postgres.Replica.Name = g.collectString(prompter, "Replica name (lowercase alphanumeric + underscore only)", "replica1")
		} else {
			g.Config.Postgres.Replica = nil
		}
	} else {
		g.Config.Postgres.ServerAddress = g.collectString(prompter, "External PostgreSQL server address", "postgres.example.com:5432")
	}
}

func (g *InstallConfig) collectCephConfig(prompter *Prompter) {
	log.Println("\n=== Ceph Configuration ===")
	g.Config.Ceph.NodesSubnet = g.collectString(prompter, "Ceph nodes subnet (CIDR)", "10.53.101.0/24")

	if len(g.Config.Ceph.Hosts) == 0 {
		numHosts := prompter.Int("Number of Ceph hosts", 3)
		g.Config.Ceph.Hosts = make([]files.CephHost, numHosts)
		for i := 0; i < numHosts; i++ {
			log.Printf("\nCeph Host %d:\n", i+1)
			g.Config.Ceph.Hosts[i].Hostname = prompter.String("  Hostname (as shown by 'hostname' command)", fmt.Sprintf("ceph-node-%d", i))
			g.Config.Ceph.Hosts[i].IPAddress = prompter.String("  IP address", fmt.Sprintf("10.53.101.%d", i+2))
			g.Config.Ceph.Hosts[i].IsMaster = (i == 0)
		}
	} else {
		existingHosts := g.Config.Ceph.Hosts
		g.Config.Ceph.Hosts = make([]files.CephHost, len(existingHosts))
		for i, host := range existingHosts {
			g.Config.Ceph.Hosts[i] = files.CephHost(host)
		}
	}
}

func (g *InstallConfig) collectK8sConfig(prompter *Prompter) {
	log.Println("\n=== Kubernetes Configuration ===")
	g.Config.Kubernetes.ManagedByCodesphere = prompter.Bool("Use Codesphere-managed Kubernetes (k0s)", g.Config.Kubernetes.ManagedByCodesphere)

	if g.Config.Kubernetes.ManagedByCodesphere {
		defaultAPIServerHost := g.Config.Kubernetes.APIServerHost
		if defaultAPIServerHost == "" {
			defaultAPIServerHost = "10.50.0.2"
		}
		g.Config.Kubernetes.APIServerHost = g.collectString(prompter, "Kubernetes API server host (LB/DNS/IP)", defaultAPIServerHost)

		defaultControlPlanes := k8sNodesToStringSlice(g.Config.Kubernetes.ControlPlanes)
		if len(defaultControlPlanes) == 0 {
			defaultControlPlanes = []string{"10.50.0.2"}
		}
		defaultWorkers := k8sNodesToStringSlice(g.Config.Kubernetes.Workers)

		controlPlaneIPs := g.collectStringSlice(prompter, "Control plane IP addresses (comma-separated)", defaultControlPlanes)
		workerIPs := g.collectStringSlice(prompter, "Worker node IP addresses (comma-separated)", defaultWorkers)

		g.Config.Kubernetes.ControlPlanes = stringSliceToK8sNodes(controlPlaneIPs)
		g.Config.Kubernetes.Workers = stringSliceToK8sNodes(workerIPs)
		g.Config.Kubernetes.NeedsKubeConfig = false
	} else {
		g.Config.Kubernetes.PodCIDR = g.collectString(prompter, "Pod CIDR of external cluster", "100.96.0.0/11")
		g.Config.Kubernetes.ServiceCIDR = g.collectString(prompter, "Service CIDR of external cluster", "100.64.0.0/13")
		g.Config.Kubernetes.NeedsKubeConfig = true
		log.Println("Note: You'll need to provide kubeconfig in the vault file for external Kubernetes")
	}
}

func (g *InstallConfig) collectGatewayConfig(prompter *Prompter) {
	log.Println("\n=== Cluster Gateway Configuration ===")
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
	log.Println("\n=== MetalLB Configuration (Optional) ===")

	g.Config.MetalLB.Enabled = prompter.Bool("Enable MetalLB", g.Config.MetalLB.Enabled)

	if g.Config.MetalLB.Enabled {
		defaultNumPools := len(g.Config.MetalLB.Pools)
		if defaultNumPools == 0 {
			defaultNumPools = 1
		}
		numPools := prompter.Int("Number of MetalLB IP pools", defaultNumPools)

		g.Config.MetalLB.Pools = make([]files.MetalLBPoolDef, numPools)
		for i := 0; i < numPools; i++ {
			log.Printf("\nMetalLB Pool %d:\n", i+1)

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

func (g *InstallConfig) collectACMEConfig(prompter *Prompter) {
	log.Println("\n=== ACME Certificate Configuration (Optional) ===")

	// Initialize ACME config if it doesn't exist
	if g.Config.Cluster.Certificates.ACME == nil {
		g.Config.Cluster.Certificates.ACME = &files.ACMEConfig{}
	}

	g.Config.Cluster.Certificates.ACME.Enabled = prompter.Bool("Enable ACME certificate issuer (e.g., Let's Encrypt)", g.Config.Cluster.Certificates.ACME.Enabled)

	// Early exit if ACME is disabled
	if !g.Config.Cluster.Certificates.ACME.Enabled {
		g.Config.Cluster.Certificates.ACME = nil
		return
	}

	defaultIssuerName := g.Config.Cluster.Certificates.ACME.Name
	if defaultIssuerName == "" {
		defaultIssuerName = "acme-issuer"
	}
	g.Config.Cluster.Certificates.ACME.Name = g.collectString(prompter, "ACME issuer name", defaultIssuerName)

	defaultEmail := g.Config.Cluster.Certificates.ACME.Email
	if defaultEmail == "" {
		defaultEmail = "admin@example.com"
	}
	g.Config.Cluster.Certificates.ACME.Email = g.collectString(prompter, "Email address for ACME account registration", defaultEmail)

	defaultServer := g.Config.Cluster.Certificates.ACME.Server
	if defaultServer == "" {
		defaultServer = "https://acme-v02.api.letsencrypt.org/directory"
	}
	g.Config.Cluster.Certificates.ACME.Server = g.collectString(prompter, "ACME server URL", defaultServer)

	// External Account Binding (EAB)
	log.Println("\n--- External Account Binding (Optional) ---")
	hasEAB := prompter.Bool("Configure External Account Binding (required by some ACME CAs)", g.Config.Cluster.Certificates.ACME.EABKeyID != "")

	g.Config.Cluster.Certificates.ACME.EABKeyID = ""
	g.Config.Cluster.Certificates.ACME.EABMacKey = ""
	if hasEAB {
		g.Config.Cluster.Certificates.ACME.EABKeyID = g.collectString(prompter, "EAB Key ID", g.Config.Cluster.Certificates.ACME.EABKeyID)
		g.Config.Cluster.Certificates.ACME.EABMacKey = g.collectString(prompter, "EAB MAC Key", g.Config.Cluster.Certificates.ACME.EABMacKey)
	}

	// DNS-01 Challenge Configuration
	log.Println("\n--- DNS-01 Challenge Configuration (Optional) ---")
	if g.Config.Cluster.Certificates.ACME.Solver.DNS01 == nil {
		g.Config.Cluster.Certificates.ACME.Solver.DNS01 = &files.ACMEDNS01Solver{}
	}

	useDNS01 := prompter.Bool("Configure DNS-01 challenge solver", g.Config.Cluster.Certificates.ACME.Solver.DNS01.Provider != "")
	if !useDNS01 {
		g.Config.Cluster.Certificates.ACME.Solver.DNS01 = nil
		return
	}
	providerOptions := []string{"route53", "cloudflare", "azure", "gcp", "other"}
	defaultProvider := g.Config.Cluster.Certificates.ACME.Solver.DNS01.Provider
	if defaultProvider == "" {
		defaultProvider = "cloudflare"
	}
	g.Config.Cluster.Certificates.ACME.Solver.DNS01.Provider = g.collectChoice(prompter, "DNS provider", providerOptions, defaultProvider)
	log.Println("Note: Additional DNS provider configuration will need to be added to the vault file.")
	log.Println("Provider config and secrets should be added manually after generation.")
}

func (g *InstallConfig) collectCodesphereConfig(prompter *Prompter) {
	log.Println("\n=== Codesphere Application Configuration ===")
	defaultDomain := g.Config.Codesphere.Domain
	if defaultDomain == "" {
		defaultDomain = "codesphere.yourcompany.com"
	}
	defaultWorkspaceDomain := g.Config.Codesphere.WorkspaceHostingBaseDomain
	if defaultWorkspaceDomain == "" {
		defaultWorkspaceDomain = "ws.yourcompany.com"
	}
	defaultCustomDomain := g.Config.Codesphere.CustomDomains.CNameBaseDomain
	if defaultCustomDomain == "" {
		defaultCustomDomain = "custom.yourcompany.com"
	}
	defaultDNSServers := g.Config.Codesphere.DNSServers
	if len(defaultDNSServers) == 0 {
		defaultDNSServers = []string{"1.1.1.1", "8.8.8.8"}
	}
	g.Config.Codesphere.Domain = g.collectString(prompter, "Main Codesphere domain", defaultDomain)
	g.Config.Codesphere.WorkspaceHostingBaseDomain = g.collectString(prompter, "Workspace base domain (*.domain should point to public gateway)", defaultWorkspaceDomain)
	g.Config.Codesphere.PublicIP = g.collectString(prompter, "Primary public IP for workspaces", "")
	g.Config.Codesphere.CustomDomains.CNameBaseDomain = g.collectString(prompter, "Custom domain CNAME base", defaultCustomDomain)
	g.Config.Codesphere.DNSServers = g.collectStringSlice(prompter, "DNS servers (comma-separated)", defaultDNSServers)

	log.Println("\n=== Workspace Plans Configuration ===")

	if g.Config.Codesphere.WorkspaceImages == nil {
		g.Config.Codesphere.WorkspaceImages = &files.WorkspaceImagesConfig{}
	}
	if g.Config.Codesphere.WorkspaceImages.Agent == nil {
		g.Config.Codesphere.WorkspaceImages.Agent = &files.ImageRef{}
	}

	defaultBomRef := g.Config.Codesphere.WorkspaceImages.Agent.BomRef
	if defaultBomRef == "" {
		defaultBomRef = "workspace-agent-24.04"
	}
	g.Config.Codesphere.WorkspaceImages.Agent.BomRef = g.collectString(prompter, "Workspace agent image BOM reference", defaultBomRef)
	hostingPlan := files.HostingPlan{}
	hostingPlan.CPUTenth = g.collectInt(prompter, "Hosting plan CPU (tenths, e.g., 10 = 1 core)", 10)
	hostingPlan.MemoryMb = g.collectInt(prompter, "Hosting plan memory (MB)", 2048)
	hostingPlan.StorageMb = g.collectInt(prompter, "Hosting plan storage (MB)", 20480)
	hostingPlan.TempStorageMb = g.collectInt(prompter, "Hosting plan temp storage (MB)", 1024)

	workspacePlan := files.WorkspacePlan{
		HostingPlanID: 1,
	}
	defaultWorkspacePlanName := "Standard Developer"
	defaultMaxReplicas := 3
	if existingPlan, ok := g.Config.Codesphere.Plans.WorkspacePlans[1]; ok {
		if existingPlan.Name != "" {
			defaultWorkspacePlanName = existingPlan.Name
		}
		if existingPlan.MaxReplicas > 0 {
			defaultMaxReplicas = existingPlan.MaxReplicas
		}
	}
	workspacePlan.Name = g.collectString(prompter, "Workspace plan name", defaultWorkspacePlanName)
	workspacePlan.MaxReplicas = g.collectInt(prompter, "Max replicas per workspace", defaultMaxReplicas)

	g.Config.Codesphere.Plans = files.PlansConfig{
		HostingPlans: map[int]files.HostingPlan{
			1: hostingPlan,
		},
		WorkspacePlans: map[int]files.WorkspacePlan{
			1: workspacePlan,
		},
	}

	g.collectOpenBaoConfig(prompter)
}

func (g *InstallConfig) collectOpenBaoConfig(prompter *Prompter) {
	log.Println("\n=== OpenBao Configuration (Optional) ===")
	hasOpenBao := prompter.Bool("Configure OpenBao integration", g.Config.Codesphere.OpenBao != nil && g.Config.Codesphere.OpenBao.URI != "")
	if !hasOpenBao {
		g.Config.Codesphere.OpenBao = nil
		return
	}

	if g.Config.Codesphere.OpenBao == nil {
		g.Config.Codesphere.OpenBao = &files.OpenBaoConfig{}
	}

	g.Config.Codesphere.OpenBao.URI = g.collectString(prompter, "OpenBao URI (e.g., https://openbao.example.com)", "")
	g.Config.Codesphere.OpenBao.Engine = g.collectString(prompter, "OpenBao engine name", "cs-secrets-engine")
	g.Config.Codesphere.OpenBao.User = g.collectString(prompter, "OpenBao username", "admin")
	g.Config.Codesphere.OpenBao.Password = g.collectString(prompter, "OpenBao password", "")
}
