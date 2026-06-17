// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"strings"

	csio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

type InitInstallConfigCmd struct {
	cmd        *cobra.Command
	Opts       *InitInstallConfigOpts
	FileWriter util.FileIO
}

type InitInstallConfigOpts struct {
	*GlobalOptions

	ConfigFile string
	VaultFile  string

	Profile              string
	AnsibleInventoryFile string

	ValidateOnly   bool
	WithComments   bool
	Interactive    bool
	GenerateKeys   bool
	SecretsBaseDir string

	DatacenterID          int
	DatacenterName        string
	DatacenterCity        string
	DatacenterCountryCode string

	RegistryServer              string
	RegistryReplaceImagesInBom  bool
	RegistryLoadContainerImages bool

	PostgresMode            string
	PostgresPrimaryIP       string
	PostgresPrimaryHostname string
	PostgresReplicaIP       string
	PostgresReplicaName     string
	PostgresServerAddress   string

	CephCsiKubeletDir      string
	CephNodesSubnet        string
	CephHosts              []files.CephHostConfig
	CephOSDDataDevicesSize string
	CephOSDDBDevicesSize   string

	KubernetesManagedByCodesphere bool
	KubernetesAPIServerHost       string
	KubernetesControlPlanes       []string
	KubernetesWorkers             []string
	KubernetesPodCIDR             string
	KubernetesServiceCIDR         string

	ClusterGatewayServiceType       string
	ClusterGatewayIPAddresses       []string
	ClusterPublicGatewayServiceType string
	ClusterPublicGatewayIPAddresses []string

	MetalLBEnabled bool
	MetalLBPools   []files.MetalLBPool

	ACMEEnabled       bool
	ACMEIssuerName    string
	ACMEEmail         string
	ACMEServer        string
	ACMEEABKeyID      string
	ACMEEABMacKey     string
	ACMEDNS01Provider string

	CodesphereDomain                       string
	CodespherePublicIP                     string
	CodesphereWorkspaceHostingBaseDomain   string
	CodesphereCustomDomainsCNameBaseDomain string
	CodesphereDNSServers                   []string
	CodesphereWorkspaceImageBomRef         string
	CodesphereHostingPlanCPUTenth          int
	CodesphereHostingPlanMemoryMb          int
	CodesphereHostingPlanStorageMb         int
	CodesphereHostingPlanTempStorageMb     int
	CodesphereWorkspacePlanName            string
	CodesphereWorkspacePlanMaxReplicas     int

	CodesphereOpenBaoUri      string
	CodesphereOpenBaoEngine   string
	CodesphereOpenBaoUser     string
	CodesphereOpenBaoPassword string
}

func (c *InitInstallConfigCmd) RunE(_ *cobra.Command, args []string) error {
	icg := installer.NewInstallConfigManager()

	return c.InitInstallConfig(icg)
}

func AddInitInstallConfigCmd(init *cobra.Command, opts *GlobalOptions) {
	c := InitInstallConfigCmd{
		cmd: &cobra.Command{
			Use:   "install-config",
			Short: "Initialize Codesphere installer configuration files",
			Long: csio.Long(`Initialize config.yaml and prod.vault.yaml for the Codesphere installer.
			
			This command generates two files:
			- config.yaml: Main configuration (infrastructure, networking, plans)
			- prod.vault.yaml: Secrets file (keys, certificates, passwords)
			
			Note: When --interactive=true (default), all other configuration flags are ignored 
			and you will be prompted for all settings interactively.
			
			Note: When using ansible-inventory make sure the inventory follows our supported structure.
			Supported YAML format (where 'hosts' is a dictionary of hostname keys):
			- <k8s-cp|k8s-workers|ceph>.hosts.<hostname>.private_ip

			Supports configuration profiles for common scenarios:
			- dev: Single-node development setup
			- production: HA multi-node setup
			- minimal: Minimal testing setup
			`),
			Example: formatExamples("init install-config", []csio.Example{
				{Cmd: "-c config.yaml --vault prod.vault.yaml", Desc: "Create config files interactively"},
				{Cmd: "--profile dev -c config.yaml --vault prod.vault.yaml", Desc: "Use dev profile with defaults"},
				{Cmd: "--profile production -c config.yaml --vault prod.vault.yaml", Desc: "Use production profile"},
				{Cmd: "--profile production -c config.yaml --ansible-inventory inventory.yaml", Desc: "Use ansible inventory for host definitions"},
				{Cmd: "--validate -c config.yaml --vault prod.vault.yaml", Desc: "Validate existing configuration files"},
			}),
		},
		Opts:       &InitInstallConfigOpts{GlobalOptions: opts},
		FileWriter: util.NewFilesystemWriter(),
	}

	c.cmd.Flags().StringVarP(&c.Opts.ConfigFile, "config", "c", "config.yaml", "Output file path for config.yaml")
	c.cmd.Flags().StringVar(&c.Opts.VaultFile, "vault", "prod.vault.yaml", "Output file path for prod.vault.yaml")

	c.cmd.Flags().StringVar(&c.Opts.Profile, "profile", "", "Use a predefined configuration profile (dev, production, minimal)")
	c.cmd.Flags().StringVar(&c.Opts.AnsibleInventoryFile, "ansible-inventory", "", "Path to Ansible inventory file to import host information from")

	c.cmd.Flags().BoolVar(&c.Opts.ValidateOnly, "validate", false, "Validate existing config files instead of creating new ones")
	c.cmd.Flags().BoolVar(&c.Opts.WithComments, "with-comments", false, "Add helpful comments to the generated YAML files")
	c.cmd.Flags().BoolVar(&c.Opts.Interactive, "interactive", true, "Enable interactive prompting (when true, other config flags are ignored)")
	c.cmd.Flags().BoolVar(&c.Opts.GenerateKeys, "generate-keys", true, "Generate SSH keys and certificates")
	c.cmd.Flags().StringVar(&c.Opts.SecretsBaseDir, "secrets-dir", "/root/secrets", "Secrets base directory")

	// Datacenter
	c.cmd.Flags().IntVar(&c.Opts.DatacenterID, "dc-id", 0, "Datacenter ID")
	c.cmd.Flags().StringVar(&c.Opts.DatacenterName, "dc-name", "", "Datacenter name")
	c.cmd.Flags().StringVar(&c.Opts.DatacenterCity, "dc-city", "", "Datacenter city")
	c.cmd.Flags().StringVar(&c.Opts.DatacenterCountryCode, "dc-country-code", "", "Datacenter country code")

	// Registry
	c.cmd.Flags().StringVar(&c.Opts.RegistryServer, "registry-server", "", "Server for container registry")

	// Postgres
	c.cmd.Flags().StringVar(&c.Opts.PostgresMode, "postgres-mode", "", "PostgreSQL setup mode (install/external)")
	c.cmd.Flags().StringVar(&c.Opts.PostgresServerAddress, "postgres-server", "", "PostgreSQL server address. Required when using external mode.")
	c.cmd.Flags().StringVar(&c.Opts.PostgresPrimaryIP, "postgres-primary-ip", "", "Primary PostgreSQL server IP")

	// K8s
	c.cmd.Flags().BoolVar(&c.Opts.KubernetesManagedByCodesphere, "k8s-managed", true, "Use Codesphere-managed Kubernetes")
	c.cmd.Flags().StringSliceVar(&c.Opts.KubernetesControlPlanes, "k8s-control-plane", []string{}, "K8s control plane IPs (comma-separated)")

	// Ceph
	c.cmd.Flags().StringVar(&c.Opts.CephCsiKubeletDir, "ceph-csi-kubelet-dir", "", "Directory of kubelet for ceph csi. Required for some cloud providers")
	c.cmd.Flags().StringVar(&c.Opts.CephNodesSubnet, "ceph-nodes-subnet", "", "CIDR subnet for ceph nodes")

	// ACME
	c.cmd.Flags().BoolVar(&c.Opts.ACMEEnabled, "acme-enabled", false, "Enable ACME certificate issuer")
	c.cmd.Flags().StringVar(&c.Opts.ACMEIssuerName, "acme-issuer-name", "acme-issuer", "Name for the ACME ClusterIssuer")
	c.cmd.Flags().StringVar(&c.Opts.ACMEEmail, "acme-email", "", "Email address for ACME account registration")
	c.cmd.Flags().StringVar(&c.Opts.ACMEServer, "acme-server", "https://acme-v02.api.letsencrypt.org/directory", "ACME server URL")
	c.cmd.Flags().StringVar(&c.Opts.ACMEEABKeyID, "acme-eab-key-id", "", "External Account Binding key ID (required by some ACME providers)")
	c.cmd.Flags().StringVar(&c.Opts.ACMEEABMacKey, "acme-eab-mac-key", "", "External Account Binding MAC key (required by some ACME providers)")
	c.cmd.Flags().StringVar(&c.Opts.ACMEDNS01Provider, "acme-dns01-provider", "", "DNS provider for DNS-01 solver (e.g., cloudflare)")

	c.cmd.Flags().StringVar(&c.Opts.CodesphereDomain, "domain", "", "Main Codesphere domain")

	// OpenBao
	c.cmd.Flags().StringVar(&c.Opts.CodesphereOpenBaoUri, "openbao-uri", "", "URI for OpenBao (e.g., https://openbao.example.com)")
	c.cmd.Flags().StringVar(&c.Opts.CodesphereOpenBaoEngine, "openbao-engine", "cs-secrets-engine", "Engine for OpenBao")
	c.cmd.Flags().StringVar(&c.Opts.CodesphereOpenBaoUser, "openbao-user", "admin", "Username for OpenBao authentication")
	c.cmd.Flags().StringVar(&c.Opts.CodesphereOpenBaoPassword, "openbao-password", "", "Password for OpenBao authentication")

	util.MarkFlagRequired(c.cmd, "config")
	util.MarkFlagRequired(c.cmd, "vault")

	c.cmd.RunE = c.RunE
	AddCmd(init, c.cmd)
}

func (c *InitInstallConfigCmd) InitInstallConfig(icg installer.InstallConfigManager) error {
	if c.Opts.ValidateOnly {
		return c.validateOnly(icg)
	}

	// Generate new configuration from either Opts or interactively
	err := icg.ApplyProfile(c.Opts.Profile)
	if err != nil {
		return fmt.Errorf("failed to apply profile: %w", err)
	}

	c.printWelcomeMessage()

	// If Ansible inventory file is provided, import host information from it
	if c.Opts.AnsibleInventoryFile != "" {
		err = icg.FetchFromAnsibleInventory(c.Opts.AnsibleInventoryFile)
		if err != nil {
			return fmt.Errorf("failed to import from Ansible inventory: %w", err)
		}
	}

	if c.Opts.Interactive {
		err = icg.CollectInteractively()
		if err != nil {
			return fmt.Errorf("failed to collect configuration interactively: %w", err)
		}
	} else {
		c.updateConfigFromOpts(icg.GetInstallConfig(), icg.GetVault())
	}

	validationWarnings := icg.ValidateInstallConfig()
	if len(validationWarnings) > 0 {
		if !c.Opts.Interactive {
			return fmt.Errorf("configuration validation failed: %s", strings.Join(validationWarnings, ", "))
		}
		c.printWarningsMessage(validationWarnings)
	}

	if err := icg.GenerateSecrets(); err != nil {
		return fmt.Errorf("failed to generate secrets: %w", err)
	}

	if err := icg.WriteInstallConfig(c.Opts.ConfigFile, c.Opts.WithComments); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if err := icg.WriteVault(c.Opts.VaultFile, c.Opts.WithComments); err != nil {
		return fmt.Errorf("failed to write vault file: %w", err)
	}

	c.printSuccessMessage(len(validationWarnings))

	return nil
}

func (c *InitInstallConfigCmd) printWelcomeMessage() {
	log.Println("Welcome to OMS!")
	log.Println("This wizard will help you create config.yaml and prod.vault.yaml for Codesphere installation.")
	log.Println()
}

func (c *InitInstallConfigCmd) printWarningsMessage(warnings []string) {
	log.Println("\n" + strings.Repeat("!", 70))
	log.Printf("Configuration has %d warning(s):\n", len(warnings))
	for _, w := range warnings {
		log.Printf("  WARNING: %s\n", w)
	}
	log.Println(strings.Repeat("!", 70))
	log.Println("The configuration files will be generated.")
	log.Println("Please review and fix the issues in the generated files before use!")
}

func (c *InitInstallConfigCmd) printSuccessMessage(warningCount int) {
	log.Println("\n" + strings.Repeat("=", 70))
	if warningCount > 0 {
		log.Printf("Configuration files generated with %d warning(s)! Review before use!\n", warningCount)
	} else {
		log.Println("Configuration files successfully generated!")
	}
	log.Println(strings.Repeat("=", 70))

	log.Println("\nIMPORTANT: Keys and certificates have been generated and embedded in the vault file.")
	log.Println("   Keep the vault file secure and encrypt it with SOPS before storing.")

	log.Println("\nNext steps:")
	log.Println("1. Review the generated config.yaml and prod.vault.yaml")
	log.Println("2. Install SOPS and Age: brew install sops age")
	log.Println("3. Generate an Age keypair: age-keygen -o age_key.txt")
	log.Println("4. Encrypt the vault file:")
	log.Printf("   age-keygen -y age_key.txt  # Get public key\n")
	log.Printf("   sops --encrypt --age <PUBLIC_KEY> --in-place %s\n", c.Opts.VaultFile)
	log.Println("5. Run the Codesphere installer with these configuration files")
	log.Println()
}

func (c *InitInstallConfigCmd) validateOnly(icg installer.InstallConfigManager) error {
	log.Printf("Validating configuration files...\n")

	log.Printf("Reading install config file: %s\n", c.Opts.ConfigFile)
	err := icg.LoadInstallConfigFromFile(c.Opts.ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to load config file: %w", err)
	}

	errors := icg.ValidateInstallConfig()
	if len(errors) > 0 {
		return fmt.Errorf("install config validation failed: %s", strings.Join(errors, ", "))
	}

	if c.Opts.VaultFile != "" {
		log.Printf("Reading vault file: %s\n", c.Opts.VaultFile)
		err := icg.LoadVaultFromFile(c.Opts.VaultFile)
		if err != nil {
			return fmt.Errorf("failed to load vault file: %w", err)
		}

		vaultErrors := icg.ValidateVault()
		if len(vaultErrors) > 0 {
			return fmt.Errorf("vault validation errors: %s", strings.Join(vaultErrors, ", "))
		}
	}

	log.Println("Configuration is valid!")
	return nil
}

func (c *InitInstallConfigCmd) updateConfigFromOpts(config *files.RootConfig, vault *files.InstallVault) *files.RootConfig {
	// Datacenter settings
	if c.Opts.DatacenterID != 0 {
		config.Datacenter.ID = c.Opts.DatacenterID
	}
	if c.Opts.DatacenterCity != "" {
		config.Datacenter.City = c.Opts.DatacenterCity
	}
	if c.Opts.DatacenterCountryCode != "" {
		config.Datacenter.CountryCode = c.Opts.DatacenterCountryCode
	}
	if c.Opts.DatacenterName != "" {
		config.Datacenter.Name = c.Opts.DatacenterName
	}

	// Registry settings
	if c.Opts.RegistryServer != "" {
		config.Registry.LoadContainerImages = c.Opts.RegistryLoadContainerImages
		config.Registry.ReplaceImagesInBom = c.Opts.RegistryReplaceImagesInBom
		config.Registry.Server = c.Opts.RegistryServer
	}

	// Postgres settings
	if c.Opts.PostgresMode != "" {
		config.Postgres.Mode = c.Opts.PostgresMode
	}

	if c.Opts.PostgresServerAddress != "" {
		config.Postgres.ServerAddress = c.Opts.PostgresServerAddress
	}

	if c.Opts.PostgresPrimaryHostname != "" && c.Opts.PostgresPrimaryIP != "" {
		if config.Postgres.Primary == nil {
			config.Postgres.Primary = &files.PostgresPrimaryConfig{
				Hostname: c.Opts.PostgresPrimaryHostname,
				IP:       c.Opts.PostgresPrimaryIP,
			}
		} else {
			config.Postgres.Primary.Hostname = c.Opts.PostgresPrimaryHostname
			config.Postgres.Primary.IP = c.Opts.PostgresPrimaryIP
		}
	}

	if c.Opts.PostgresReplicaIP != "" && c.Opts.PostgresReplicaName != "" {
		if config.Postgres.Replica == nil {
			config.Postgres.Replica = &files.PostgresReplicaConfig{
				Name: c.Opts.PostgresReplicaName,
				IP:   c.Opts.PostgresReplicaIP,
			}
		} else {
			config.Postgres.Replica.Name = c.Opts.PostgresReplicaName
			config.Postgres.Replica.IP = c.Opts.PostgresReplicaIP
		}
	}

	// Ceph settings
	if c.Opts.CephCsiKubeletDir != "" {
		config.Ceph.CsiKubeletDir = c.Opts.CephCsiKubeletDir
	}
	if c.Opts.CephNodesSubnet != "" {
		config.Ceph.NodesSubnet = c.Opts.CephNodesSubnet
	}
	if len(c.Opts.CephHosts) > 0 {
		cephHosts := []files.CephHost{}
		for _, hostCfg := range c.Opts.CephHosts {
			cephHosts = append(cephHosts, files.CephHost(hostCfg))
		}
		config.Ceph.Hosts = cephHosts
	}

	// Kubernetes settings
	if c.Opts.KubernetesAPIServerHost != "" {
		config.Kubernetes.APIServerHost = c.Opts.KubernetesAPIServerHost
	}
	if c.Opts.KubernetesPodCIDR != "" {
		config.Kubernetes.PodCIDR = c.Opts.KubernetesPodCIDR
	}
	if c.Opts.KubernetesServiceCIDR != "" {
		config.Kubernetes.ServiceCIDR = c.Opts.KubernetesServiceCIDR
	}

	if len(c.Opts.KubernetesControlPlanes) > 0 {
		kubernetesControlPlanes := []files.K8sNode{}
		for _, ip := range c.Opts.KubernetesControlPlanes {
			kubernetesControlPlanes = append(kubernetesControlPlanes, files.K8sNode{
				IPAddress: ip,
			})
		}
		config.Kubernetes.ControlPlanes = kubernetesControlPlanes
	}

	if len(c.Opts.KubernetesWorkers) > 0 {
		kubernetesWorkers := []files.K8sNode{}
		for _, ip := range c.Opts.KubernetesWorkers {
			kubernetesWorkers = append(kubernetesWorkers, files.K8sNode{
				IPAddress: ip,
			})
		}
		config.Kubernetes.Workers = kubernetesWorkers
	}

	// Cluster Gateway settings
	if c.Opts.ClusterGatewayServiceType != "" {
		config.Cluster.Gateway.ServiceType = c.Opts.ClusterGatewayServiceType
	}
	if len(c.Opts.ClusterGatewayIPAddresses) > 0 {
		config.Cluster.Gateway.IPAddresses = c.Opts.ClusterGatewayIPAddresses
	}
	if c.Opts.ClusterPublicGatewayServiceType != "" {
		config.Cluster.PublicGateway.ServiceType = c.Opts.ClusterPublicGatewayServiceType
	}
	if len(c.Opts.ClusterPublicGatewayIPAddresses) > 0 {
		config.Cluster.PublicGateway.IPAddresses = c.Opts.ClusterPublicGatewayIPAddresses
	}

	// MetalLB settings
	if c.Opts.MetalLBEnabled {
		if config.MetalLB == nil {
			config.MetalLB = &files.MetalLBConfig{
				Enabled: c.Opts.MetalLBEnabled,
				Pools:   []files.MetalLBPoolDef{},
			}
		} else {
			config.MetalLB.Enabled = c.Opts.MetalLBEnabled
			config.MetalLB.Pools = []files.MetalLBPoolDef{}
		}

		for _, pool := range c.Opts.MetalLBPools {
			config.MetalLB.Pools = append(config.MetalLB.Pools, files.MetalLBPoolDef(pool))
		}
	}

	// ACME configuration
	if c.Opts.ACMEEnabled {
		if config.Codesphere.CertIssuer.Acme == nil {
			config.Codesphere.CertIssuer.Acme = &files.ACMEConfig{}
		}
		config.Codesphere.CertIssuer.Type = files.CertIssuerTypeACME
		config.Codesphere.CertIssuer.Acme.Enabled = true

		if c.Opts.ACMEIssuerName != "" {
			config.Codesphere.CertIssuer.Acme.Name = c.Opts.ACMEIssuerName
		}
		if c.Opts.ACMEEmail != "" {
			config.Codesphere.CertIssuer.Acme.Email = c.Opts.ACMEEmail
		}
		if c.Opts.ACMEServer != "" {
			config.Codesphere.CertIssuer.Acme.Server = c.Opts.ACMEServer
		}

		if c.Opts.ACMEEABKeyID != "" {
			config.Codesphere.CertIssuer.Acme.EABKeyID = c.Opts.ACMEEABKeyID
		}
		if c.Opts.ACMEEABMacKey != "" {
			vault.SetSecret(files.SecretEntry{Name: files.SecretAcmeEabMacKey, Fields: &files.SecretFields{Password: c.Opts.ACMEEABMacKey}})
		}

		// Configure DNS-01 solver
		if c.Opts.ACMEDNS01Provider != "" {
			config.Codesphere.CertIssuer.Acme.Solver.DNS01 = &files.ACMEDNS01Solver{
				Provider: c.Opts.ACMEDNS01Provider,
			}
		}
	}

	// Codesphere settings
	if c.Opts.CodesphereDomain != "" {
		config.Codesphere.Domain = c.Opts.CodesphereDomain
	}
	if c.Opts.CodespherePublicIP != "" {
		config.Codesphere.PublicIP = c.Opts.CodespherePublicIP
	}
	if c.Opts.CodesphereWorkspaceHostingBaseDomain != "" {
		config.Codesphere.WorkspaceHostingBaseDomain = c.Opts.CodesphereWorkspaceHostingBaseDomain
	}
	if c.Opts.CodesphereCustomDomainsCNameBaseDomain != "" {
		config.Codesphere.CustomDomains = files.CustomDomainsConfig{CNameBaseDomain: c.Opts.CodesphereCustomDomainsCNameBaseDomain}
	}
	if len(c.Opts.CodesphereDNSServers) > 0 {
		config.Codesphere.DNSServers = c.Opts.CodesphereDNSServers
	}

	if c.Opts.CodesphereWorkspaceImageBomRef != "" {
		if config.Codesphere.WorkspaceImages == nil {
			config.Codesphere.WorkspaceImages = &files.WorkspaceImagesConfig{}
		}
		config.Codesphere.WorkspaceImages.Agent = &files.ImageRef{
			BomRef: c.Opts.CodesphereWorkspaceImageBomRef,
		}
	}

	if c.Opts.CodesphereOpenBaoUri != "" {
		if config.Codesphere.OpenBao == nil {
			config.Codesphere.OpenBao = &files.OpenBaoConfig{}
		}
		config.Codesphere.OpenBao.URI = c.Opts.CodesphereOpenBaoUri
		config.Codesphere.OpenBao.Engine = c.Opts.CodesphereOpenBaoEngine
		config.Codesphere.OpenBao.User = c.Opts.CodesphereOpenBaoUser
		if c.Opts.CodesphereOpenBaoPassword != "" {
			vault.SetSecret(files.SecretEntry{Name: files.SecretOpenBaoPassword, Fields: &files.SecretFields{Password: c.Opts.CodesphereOpenBaoPassword}})
		}
	}

	// Plans
	if c.Opts.CodesphereHostingPlanCPUTenth != 0 || c.Opts.CodesphereHostingPlanMemoryMb != 0 ||
		c.Opts.CodesphereHostingPlanStorageMb != 0 || c.Opts.CodesphereHostingPlanTempStorageMb != 0 {
		config.Codesphere.Plans = files.PlansConfig{
			HostingPlans: map[int]files.HostingPlan{
				1: {
					CPUTenth:      c.Opts.CodesphereHostingPlanCPUTenth,
					MemoryMb:      c.Opts.CodesphereHostingPlanMemoryMb,
					StorageMb:     c.Opts.CodesphereHostingPlanStorageMb,
					TempStorageMb: c.Opts.CodesphereHostingPlanTempStorageMb,
				},
			},
			WorkspacePlans: map[int]files.WorkspacePlan{
				1: {
					Name:          c.Opts.CodesphereWorkspacePlanName,
					HostingPlanID: 1,
					MaxReplicas:   c.Opts.CodesphereWorkspacePlanMaxReplicas,
					OnDemand:      true,
				},
			},
		}
	}

	// Secrets base dir
	if c.Opts.SecretsBaseDir != "" && c.Opts.SecretsBaseDir != "/root/secrets" {
		config.Secrets.BaseDir = c.Opts.SecretsBaseDir
	}

	return config
}
