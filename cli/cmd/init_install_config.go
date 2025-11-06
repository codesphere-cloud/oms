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
	Generator  installer.InstallConfigManager
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

// TODO: Implement this function that should be the only function in RunE
// func (c *InitInstallConfigCmd) CreateConfig(icm files.InstallConfigManager) error {
// 	if c.Opts.Interactive {
// 		_, err := icm.CollectConfiguration(c.cmd)
// 		if err != nil {
// 			return fmt.Errorf("failed to collect configuration: %w", err)
// 		}

// 		icm.SetConfig(c.buildConfigOptions())
// 	} else {
// 		icm.SetConfig(c.buildConfigOptions())
// 	}

// 	// icm.ApplyProfile(c.Opts.Profile)

// 	// Create secrets

// 	// Write config file

// 	// Write vault file

// 	return nil
// }

func (c *InitInstallConfigCmd) RunE(_ *cobra.Command, args []string) error {
	if c.Opts.ValidateOnly {
		return c.validateConfig()
	}

	if c.Opts.Profile != "" {
		if err := c.applyProfile(); err != nil {
			return fmt.Errorf("failed to apply profile: %w", err)
		}
	}

	fmt.Println("Welcome to OMS!")
	fmt.Println("This wizard will help you create config.yaml and prod.vault.yaml for Codesphere installation.")
	fmt.Println()

	configOpts := c.buildConfigOptions()

	_, err := c.Generator.CollectConfiguration(configOpts)
	if err != nil {
		return fmt.Errorf("failed to collect configuration: %w", err)
	}

	if err := c.Generator.WriteConfigAndVault(c.Opts.ConfigFile, c.Opts.VaultFile, c.Opts.WithComments); err != nil {
		return err
	}

	c.printSuccessMessage()

	return nil
}

func (c *InitInstallConfigCmd) buildConfigOptions() *files.ConfigOptions {
	return &files.ConfigOptions{
		DatacenterID:          c.Opts.DatacenterID,
		DatacenterName:        c.Opts.DatacenterName,
		DatacenterCity:        c.Opts.DatacenterCity,
		DatacenterCountryCode: c.Opts.DatacenterCountryCode,

		RegistryServer:            c.Opts.RegistryServer,
		RegistryReplaceImages:     c.Opts.RegistryReplaceImages,
		RegistryLoadContainerImgs: c.Opts.RegistryLoadContainerImgs,

		PostgresMode:        c.Opts.PostgresMode,
		PostgresPrimaryIP:   c.Opts.PostgresPrimaryIP,
		PostgresPrimaryHost: c.Opts.PostgresPrimaryHost,
		PostgresReplicaIP:   c.Opts.PostgresReplicaIP,
		PostgresReplicaName: c.Opts.PostgresReplicaName,
		PostgresExternal:    c.Opts.PostgresExternal,

		CephSubnet: c.Opts.CephSubnet,
		CephHosts:  c.Opts.CephHosts,

		K8sManaged:      c.Opts.K8sManaged,
		K8sAPIServer:    c.Opts.K8sAPIServer,
		K8sControlPlane: c.Opts.K8sControlPlane,
		K8sWorkers:      c.Opts.K8sWorkers,
		K8sExternalHost: c.Opts.K8sExternalHost,
		K8sPodCIDR:      c.Opts.K8sPodCIDR,
		K8sServiceCIDR:  c.Opts.K8sServiceCIDR,

		ClusterGatewayType:       c.Opts.ClusterGatewayType,
		ClusterGatewayIPs:        c.Opts.ClusterGatewayIPs,
		ClusterPublicGatewayType: c.Opts.ClusterPublicGatewayType,
		ClusterPublicGatewayIPs:  c.Opts.ClusterPublicGatewayIPs,

		MetalLBEnabled: c.Opts.MetalLBEnabled,
		MetalLBPools:   c.Opts.MetalLBPools,

		CodesphereDomain:                  c.Opts.CodesphereDomain,
		CodespherePublicIP:                c.Opts.CodespherePublicIP,
		CodesphereWorkspaceBaseDomain:     c.Opts.CodesphereWorkspaceBaseDomain,
		CodesphereCustomDomainBaseDomain:  c.Opts.CodesphereCustomDomainBaseDomain,
		CodesphereDNSServers:              c.Opts.CodesphereDNSServers,
		CodesphereWorkspaceImageBomRef:    c.Opts.CodesphereWorkspaceImageBomRef,
		CodesphereHostingPlanCPU:          c.Opts.CodesphereHostingPlanCPU,
		CodesphereHostingPlanMemory:       c.Opts.CodesphereHostingPlanMemory,
		CodesphereHostingPlanStorage:      c.Opts.CodesphereHostingPlanStorage,
		CodesphereHostingPlanTempStorage:  c.Opts.CodesphereHostingPlanTempStorage,
		CodesphereWorkspacePlanName:       c.Opts.CodesphereWorkspacePlanName,
		CodesphereWorkspacePlanMaxReplica: c.Opts.CodesphereWorkspacePlanMaxReplica,

		SecretsBaseDir: c.Opts.SecretsBaseDir,
	}
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

func (c *InitInstallConfigCmd) applyProfile() error {
	switch strings.ToLower(c.Opts.Profile) {
	case "dev", "development":
		c.Opts.DatacenterID = 1
		c.Opts.DatacenterName = "dev"
		c.Opts.DatacenterCity = "Karlsruhe"
		c.Opts.DatacenterCountryCode = "DE"
		c.Opts.PostgresMode = "install"
		c.Opts.PostgresPrimaryIP = "127.0.0.1"
		c.Opts.PostgresPrimaryHost = "localhost"
		c.Opts.CephSubnet = "127.0.0.1/32"
		c.Opts.CephHosts = []files.CephHostConfig{{Hostname: "localhost", IPAddress: "127.0.0.1", IsMaster: true}}
		c.Opts.K8sManaged = true
		c.Opts.K8sAPIServer = "127.0.0.1"
		c.Opts.K8sControlPlane = []string{"127.0.0.1"}
		c.Opts.K8sWorkers = []string{"127.0.0.1"}
		c.Opts.ClusterGatewayType = "LoadBalancer"
		c.Opts.ClusterPublicGatewayType = "LoadBalancer"
		c.Opts.CodesphereDomain = "codesphere.local"
		c.Opts.CodesphereWorkspaceBaseDomain = "ws.local"
		c.Opts.CodesphereCustomDomainBaseDomain = "custom.local"
		c.Opts.CodesphereDNSServers = []string{"8.8.8.8", "1.1.1.1"}
		c.Opts.CodesphereWorkspaceImageBomRef = "workspace-agent-24.04"
		c.Opts.CodesphereHostingPlanCPU = 10
		c.Opts.CodesphereHostingPlanMemory = 2048
		c.Opts.CodesphereHostingPlanStorage = 20480
		c.Opts.CodesphereHostingPlanTempStorage = 1024
		c.Opts.CodesphereWorkspacePlanName = "Standard Developer"
		c.Opts.CodesphereWorkspacePlanMaxReplica = 3
		c.Opts.Interactive = false
		c.Opts.GenerateKeys = true
		c.Opts.SecretsBaseDir = "/root/secrets"
		fmt.Println("Applied 'dev' profile: single-node development setup")

	case "prod", "production":
		c.Opts.DatacenterID = 1
		c.Opts.DatacenterName = "production"
		c.Opts.DatacenterCity = "Karlsruhe"
		c.Opts.DatacenterCountryCode = "DE"
		c.Opts.PostgresMode = "install"
		c.Opts.PostgresPrimaryIP = "10.50.0.2"
		c.Opts.PostgresPrimaryHost = "pg-primary"
		c.Opts.PostgresReplicaIP = "10.50.0.3"
		c.Opts.PostgresReplicaName = "replica1"
		c.Opts.CephSubnet = "10.53.101.0/24"
		c.Opts.CephHosts = []files.CephHostConfig{
			{Hostname: "ceph-node-0", IPAddress: "10.53.101.2", IsMaster: true},
			{Hostname: "ceph-node-1", IPAddress: "10.53.101.3", IsMaster: false},
			{Hostname: "ceph-node-2", IPAddress: "10.53.101.4", IsMaster: false},
		}
		c.Opts.K8sManaged = true
		c.Opts.K8sAPIServer = "10.50.0.2"
		c.Opts.K8sControlPlane = []string{"10.50.0.2"}
		c.Opts.K8sWorkers = []string{"10.50.0.2", "10.50.0.3", "10.50.0.4"}
		c.Opts.ClusterGatewayType = "LoadBalancer"
		c.Opts.ClusterPublicGatewayType = "LoadBalancer"
		c.Opts.CodesphereDomain = "codesphere.yourcompany.com"
		c.Opts.CodesphereWorkspaceBaseDomain = "ws.yourcompany.com"
		c.Opts.CodesphereCustomDomainBaseDomain = "custom.yourcompany.com"
		c.Opts.CodesphereDNSServers = []string{"1.1.1.1", "8.8.8.8"}
		c.Opts.CodesphereWorkspaceImageBomRef = "workspace-agent-24.04"
		c.Opts.CodesphereHostingPlanCPU = 10
		c.Opts.CodesphereHostingPlanMemory = 2048
		c.Opts.CodesphereHostingPlanStorage = 20480
		c.Opts.CodesphereHostingPlanTempStorage = 1024
		c.Opts.CodesphereWorkspacePlanName = "Standard Developer"
		c.Opts.CodesphereWorkspacePlanMaxReplica = 3
		c.Opts.GenerateKeys = true
		c.Opts.SecretsBaseDir = "/root/secrets"
		fmt.Println("Applied 'production' profile: HA multi-node setup")

	case "minimal":
		c.Opts.DatacenterID = 1
		c.Opts.DatacenterName = "minimal"
		c.Opts.DatacenterCity = "Karlsruhe"
		c.Opts.DatacenterCountryCode = "DE"
		c.Opts.PostgresMode = "install"
		c.Opts.PostgresPrimaryIP = "127.0.0.1"
		c.Opts.PostgresPrimaryHost = "localhost"
		c.Opts.CephSubnet = "127.0.0.1/32"
		c.Opts.CephHosts = []files.CephHostConfig{{Hostname: "localhost", IPAddress: "127.0.0.1", IsMaster: true}}
		c.Opts.K8sManaged = true
		c.Opts.K8sAPIServer = "127.0.0.1"
		c.Opts.K8sControlPlane = []string{"127.0.0.1"}
		c.Opts.K8sWorkers = []string{}
		c.Opts.ClusterGatewayType = "LoadBalancer"
		c.Opts.ClusterPublicGatewayType = "LoadBalancer"
		c.Opts.CodesphereDomain = "codesphere.local"
		c.Opts.CodesphereWorkspaceBaseDomain = "ws.local"
		c.Opts.CodesphereCustomDomainBaseDomain = "custom.local"
		c.Opts.CodesphereDNSServers = []string{"8.8.8.8"}
		c.Opts.CodesphereWorkspaceImageBomRef = "workspace-agent-24.04"
		c.Opts.CodesphereHostingPlanCPU = 10
		c.Opts.CodesphereHostingPlanMemory = 2048
		c.Opts.CodesphereHostingPlanStorage = 20480
		c.Opts.CodesphereHostingPlanTempStorage = 1024
		c.Opts.CodesphereWorkspacePlanName = "Standard Developer"
		c.Opts.CodesphereWorkspacePlanMaxReplica = 1
		c.Opts.Interactive = false
		c.Opts.GenerateKeys = true
		c.Opts.SecretsBaseDir = "/root/secrets"
		fmt.Println("Applied 'minimal' profile: minimal single-node setup")

	default:
		return fmt.Errorf("unknown profile: %s. Available profiles: dev, production, minimal", c.Opts.Profile)
	}

	return nil
}

func (c *InitInstallConfigCmd) validateConfig() error {
	fmt.Printf("Validating configuration files...\n")

	fmt.Printf("Reading config file: %s\n", c.Opts.ConfigFile)
	configFile, err := c.FileWriter.Open(c.Opts.ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}
	defer util.CloseFileIgnoreError(configFile)

	configData, err := io.ReadAll(configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	config, err := installer.UnmarshalConfig(configData)
	if err != nil {
		return fmt.Errorf("failed to parse config.yaml: %w", err)
	}

	errors := installer.ValidateConfig(config)

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

	c.cmd.PreRun = func(cmd *cobra.Command, args []string) {
		c.Generator = installer.NewConfigGenerator(c.Opts.Interactive)
	}

	c.cmd.RunE = c.RunE
	init.AddCommand(c.cmd)
}
