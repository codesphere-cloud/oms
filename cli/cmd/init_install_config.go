// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"io"
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

	RegistryServer            string
	RegistryReplaceImages     bool
	RegistryLoadContainerImgs bool

	PostgresMode        string
	PostgresPrimaryIP   string
	PostgresPrimaryHost string
	PostgresReplicaIP   string
	PostgresReplicaName string
	PostgresExternal    string

	CephSubnet string
	CephHosts  []files.CephHostConfig

	K8sManaged      bool
	K8sAPIServer    string
	K8sControlPlane []string
	K8sWorkers      []string
	K8sExternalHost string
	K8sPodCIDR      string
	K8sServiceCIDR  string

	ClusterGatewayType       string
	ClusterGatewayIPs        []string
	ClusterPublicGatewayType string
	ClusterPublicGatewayIPs  []string

	MetalLBEnabled bool
	MetalLBPools   []files.MetalLBPool

	CodesphereDomain                  string
	CodespherePublicIP                string
	CodesphereWorkspaceBaseDomain     string
	CodesphereCustomDomainBaseDomain  string
	CodesphereDNSServers              []string
	CodesphereWorkspaceImageBomRef    string
	CodesphereHostingPlanCPU          int
	CodesphereHostingPlanMemory       int
	CodesphereHostingPlanStorage      int
	CodesphereHostingPlanTempStorage  int
	CodesphereWorkspacePlanName       string
	CodesphereWorkspacePlanMaxReplica int
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
				{Cmd: "-c config.yaml -v prod.vault.yaml", Desc: "Create config files interactively"},
				{Cmd: "--profile dev -c config.yaml -v prod.vault.yaml", Desc: "Use dev profile with defaults"},
				{Cmd: "--profile production -c config.yaml -v prod.vault.yaml", Desc: "Use production profile"},
				{Cmd: "--validate -c config.yaml -v prod.vault.yaml", Desc: "Validate existing configuration files"},
			}, "oms-cli"),
		},
		Opts:       &InitInstallConfigOpts{GlobalOptions: opts},
		FileWriter: util.NewFilesystemWriter(),
	}

	c.cmd.Flags().StringVarP(&c.Opts.ConfigFile, "config", "c", "config.yaml", "Output file path for config.yaml")
	c.cmd.Flags().StringVarP(&c.Opts.VaultFile, "vault", "v", "prod.vault.yaml", "Output file path for prod.vault.yaml")

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

	c.cmd.Flags().BoolVar(&c.Opts.K8sManaged, "k8s-managed", true, "Use Codesphere-managed Kubernetes")
	c.cmd.Flags().StringSliceVar(&c.Opts.K8sControlPlane, "k8s-control-plane", []string{}, "K8s control plane IPs (comma-separated)")

	c.cmd.Flags().StringVar(&c.Opts.CodesphereDomain, "domain", "", "Main Codesphere domain")

	util.MarkFlagRequired(c.cmd, "config")
	util.MarkFlagRequired(c.cmd, "vault")

	c.cmd.RunE = c.RunE
	init.AddCommand(c.cmd)
}

func (c *InitInstallConfigCmd) InitInstallConfig(icg installer.InstallConfigManager) error {
	// Validation only mode
	if c.Opts.ValidateOnly {
		// TODO: put into validateOnly method
		err := icg.LoadConfigFromFile(c.Opts.ConfigFile)
		if err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}

		err = icg.Validate()
		if err != nil {
			return fmt.Errorf("configuration validation failed: %w", err)
		}

		// err = icg.LoadVaultFromFile(c.Opts.VaultFile)
		// if err != nil {
		// 	return fmt.Errorf("failed to load vault file: %w", err)
		// }

		// err = icg.ValidateVault()
		// if err != nil {
		// 	return fmt.Errorf("vault validation failed: %w", err)
		// }

		return nil
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
		c.updateConfigFromOpts(icg.GetConfig())
	}

	if err := icg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
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

	// TODO: Check if config file can be empty
	if c.Opts.ConfigFile != "" {
		fmt.Printf("Reading config file: %s\n", c.Opts.ConfigFile)
		err := icg.LoadConfigFromFile(c.Opts.ConfigFile)
		if err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}

		err = icg.Validate()
		if err != nil {
			return fmt.Errorf("configuration validation failed: %w", err)
		}
	}

	var errors []string
	if c.Opts.VaultFile != "" {
		fmt.Printf("Reading vault file: %s\n", c.Opts.VaultFile)
		vaultFile, err := c.FileWriter.Open(c.Opts.VaultFile)
		if err != nil {
			fmt.Printf("Warning: Could not open vault file: %v\n", err)
		} else {
			defer util.CloseFileIgnoreError(vaultFile)

			vaultData, err := io.ReadAll(vaultFile)
			if err != nil {
				errors = append(errors, fmt.Sprintf("failed to read vault.yaml: %v", err))
			} else {
				vault, err := installer.UnmarshalVault(vaultData)
				if err != nil {
					errors = append(errors, fmt.Sprintf("failed to parse vault.yaml: %v", err))
				} else {
					vaultErrors := installer.ValidateVault(vault)
					errors = append(errors, vaultErrors...)
				}
			}
		}
	}

	if len(errors) > 0 {
		fmt.Println("Validation failed:")
		for _, err := range errors {
			fmt.Printf("  - %s\n", err)
		}
		return fmt.Errorf("configuration validation failed with %d error(s)", len(errors))
	}

	fmt.Println("Configuration is valid!")
	return nil
}

func (c *InitInstallConfigCmd) updateConfigFromOpts(config *files.RootConfig) *files.RootConfig {
	// Datacenter settings
	config.Datacenter.ID = c.Opts.DatacenterID
	config.Datacenter.City = c.Opts.DatacenterCity
	config.Datacenter.CountryCode = c.Opts.DatacenterCountryCode
	config.Datacenter.Name = c.Opts.DatacenterName

	// Registry settings
	config.Registry.LoadContainerImages = c.Opts.RegistryLoadContainerImgs
	config.Registry.ReplaceImagesInBom = c.Opts.RegistryReplaceImages
	config.Registry.Server = c.Opts.RegistryServer

	// Postgres settings
	if c.Opts.PostgresExternal != "" {
		config.Postgres.ServerAddress = c.Opts.PostgresExternal
	}

	if c.Opts.PostgresPrimaryHost != "" && c.Opts.PostgresPrimaryIP != "" {
		if config.Postgres.Primary == nil {
			// TODO: Mode:        c.Opts.PostgresMode,
			// TODO: External:    c.Opts.PostgresExternal,
			config.Postgres.Primary = &files.PostgresPrimaryConfig{
				Hostname: c.Opts.PostgresPrimaryHost,
				IP:       c.Opts.PostgresPrimaryIP,
			}
		} else {
			// TODO: Mode:        c.Opts.PostgresMode,
			// TODO: External:    c.Opts.PostgresExternal,
			config.Postgres.Primary.Hostname = c.Opts.PostgresPrimaryHost
			config.Postgres.Primary.IP = c.Opts.PostgresPrimaryIP
		}
	}

	if c.Opts.PostgresReplicaIP != "" && c.Opts.PostgresReplicaName != "" {
		if config.Postgres.Replica == nil {
			// TODO: Mode:        c.Opts.PostgresMode,
			// TODO: External:    c.Opts.PostgresExternal,
			config.Postgres.Replica = &files.PostgresReplicaConfig{
				Name: c.Opts.PostgresReplicaName,
				IP:   c.Opts.PostgresReplicaIP,
			}
		} else {
			// TODO: Mode:        c.Opts.PostgresMode,
			// TODO: External:    c.Opts.PostgresExternal,
			config.Postgres.Replica.Name = c.Opts.PostgresReplicaName
			config.Postgres.Replica.IP = c.Opts.PostgresReplicaIP
		}
	}

	// Ceph settings
	config.Ceph.NodesSubnet = c.Opts.CephSubnet
	cephHosts := []files.CephHost{}
	for _, hostCfg := range c.Opts.CephHosts {
		cephHosts = append(config.Ceph.Hosts, files.CephHost{
			Hostname:  hostCfg.Hostname,
			IPAddress: hostCfg.IPAddress,
			IsMaster:  hostCfg.IsMaster,
		})
	}
	if len(cephHosts) > 0 {
		config.Ceph.Hosts = cephHosts
	}

	// Kubernetes settings
	config.Kubernetes.ManagedByCodesphere = c.Opts.K8sManaged
	config.Kubernetes.APIServerHost = c.Opts.K8sAPIServer
	config.Kubernetes.PodCIDR = c.Opts.K8sPodCIDR
	config.Kubernetes.ServiceCIDR = c.Opts.K8sServiceCIDR

	kubernetesControlPlanes := []files.K8sNode{}
	for _, ip := range c.Opts.K8sControlPlane {
		kubernetesControlPlanes = append(kubernetesControlPlanes, files.K8sNode{
			IPAddress: ip,
		})
	}
	config.Kubernetes.ControlPlanes = kubernetesControlPlanes

	kubernetesWorkers := []files.K8sNode{}
	for _, ip := range c.Opts.K8sWorkers {
		kubernetesWorkers = append(kubernetesWorkers, files.K8sNode{
			IPAddress: ip,
		})
	}
	config.Kubernetes.Workers = kubernetesWorkers

	// Cluster Gateway settings
	config.Cluster.Gateway.ServiceType = c.Opts.ClusterGatewayType
	config.Cluster.Gateway.IPAddresses = c.Opts.ClusterGatewayIPs
	config.Cluster.PublicGateway.ServiceType = c.Opts.ClusterPublicGatewayType
	config.Cluster.PublicGateway.IPAddresses = c.Opts.ClusterPublicGatewayIPs

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
			config.MetalLB.Pools = append(config.MetalLB.Pools, files.MetalLBPoolDef{
				Name:        pool.Name,
				IPAddresses: pool.IPAddresses,
				// TODO: ARPEnabled: pool.ARPEnabled,
			})
		}
	}

	// Codesphere settings
	config.Codesphere.Domain = c.Opts.CodesphereDomain
	config.Codesphere.PublicIP = c.Opts.CodespherePublicIP
	config.Codesphere.WorkspaceHostingBaseDomain = c.Opts.CodesphereWorkspaceBaseDomain
	config.Codesphere.CustomDomains = files.CustomDomainsConfig{CNameBaseDomain: c.Opts.CodesphereCustomDomainBaseDomain}
	config.Codesphere.DNSServers = c.Opts.CodesphereDNSServers

	if config.Codesphere.WorkspaceImages == nil {
		config.Codesphere.WorkspaceImages = &files.WorkspaceImagesConfig{}
	}
	config.Codesphere.WorkspaceImages.Agent = &files.ImageRef{
		BomRef: c.Opts.CodesphereWorkspaceImageBomRef,
	}

	config.Codesphere.Plans = files.PlansConfig{
		HostingPlans: map[int]files.HostingPlan{
			1: {
				CPUTenth:      c.Opts.CodesphereHostingPlanCPU,
				MemoryMb:      c.Opts.CodesphereHostingPlanMemory,
				StorageMb:     c.Opts.CodesphereHostingPlanStorage,
				TempStorageMb: c.Opts.CodesphereHostingPlanTempStorage,
			},
		},
		WorkspacePlans: map[int]files.WorkspacePlan{
			1: {
				Name:          c.Opts.CodesphereWorkspacePlanName,
				HostingPlanID: 1,
				MaxReplicas:   c.Opts.CodesphereWorkspacePlanMaxReplica,
				OnDemand:      true,
			},
		},
	}

	// Secrets base dir
	config.Secrets.BaseDir = c.Opts.SecretsBaseDir

	return config
}
