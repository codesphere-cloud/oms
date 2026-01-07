// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
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

	Profile        string
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

	CephNodesSubnet string
	CephHosts       []files.CephHostConfig

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
			
			Supports configuration profiles for common scenarios:
			- dev: Single-node development setup
			- production: HA multi-node setup
			- minimal: Minimal testing setup`),
			Example: formatExamplesWithBinary("init install-config", []csio.Example{
				{Cmd: "-c config.yaml --vault prod.vault.yaml", Desc: "Create config files interactively"},
				{Cmd: "--profile dev -c config.yaml --vault prod.vault.yaml", Desc: "Use dev profile with defaults"},
				{Cmd: "--profile production -c config.yaml --vault prod.vault.yaml", Desc: "Use production profile"},
				{Cmd: "--validate -c config.yaml --vault prod.vault.yaml", Desc: "Validate existing configuration files"},
			}, "oms-cli"),
		},
		Opts:       &InitInstallConfigOpts{GlobalOptions: opts},
		FileWriter: util.NewFilesystemWriter(),
	}

	c.cmd.Flags().StringVarP(&c.Opts.ConfigFile, "config", "c", "config.yaml", "Output file path for config.yaml")
	c.cmd.Flags().StringVar(&c.Opts.VaultFile, "vault", "prod.vault.yaml", "Output file path for prod.vault.yaml")

	c.cmd.Flags().StringVar(&c.Opts.Profile, "profile", "", "Use a predefined configuration profile (dev, production, minimal)")
	c.cmd.Flags().BoolVar(&c.Opts.ValidateOnly, "validate", false, "Validate existing config files instead of creating new ones")
	c.cmd.Flags().BoolVar(&c.Opts.WithComments, "with-comments", false, "Add helpful comments to the generated YAML files")
	c.cmd.Flags().BoolVar(&c.Opts.Interactive, "interactive", true, "Enable interactive prompting (when true, other config flags are ignored)")
	c.cmd.Flags().BoolVar(&c.Opts.GenerateKeys, "generate-keys", true, "Generate SSH keys and certificates")
	c.cmd.Flags().StringVar(&c.Opts.SecretsBaseDir, "secrets-dir", "/root/secrets", "Secrets base directory")

	c.cmd.Flags().IntVar(&c.Opts.DatacenterID, "dc-id", 0, "Datacenter ID")
	c.cmd.Flags().StringVar(&c.Opts.DatacenterName, "dc-name", "", "Datacenter name")

	c.cmd.Flags().StringVar(&c.Opts.PostgresMode, "postgres-mode", "", "PostgreSQL setup mode (install/external)")
	c.cmd.Flags().StringVar(&c.Opts.PostgresPrimaryIP, "postgres-primary-ip", "", "Primary PostgreSQL server IP")

	c.cmd.Flags().BoolVar(&c.Opts.KubernetesManagedByCodesphere, "k8s-managed", true, "Use Codesphere-managed Kubernetes")
	c.cmd.Flags().StringSliceVar(&c.Opts.KubernetesControlPlanes, "k8s-control-plane", []string{}, "K8s control plane IPs (comma-separated)")

	c.cmd.Flags().StringVar(&c.Opts.CodesphereDomain, "domain", "", "Main Codesphere domain")

	util.MarkFlagRequired(c.cmd, "config")
	util.MarkFlagRequired(c.cmd, "vault")

	c.cmd.RunE = c.RunE
	init.AddCommand(c.cmd)
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

	if c.Opts.Interactive {
		err = icg.CollectInteractively()
		if err != nil {
			return fmt.Errorf("failed to collect configuration interactively: %w", err)
		}
	} else {
		c.updateConfigFromOpts(icg.GetInstallConfig())
	}

	errors := icg.ValidateInstallConfig()
	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed: %s", strings.Join(errors, ", "))
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

	c.printSuccessMessage()

	return nil
}

func (c *InitInstallConfigCmd) printWelcomeMessage() {
	fmt.Println("Welcome to OMS!")
	fmt.Println("This wizard will help you create config.yaml and prod.vault.yaml for Codesphere installation.")
	fmt.Println()
}

func (c *InitInstallConfigCmd) printSuccessMessage() {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("Configuration files successfully generated!")
	fmt.Println(strings.Repeat("=", 70))

	fmt.Println("\nIMPORTANT: Keys and certificates have been generated and embedded in the vault file.")
	fmt.Println("   Keep the vault file secure and encrypt it with SOPS before storing.")

	fmt.Println("\nNext steps:")
	fmt.Println("1. Review the generated config.yaml and prod.vault.yaml")
	fmt.Println("2. Install SOPS and Age: brew install sops age")
	fmt.Println("3. Generate an Age keypair: age-keygen -o age_key.txt")
	fmt.Println("4. Encrypt the vault file:")
	fmt.Printf("   age-keygen -y age_key.txt  # Get public key\n")
	fmt.Printf("   sops --encrypt --age <PUBLIC_KEY> --in-place %s\n", c.Opts.VaultFile)
	fmt.Println("5. Run the Codesphere installer with these configuration files")
	fmt.Println()
}

func (c *InitInstallConfigCmd) validateOnly(icg installer.InstallConfigManager) error {
	fmt.Printf("Validating configuration files...\n")

	fmt.Printf("Reading install config file: %s\n", c.Opts.ConfigFile)
	err := icg.LoadInstallConfigFromFile(c.Opts.ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to load config file: %w", err)
	}

	errors := icg.ValidateInstallConfig()
	if len(errors) > 0 {
		return fmt.Errorf("install config validation failed: %s", strings.Join(errors, ", "))
	}

	if c.Opts.VaultFile != "" {
		fmt.Printf("Reading vault file: %s\n", c.Opts.VaultFile)
		err := icg.LoadVaultFromFile(c.Opts.VaultFile)
		if err != nil {
			return fmt.Errorf("failed to load vault file: %w", err)
		}

		vaultErrors := icg.ValidateVault()
		if len(vaultErrors) > 0 {
			return fmt.Errorf("vault validation errors: %s", strings.Join(vaultErrors, ", "))
		}
	}

	fmt.Println("Configuration is valid!")
	return nil
}

func (c *InitInstallConfigCmd) updateConfigFromOpts(config *files.RootConfig) *files.RootConfig {
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
