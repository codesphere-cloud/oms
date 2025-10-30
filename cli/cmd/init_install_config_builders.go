// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func (c *InitInstallConfigCmd) collectConfiguration() error {
	fmt.Println("=== Datacenter Configuration ===")
	if c.Opts.DatacenterID == 0 {
		c.Opts.DatacenterID = c.promptInt("Datacenter ID", 1)
	}
	if c.Opts.DatacenterName == "" {
		c.Opts.DatacenterName = c.promptString("Datacenter name", "main")
	}
	if c.Opts.DatacenterCity == "" {
		c.Opts.DatacenterCity = c.promptString("Datacenter city", "Karlsruhe")
	}
	if c.Opts.DatacenterCountryCode == "" {
		c.Opts.DatacenterCountryCode = c.promptString("Country code", "DE")
	}

	if c.Opts.SecretsBaseDir == "" {
		c.Opts.SecretsBaseDir = c.promptString("Secrets base directory", "/root/secrets")
	}

	fmt.Println("\n=== Container Registry Configuration ===")
	if c.Opts.RegistryServer == "" {
		c.Opts.RegistryServer = c.promptString("Container registry server (e.g., ghcr.io, leave empty to skip)", "")
	}
	if c.Opts.RegistryServer != "" {
		if !c.Opts.NonInteractive {
			c.Opts.RegistryReplaceImages = c.promptBool("Replace images in BOM", true)
			c.Opts.RegistryLoadContainerImgs = c.promptBool("Load container images from installer", false)
		}
	}

	fmt.Println("\n=== PostgreSQL Configuration ===")
	if c.Opts.PostgresMode == "" {
		c.Opts.PostgresMode = c.promptChoice("PostgreSQL setup", []string{"install", "external"}, "install")
	}

	if c.Opts.PostgresMode == "install" {
		if c.Opts.PostgresPrimaryIP == "" {
			c.Opts.PostgresPrimaryIP = c.promptString("Primary PostgreSQL server IP", "10.50.0.2")
		}
		if c.Opts.PostgresPrimaryHost == "" {
			c.Opts.PostgresPrimaryHost = c.promptString("Primary PostgreSQL hostname", "pg-primary-node")
		}
		if !c.Opts.NonInteractive {
			hasReplica := c.promptBool("Configure PostgreSQL replica", true)
			if hasReplica {
				if c.Opts.PostgresReplicaIP == "" {
					c.Opts.PostgresReplicaIP = c.promptString("Replica PostgreSQL server IP", "10.50.0.3")
				}
				if c.Opts.PostgresReplicaName == "" {
					c.Opts.PostgresReplicaName = c.promptString("Replica name (lowercase alphanumeric + underscore only)", "replica1")
				}
			}
		}
		c.Opts.GenerateKeys = true
	} else {
		if c.Opts.PostgresExternal == "" {
			c.Opts.PostgresExternal = c.promptString("External PostgreSQL server address", "postgres.example.com:5432")
		}
	}

	fmt.Println("\n=== Ceph Configuration ===")
	if c.Opts.CephSubnet == "" {
		c.Opts.CephSubnet = c.promptString("Ceph nodes subnet (CIDR)", "10.53.101.0/24")
	}

	if len(c.Opts.CephHosts) == 0 {
		numHosts := c.promptInt("Number of Ceph hosts", 3)
		c.Opts.CephHosts = make([]CephHostConfig, numHosts)
		for i := 0; i < numHosts; i++ {
			fmt.Printf("\nCeph Host %d:\n", i+1)
			c.Opts.CephHosts[i].Hostname = c.promptString("  Hostname (as shown by 'hostname' command)", fmt.Sprintf("ceph-node-%d", i))
			c.Opts.CephHosts[i].IPAddress = c.promptString("  IP address", fmt.Sprintf("10.53.101.%d", i+2))
			c.Opts.CephHosts[i].IsMaster = (i == 0)
		}
	}
	c.Opts.GenerateKeys = true

	fmt.Println("\n=== Kubernetes Configuration ===")
	if !c.Opts.NonInteractive {
		c.Opts.K8sManaged = c.promptBool("Use Codesphere-managed Kubernetes (k0s)", true)
	}

	if c.Opts.K8sManaged {
		if c.Opts.K8sAPIServer == "" {
			c.Opts.K8sAPIServer = c.promptString("Kubernetes API server host (LB/DNS/IP)", "10.50.0.2")
		}
		if len(c.Opts.K8sControlPlane) == 0 {
			c.Opts.K8sControlPlane = c.promptStringSlice("Control plane IP addresses (comma-separated)", []string{"10.50.0.2"})
		}
		if len(c.Opts.K8sWorkers) == 0 {
			c.Opts.K8sWorkers = c.promptStringSlice("Worker node IP addresses (comma-separated)", []string{"10.50.0.2", "10.50.0.3", "10.50.0.4"})
		}
	} else {
		if c.Opts.K8sPodCIDR == "" {
			c.Opts.K8sPodCIDR = c.promptString("Pod CIDR of external cluster", "100.96.0.0/11")
		}
		if c.Opts.K8sServiceCIDR == "" {
			c.Opts.K8sServiceCIDR = c.promptString("Service CIDR of external cluster", "100.64.0.0/13")
		}
		fmt.Println("Note: You'll need to provide kubeconfig in the vault file for external Kubernetes")
	}

	fmt.Println("\n=== Cluster Gateway Configuration ===")
	if c.Opts.ClusterGatewayType == "" {
		c.Opts.ClusterGatewayType = c.promptChoice("Gateway service type", []string{"LoadBalancer", "ExternalIP"}, "LoadBalancer")
	}
	if c.Opts.ClusterGatewayType == "ExternalIP" && len(c.Opts.ClusterGatewayIPs) == 0 {
		c.Opts.ClusterGatewayIPs = c.promptStringSlice("Gateway IP addresses (comma-separated)", []string{"10.51.0.2", "10.51.0.3"})
	}

	if c.Opts.ClusterPublicGatewayType == "" {
		c.Opts.ClusterPublicGatewayType = c.promptChoice("Public gateway service type", []string{"LoadBalancer", "ExternalIP"}, "LoadBalancer")
	}
	if c.Opts.ClusterPublicGatewayType == "ExternalIP" && len(c.Opts.ClusterPublicGatewayIPs) == 0 {
		c.Opts.ClusterPublicGatewayIPs = c.promptStringSlice("Public gateway IP addresses (comma-separated)", []string{"10.52.0.2", "10.52.0.3"})
	}

	fmt.Println("\n=== MetalLB Configuration (Optional) ===")
	if !c.Opts.NonInteractive {
		c.Opts.MetalLBEnabled = c.promptBool("Enable MetalLB", false)
		if c.Opts.MetalLBEnabled {
			numPools := c.promptInt("Number of MetalLB IP pools", 1)
			c.Opts.MetalLBPools = make([]MetalLBPool, numPools)
			for i := 0; i < numPools; i++ {
				fmt.Printf("\nMetalLB Pool %d:\n", i+1)
				c.Opts.MetalLBPools[i].Name = c.promptString("  Pool name", fmt.Sprintf("pool-%d", i+1))
				c.Opts.MetalLBPools[i].IPAddresses = c.promptStringSlice("  IP addresses/ranges (comma-separated)", []string{"10.10.10.100-10.10.10.200"})
			}
		}
	}

	fmt.Println("\n=== Codesphere Application Configuration ===")
	if c.Opts.CodesphereDomain == "" {
		c.Opts.CodesphereDomain = c.promptString("Main Codesphere domain", "codesphere.yourcompany.com")
	}
	if c.Opts.CodesphereWorkspaceBaseDomain == "" {
		c.Opts.CodesphereWorkspaceBaseDomain = c.promptString("Workspace base domain (*.domain should point to public gateway)", "ws.yourcompany.com")
	}
	if c.Opts.CodespherePublicIP == "" {
		c.Opts.CodespherePublicIP = c.promptString("Primary public IP for workspaces", "")
	}
	if c.Opts.CodesphereCustomDomainBaseDomain == "" {
		c.Opts.CodesphereCustomDomainBaseDomain = c.promptString("Custom domain CNAME base", "custom.yourcompany.com")
	}
	if len(c.Opts.CodesphereDNSServers) == 0 {
		c.Opts.CodesphereDNSServers = c.promptStringSlice("DNS servers (comma-separated)", []string{"1.1.1.1", "8.8.8.8"})
	}

	fmt.Println("\n=== Workspace Plans Configuration ===")
	if c.Opts.CodesphereWorkspaceImageBomRef == "" {
		c.Opts.CodesphereWorkspaceImageBomRef = c.promptString("Workspace agent image BOM reference", "workspace-agent-24.04")
	}
	if c.Opts.CodesphereHostingPlanCPU == 0 {
		c.Opts.CodesphereHostingPlanCPU = c.promptInt("Hosting plan CPU (tenths, e.g., 10 = 1 core)", 10)
	}
	if c.Opts.CodesphereHostingPlanMemory == 0 {
		c.Opts.CodesphereHostingPlanMemory = c.promptInt("Hosting plan memory (MB)", 2048)
	}
	if c.Opts.CodesphereHostingPlanStorage == 0 {
		c.Opts.CodesphereHostingPlanStorage = c.promptInt("Hosting plan storage (MB)", 20480)
	}
	if c.Opts.CodesphereHostingPlanTempStorage == 0 {
		c.Opts.CodesphereHostingPlanTempStorage = c.promptInt("Hosting plan temp storage (MB)", 1024)
	}
	if c.Opts.CodesphereWorkspacePlanName == "" {
		c.Opts.CodesphereWorkspacePlanName = c.promptString("Workspace plan name", "Standard Developer")
	}
	if c.Opts.CodesphereWorkspacePlanMaxReplica == 0 {
		c.Opts.CodesphereWorkspacePlanMaxReplica = c.promptInt("Max replicas per workspace", 3)
	}

	return nil
}

func (c *InitInstallConfigCmd) buildGen0Config(secrets *GeneratedSecrets) *Gen0Config {
	config := &Gen0Config{
		DataCenter: DataCenterConfig{
			ID:          c.Opts.DatacenterID,
			Name:        c.Opts.DatacenterName,
			City:        c.Opts.DatacenterCity,
			CountryCode: c.Opts.DatacenterCountryCode,
		},
		Secrets: SecretsConfig{
			BaseDir: c.Opts.SecretsBaseDir,
		},
		Ceph: CephConfig{
			CephAdmSSHKey: CephSSHKey{
				PublicKey: secrets.CephSSHPublicKey,
			},
			NodesSubnet: c.Opts.CephSubnet,
			Hosts:       make([]CephHost, len(c.Opts.CephHosts)),
			OSDs: []CephOSD{
				{
					SpecID: "default",
					Placement: CephPlacement{
						HostPattern: "*",
					},
					DataDevices: CephDataDevices{
						Size:  "240G:300G",
						Limit: 1,
					},
					DBDevices: CephDBDevices{
						Size:  "120G:150G",
						Limit: 1,
					},
				},
			},
		},
		Cluster: ClusterConfig{
			Certificates: ClusterCertificates{
				CA: CAConfig{
					Algorithm:   "RSA",
					KeySizeBits: 2048,
					CertPem:     secrets.IngressCACert,
				},
			},
			Gateway: GatewayConfig{
				ServiceType: c.Opts.ClusterGatewayType,
				IPAddresses: c.Opts.ClusterGatewayIPs,
			},
			PublicGateway: GatewayConfig{
				ServiceType: c.Opts.ClusterPublicGatewayType,
				IPAddresses: c.Opts.ClusterPublicGatewayIPs,
			},
		},
		Codesphere: CodesphereConfig{
			Domain:                     c.Opts.CodesphereDomain,
			WorkspaceHostingBaseDomain: c.Opts.CodesphereWorkspaceBaseDomain,
			PublicIP:                   c.Opts.CodespherePublicIP,
			CustomDomains: CustomDomainsConfig{
				CNameBaseDomain: c.Opts.CodesphereCustomDomainBaseDomain,
			},
			DNSServers:  c.Opts.CodesphereDNSServers,
			Experiments: []string{},
			DeployConfig: DeployConfig{
				Images: map[string]DeployImage{
					"ubuntu-24.04": {
						Name:           "Ubuntu 24.04",
						SupportedUntil: "2028-05-31",
						Flavors: map[string]DeployFlavor{
							"default": {
								Image: ImageRef{
									BomRef: c.Opts.CodesphereWorkspaceImageBomRef,
								},
								Pool: map[int]int{1: 1},
							},
						},
					},
				},
			},
			Plans: PlansConfig{
				HostingPlans: map[int]HostingPlan{
					1: {
						CPUTenth:      c.Opts.CodesphereHostingPlanCPU,
						GPUParts:      0,
						MemoryMb:      c.Opts.CodesphereHostingPlanMemory,
						StorageMb:     c.Opts.CodesphereHostingPlanStorage,
						TempStorageMb: c.Opts.CodesphereHostingPlanTempStorage,
					},
				},
				WorkspacePlans: map[int]WorkspacePlan{
					1: {
						Name:          c.Opts.CodesphereWorkspacePlanName,
						HostingPlanID: 1,
						MaxReplicas:   c.Opts.CodesphereWorkspacePlanMaxReplica,
						OnDemand:      true,
					},
				},
			},
		},
	}

	for i, host := range c.Opts.CephHosts {
		config.Ceph.Hosts[i] = CephHost(host)
	}

	if c.Opts.RegistryServer != "" {
		config.Registry = &RegistryConfig{
			Server:              c.Opts.RegistryServer,
			ReplaceImagesInBom:  c.Opts.RegistryReplaceImages,
			LoadContainerImages: c.Opts.RegistryLoadContainerImgs,
		}
	}

	if c.Opts.PostgresMode == "install" {
		config.Postgres = PostgresConfig{
			CACertPem: secrets.PostgresCACert,
			Primary: &PostgresPrimaryConfig{
				SSLConfig: SSLConfig{
					ServerCertPem: secrets.PostgresPrimaryCert,
				},
				IP:       c.Opts.PostgresPrimaryIP,
				Hostname: c.Opts.PostgresPrimaryHost,
			},
		}
		if c.Opts.PostgresReplicaIP != "" {
			config.Postgres.Replica = &PostgresReplicaConfig{
				IP:   c.Opts.PostgresReplicaIP,
				Name: c.Opts.PostgresReplicaName,
				SSLConfig: SSLConfig{
					ServerCertPem: secrets.PostgresReplicaCert,
				},
			}
		}
	} else {
		config.Postgres = PostgresConfig{
			ServerAddress: c.Opts.PostgresExternal,
		}
	}

	config.Kubernetes = KubernetesConfig{
		ManagedByCodesphere: c.Opts.K8sManaged,
	}
	if c.Opts.K8sManaged {
		config.Kubernetes.APIServerHost = c.Opts.K8sAPIServer
		config.Kubernetes.ControlPlanes = make([]K8sNode, len(c.Opts.K8sControlPlane))
		for i, ip := range c.Opts.K8sControlPlane {
			config.Kubernetes.ControlPlanes[i] = K8sNode{IPAddress: ip}
		}
		config.Kubernetes.Workers = make([]K8sNode, len(c.Opts.K8sWorkers))
		for i, ip := range c.Opts.K8sWorkers {
			config.Kubernetes.Workers[i] = K8sNode{IPAddress: ip}
		}
	} else {
		config.Kubernetes.PodCIDR = c.Opts.K8sPodCIDR
		config.Kubernetes.ServiceCIDR = c.Opts.K8sServiceCIDR
	}

	if c.Opts.MetalLBEnabled {
		config.MetalLB = &MetalLBConfig{
			Enabled: true,
			Pools:   make([]MetalLBPoolDef, len(c.Opts.MetalLBPools)),
		}
		for i, pool := range c.Opts.MetalLBPools {
			config.MetalLB.Pools[i] = MetalLBPoolDef(pool)
		}
	}

	config.ManagedServiceBackends = &ManagedServiceBackendsConfig{
		Postgres: make(map[string]interface{}),
	}

	return config
}

func (c *InitInstallConfigCmd) buildGen0Vault(secrets *GeneratedSecrets) *Gen0Vault {
	vault := &Gen0Vault{
		Secrets: []SecretEntry{
			{
				Name: "cephSshPrivateKey",
				File: &SecretFile{
					Name:    "id_rsa",
					Content: secrets.CephSSHPrivateKey,
				},
			},
			{
				Name: "selfSignedCaKeyPem",
				File: &SecretFile{
					Name:    "key.pem",
					Content: secrets.IngressCAKey,
				},
			},
			{
				Name: "domainAuthPrivateKey",
				File: &SecretFile{
					Name:    "key.pem",
					Content: secrets.DomainAuthPrivateKey,
				},
			},
			{
				Name: "domainAuthPublicKey",
				File: &SecretFile{
					Name:    "key.pem",
					Content: secrets.DomainAuthPublicKey,
				},
			},
		},
	}

	if c.Opts.PostgresMode == "install" {
		vault.Secrets = append(vault.Secrets,
			SecretEntry{
				Name: "postgresPassword",
				Fields: &SecretFields{
					Password: secrets.PostgresAdminPassword,
				},
			},
			SecretEntry{
				Name: "postgresReplicaPassword",
				Fields: &SecretFields{
					Password: secrets.PostgresReplicaPassword,
				},
			},
			SecretEntry{
				Name: "postgresPrimaryServerKeyPem",
				File: &SecretFile{
					Name:    "primary.key",
					Content: secrets.PostgresPrimaryKey,
				},
			},
		)
		if c.Opts.PostgresReplicaIP != "" {
			vault.Secrets = append(vault.Secrets, SecretEntry{
				Name: "postgresReplicaServerKeyPem",
				File: &SecretFile{
					Name:    "replica.key",
					Content: secrets.PostgresReplicaKey,
				},
			})
		}
	}

	services := []string{"auth", "deployment", "ide", "marketplace", "payment", "public_api", "team", "workspace"}
	for _, service := range services {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: fmt.Sprintf("postgresUser%s", capitalize(service)),
			Fields: &SecretFields{
				Password: service + "_blue",
			},
		})
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: fmt.Sprintf("postgresPassword%s", capitalize(service)),
			Fields: &SecretFields{
				Password: secrets.PostgresUserPasswords[service],
			},
		})
	}

	vault.Secrets = append(vault.Secrets, SecretEntry{
		Name: "managedServiceSecrets",
		Fields: &SecretFields{
			Password: "[]",
		},
	})

	if c.Opts.RegistryServer != "" {
		vault.Secrets = append(vault.Secrets,
			SecretEntry{
				Name: "registryUsername",
				Fields: &SecretFields{
					Password: "YOUR_REGISTRY_USERNAME",
				},
			},
			SecretEntry{
				Name: "registryPassword",
				Fields: &SecretFields{
					Password: "YOUR_REGISTRY_PASSWORD",
				},
			},
		)
	}

	if !c.Opts.K8sManaged {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "kubeConfig",
			File: &SecretFile{
				Name:    "kubeConfig",
				Content: "# YOUR KUBECONFIG CONTENT HERE\n# Replace this with your actual kubeconfig for the external cluster\n",
			},
		})
	}

	return vault
}

func (c *InitInstallConfigCmd) promptString(prompt, defaultValue string) string {
	if c.Opts.NonInteractive {
		return defaultValue
	}

	reader := bufio.NewReader(os.Stdin)
	if defaultValue != "" {
		fmt.Printf("%s (default: %s): ", prompt, defaultValue)
	} else {
		fmt.Printf("%s: ", prompt)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}
	return input
}

func (c *InitInstallConfigCmd) promptInt(prompt string, defaultValue int) int {
	if c.Opts.NonInteractive {
		return defaultValue
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s (default: %d): ", prompt, defaultValue)

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(input)
	if err != nil {
		fmt.Printf("Invalid number, using default: %d\n", defaultValue)
		return defaultValue
	}
	return value
}

func (c *InitInstallConfigCmd) promptStringSlice(prompt string, defaultValue []string) []string {
	if c.Opts.NonInteractive {
		return defaultValue
	}

	reader := bufio.NewReader(os.Stdin)
	defaultStr := strings.Join(defaultValue, ", ")
	if defaultStr != "" {
		fmt.Printf("%s (default: %s): ", prompt, defaultStr)
	} else {
		fmt.Printf("%s: ", prompt)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}

	parts := strings.Split(input, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return defaultValue
	}
	return result
}

func (c *InitInstallConfigCmd) promptBool(prompt string, defaultValue bool) bool {
	if c.Opts.NonInteractive {
		return defaultValue
	}

	reader := bufio.NewReader(os.Stdin)
	defaultStr := "n"
	if defaultValue {
		defaultStr = "y"
	}
	fmt.Printf("%s (y/n, default: %s): ", prompt, defaultStr)

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		return defaultValue
	}

	return input == "y" || input == "yes"
}

func (c *InitInstallConfigCmd) promptChoice(prompt string, choices []string, defaultValue string) string {
	if c.Opts.NonInteractive {
		return defaultValue
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [%s] (default: %s): ", prompt, strings.Join(choices, "/"), defaultValue)

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		return defaultValue
	}

	for _, choice := range choices {
		if strings.ToLower(choice) == input {
			return choice
		}
	}

	fmt.Printf("Invalid choice, using default: %s\n", defaultValue)
	return defaultValue
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "_", "")
	return strings.ToUpper(s[:1]) + s[1:]
}

func (c *InitInstallConfigCmd) addConfigComments(yamlData []byte) []byte {
	header := `# Codesphere Gen0 Installer Configuration
# Generated by OMS CLI
#
# This file contains the main configuration for installing Codesphere Private Cloud.
# Review and modify as needed before running the installer.
#
# For more information, see the installation documentation.

`
	return append([]byte(header), yamlData...)
}

func (c *InitInstallConfigCmd) addVaultComments(yamlData []byte) []byte {
	header := `# Codesphere Gen0 Installer Secrets
# Generated by OMS CLI
#
# IMPORTANT: This file contains sensitive information!
#
# Before storing or transmitting this file:
# 1. Install SOPS and Age: brew install sops age
# 2. Generate an Age keypair: age-keygen -o age_key.txt
# 3. Encrypt this file:
#    age-keygen -y age_key.txt  # Get public key
#    sops --encrypt --age <PUBLIC_KEY> --in-place prod.vault.yaml
#
# Keep the Age private key (age_key.txt) extremely secure!
#
# To edit the encrypted file later:
#    export SOPS_AGE_KEY_FILE=/path/to/age_key.txt
#    sops prod.vault.yaml

`
	return append([]byte(header), yamlData...)
}
