// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"net"
	"strings"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
	NonInteractive bool
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
	CephHosts  []CephHostConfig

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
	MetalLBPools   []MetalLBPool

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

type CephHostConfig struct {
	Hostname  string
	IPAddress string
	IsMaster  bool
}

type MetalLBPool struct {
	Name        string
	IPAddresses []string
}

type Gen0Config struct {
	DataCenter             DataCenterConfig              `yaml:"dataCenter"`
	Secrets                SecretsConfig                 `yaml:"secrets"`
	Registry               *RegistryConfig               `yaml:"registry,omitempty"`
	Postgres               PostgresConfig                `yaml:"postgres"`
	Ceph                   CephConfig                    `yaml:"ceph"`
	Kubernetes             KubernetesConfig              `yaml:"kubernetes"`
	Cluster                ClusterConfig                 `yaml:"cluster"`
	MetalLB                *MetalLBConfig                `yaml:"metallb,omitempty"`
	Codesphere             CodesphereConfig              `yaml:"codesphere"`
	ManagedServiceBackends *ManagedServiceBackendsConfig `yaml:"managedServiceBackends,omitempty"`
}

type DataCenterConfig struct {
	ID          int    `yaml:"id"`
	Name        string `yaml:"name"`
	City        string `yaml:"city"`
	CountryCode string `yaml:"countryCode"`
}

type SecretsConfig struct {
	BaseDir string `yaml:"baseDir"`
}

type RegistryConfig struct {
	Server              string `yaml:"server"`
	ReplaceImagesInBom  bool   `yaml:"replaceImagesInBom"`
	LoadContainerImages bool   `yaml:"loadContainerImages"`
}

type PostgresConfig struct {
	CACertPem     string                 `yaml:"caCertPem,omitempty"`
	Primary       *PostgresPrimaryConfig `yaml:"primary,omitempty"`
	Replica       *PostgresReplicaConfig `yaml:"replica,omitempty"`
	ServerAddress string                 `yaml:"serverAddress,omitempty"`
}

type PostgresPrimaryConfig struct {
	SSLConfig SSLConfig `yaml:"sslConfig"`
	IP        string    `yaml:"ip"`
	Hostname  string    `yaml:"hostname"`
}

type PostgresReplicaConfig struct {
	IP        string    `yaml:"ip"`
	Name      string    `yaml:"name"`
	SSLConfig SSLConfig `yaml:"sslConfig"`
}

type SSLConfig struct {
	ServerCertPem string `yaml:"serverCertPem"`
}

type CephConfig struct {
	CsiKubeletDir string     `yaml:"csiKubeletDir,omitempty"`
	CephAdmSSHKey CephSSHKey `yaml:"cephAdmSshKey"`
	NodesSubnet   string     `yaml:"nodesSubnet"`
	Hosts         []CephHost `yaml:"hosts"`
	OSDs          []CephOSD  `yaml:"osds"`
}

type CephSSHKey struct {
	PublicKey string `yaml:"publicKey"`
}

type CephHost struct {
	Hostname  string `yaml:"hostname"`
	IPAddress string `yaml:"ipAddress"`
	IsMaster  bool   `yaml:"isMaster"`
}

type CephOSD struct {
	SpecID      string          `yaml:"specId"`
	Placement   CephPlacement   `yaml:"placement"`
	DataDevices CephDataDevices `yaml:"dataDevices"`
	DBDevices   CephDBDevices   `yaml:"dbDevices"`
}

type CephPlacement struct {
	HostPattern string `yaml:"host_pattern"`
}

type CephDataDevices struct {
	Size  string `yaml:"size"`
	Limit int    `yaml:"limit"`
}

type CephDBDevices struct {
	Size  string `yaml:"size"`
	Limit int    `yaml:"limit"`
}

type KubernetesConfig struct {
	ManagedByCodesphere bool      `yaml:"managedByCodesphere"`
	APIServerHost       string    `yaml:"apiServerHost,omitempty"`
	ControlPlanes       []K8sNode `yaml:"controlPlanes,omitempty"`
	Workers             []K8sNode `yaml:"workers,omitempty"`
	PodCIDR             string    `yaml:"podCidr,omitempty"`
	ServiceCIDR         string    `yaml:"serviceCidr,omitempty"`
}

type K8sNode struct {
	IPAddress string `yaml:"ipAddress"`
}

type ClusterConfig struct {
	Certificates  ClusterCertificates `yaml:"certificates"`
	Monitoring    *MonitoringConfig   `yaml:"monitoring,omitempty"`
	Gateway       GatewayConfig       `yaml:"gateway"`
	PublicGateway GatewayConfig       `yaml:"publicGateway"`
}

type ClusterCertificates struct {
	CA CAConfig `yaml:"ca"`
}

type CAConfig struct {
	Algorithm   string `yaml:"algorithm"`
	KeySizeBits int    `yaml:"keySizeBits"`
	CertPem     string `yaml:"certPem"`
}

type GatewayConfig struct {
	ServiceType string            `yaml:"serviceType"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
	IPAddresses []string          `yaml:"ipAddresses,omitempty"`
}

type MetalLBConfig struct {
	Enabled bool             `yaml:"enabled"`
	Pools   []MetalLBPoolDef `yaml:"pools"`
	L2      []MetalLBL2      `yaml:"l2,omitempty"`
	BGP     []MetalLBBGP     `yaml:"bgp,omitempty"`
}

type MetalLBPoolDef struct {
	Name        string   `yaml:"name"`
	IPAddresses []string `yaml:"ipAddresses"`
}

type MetalLBL2 struct {
	Name          string              `yaml:"name"`
	Pools         []string            `yaml:"pools"`
	NodeSelectors []map[string]string `yaml:"nodeSelectors,omitempty"`
}

type MetalLBBGP struct {
	Name          string              `yaml:"name"`
	Pools         []string            `yaml:"pools"`
	Config        MetalLBBGPConfig    `yaml:"config"`
	NodeSelectors []map[string]string `yaml:"nodeSelectors,omitempty"`
}

type MetalLBBGPConfig struct {
	MyASN       int    `yaml:"myASN"`
	PeerASN     int    `yaml:"peerASN"`
	PeerAddress string `yaml:"peerAddress"`
	BFDProfile  string `yaml:"bfdProfile,omitempty"`
}

type CodesphereConfig struct {
	Domain                     string                 `yaml:"domain"`
	WorkspaceHostingBaseDomain string                 `yaml:"workspaceHostingBaseDomain"`
	PublicIP                   string                 `yaml:"publicIp"`
	CustomDomains              CustomDomainsConfig    `yaml:"customDomains"`
	DNSServers                 []string               `yaml:"dnsServers"`
	Experiments                []string               `yaml:"experiments"`
	ExtraCAPem                 string                 `yaml:"extraCaPem,omitempty"`
	ExtraWorkspaceEnvVars      map[string]string      `yaml:"extraWorkspaceEnvVars,omitempty"`
	ExtraWorkspaceFiles        []ExtraWorkspaceFile   `yaml:"extraWorkspaceFiles,omitempty"`
	WorkspaceImages            *WorkspaceImagesConfig `yaml:"workspaceImages,omitempty"`
	DeployConfig               DeployConfig           `yaml:"deployConfig"`
	Plans                      PlansConfig            `yaml:"plans"`
	UnderprovisionFactors      *UnderprovisionFactors `yaml:"underprovisionFactors,omitempty"`
	GitProviders               *GitProvidersConfig    `yaml:"gitProviders,omitempty"`
	ManagedServices            []ManagedServiceConfig `yaml:"managedServices,omitempty"`
}

type CustomDomainsConfig struct {
	CNameBaseDomain string `yaml:"cNameBaseDomain"`
}

type ExtraWorkspaceFile struct {
	Path    string `yaml:"path"`
	Content string `yaml:"content"`
}

type WorkspaceImagesConfig struct {
	Agent    *ImageRef `yaml:"agent,omitempty"`
	AgentGpu *ImageRef `yaml:"agentGpu,omitempty"`
	Server   *ImageRef `yaml:"server,omitempty"`
	VPN      *ImageRef `yaml:"vpn,omitempty"`
}

type ImageRef struct {
	BomRef string `yaml:"bomRef"`
}

type DeployConfig struct {
	Images map[string]DeployImage `yaml:"images"`
}

type DeployImage struct {
	Name           string                  `yaml:"name"`
	SupportedUntil string                  `yaml:"supportedUntil"`
	Flavors        map[string]DeployFlavor `yaml:"flavors"`
}

type DeployFlavor struct {
	Image ImageRef    `yaml:"image"`
	Pool  map[int]int `yaml:"pool"`
}

type PlansConfig struct {
	HostingPlans   map[int]HostingPlan   `yaml:"hostingPlans"`
	WorkspacePlans map[int]WorkspacePlan `yaml:"workspacePlans"`
}

type HostingPlan struct {
	CPUTenth      int `yaml:"cpuTenth"`
	GPUParts      int `yaml:"gpuParts"`
	MemoryMb      int `yaml:"memoryMb"`
	StorageMb     int `yaml:"storageMb"`
	TempStorageMb int `yaml:"tempStorageMb"`
}

type WorkspacePlan struct {
	Name          string `yaml:"name"`
	HostingPlanID int    `yaml:"hostingPlanId"`
	MaxReplicas   int    `yaml:"maxReplicas"`
	OnDemand      bool   `yaml:"onDemand"`
}

type UnderprovisionFactors struct {
	CPU    float64 `yaml:"cpu"`
	Memory float64 `yaml:"memory"`
}

type GitProvidersConfig struct {
	GitHub      *GitProviderConfig `yaml:"github,omitempty"`
	GitLab      *GitProviderConfig `yaml:"gitlab,omitempty"`
	Bitbucket   *GitProviderConfig `yaml:"bitbucket,omitempty"`
	AzureDevOps *GitProviderConfig `yaml:"azureDevOps,omitempty"`
}

type GitProviderConfig struct {
	Enabled bool        `yaml:"enabled"`
	URL     string      `yaml:"url"`
	API     APIConfig   `yaml:"api"`
	OAuth   OAuthConfig `yaml:"oauth"`
}

type APIConfig struct {
	BaseURL string `yaml:"baseUrl"`
}

type OAuthConfig struct {
	Issuer                string `yaml:"issuer"`
	AuthorizationEndpoint string `yaml:"authorizationEndpoint"`
	TokenEndpoint         string `yaml:"tokenEndpoint"`
	ClientAuthMethod      string `yaml:"clientAuthMethod,omitempty"`
	Scope                 string `yaml:"scope,omitempty"`
}

type ManagedServiceConfig struct {
	Name          string                 `yaml:"name"`
	API           ManagedServiceAPI      `yaml:"api"`
	Author        string                 `yaml:"author"`
	Category      string                 `yaml:"category"`
	ConfigSchema  map[string]interface{} `yaml:"configSchema"`
	DetailsSchema map[string]interface{} `yaml:"detailsSchema"`
	SecretsSchema map[string]interface{} `yaml:"secretsSchema"`
	Description   string                 `yaml:"description"`
	DisplayName   string                 `yaml:"displayName"`
	IconURL       string                 `yaml:"iconUrl"`
	Plans         []ServicePlan          `yaml:"plans"`
	Version       string                 `yaml:"version"`
}

type ManagedServiceAPI struct {
	Endpoint string `yaml:"endpoint"`
}

type ServicePlan struct {
	ID          int                  `yaml:"id"`
	Description string               `yaml:"description"`
	Name        string               `yaml:"name"`
	Parameters  map[string]PlanParam `yaml:"parameters"`
}

type PlanParam struct {
	PricedAs string                 `yaml:"pricedAs"`
	Schema   map[string]interface{} `yaml:"schema"`
}

type ManagedServiceBackendsConfig struct {
	Postgres map[string]interface{} `yaml:"postgres,omitempty"`
}

type MonitoringConfig struct {
	Prometheus *PrometheusConfig `yaml:"prometheus,omitempty"`
}

type PrometheusConfig struct {
	RemoteWrite *RemoteWriteConfig `yaml:"remoteWrite,omitempty"`
}

type RemoteWriteConfig struct {
	Enabled     bool   `yaml:"enabled"`
	ClusterName string `yaml:"clusterName,omitempty"`
}

type Gen0Vault struct {
	Secrets []SecretEntry `yaml:"secrets"`
}

type SecretEntry struct {
	Name   string        `yaml:"name"`
	File   *SecretFile   `yaml:"file,omitempty"`
	Fields *SecretFields `yaml:"fields,omitempty"`
}

type SecretFile struct {
	Name    string `yaml:"name"`
	Content string `yaml:"content"`
}

type SecretFields struct {
	Password string `yaml:"password"`
}

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

	if err := c.collectConfiguration(); err != nil {
		return fmt.Errorf("failed to collect configuration: %w", err)
	}

	var generatedSecrets *GeneratedSecrets
	if c.Opts.GenerateKeys {
		fmt.Println("\nGenerating SSH keys and certificates...")
		var err error
		generatedSecrets, err = c.generateSecrets()
		if err != nil {
			return fmt.Errorf("failed to generate secrets: %w", err)
		}
		fmt.Println("Keys and certificates generated successfully")
	}

	config := c.buildGen0Config(generatedSecrets)
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config.yaml: %w", err)
	}

	if c.Opts.WithComments {
		configYAML = c.addConfigComments(configYAML)
	}

	configFile, err := c.FileWriter.Create(c.Opts.ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer util.CloseFileIgnoreError(configFile)

	if _, err = configFile.Write(configYAML); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("\nConfiguration file created: %s\n", c.Opts.ConfigFile)

	vault := c.buildGen0Vault(generatedSecrets)
	vaultYAML, err := yaml.Marshal(vault)
	if err != nil {
		return fmt.Errorf("failed to marshal vault.yaml: %w", err)
	}

	if c.Opts.WithComments {
		vaultYAML = c.addVaultComments(vaultYAML)
	}

	vaultFile, err := c.FileWriter.Create(c.Opts.VaultFile)
	if err != nil {
		return fmt.Errorf("failed to create vault file: %w", err)
	}
	defer util.CloseFileIgnoreError(vaultFile)

	if _, err = vaultFile.Write(vaultYAML); err != nil {
		return fmt.Errorf("failed to write vault file: %w", err)
	}

	fmt.Printf("Secrets file created: %s\n", c.Opts.VaultFile)

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("Configuration files successfully generated!")
	fmt.Println(strings.Repeat("=", 70))

	if c.Opts.GenerateKeys {
		fmt.Println("\nIMPORTANT: Keys and certificates have been generated and embedded in the vault file.")
		fmt.Println("   Keep the vault file secure and encrypt it with SOPS before storing.")
	}

	fmt.Println("\nNext steps:")
	fmt.Println("1. Review the generated config.yaml and prod.vault.yaml")
	fmt.Println("2. Install SOPS and Age: brew install sops age")
	fmt.Println("3. Generate an Age keypair: age-keygen -o age_key.txt")
	fmt.Println("4. Encrypt the vault file:")
	fmt.Printf("   age-keygen -y age_key.txt  # Get public key\n")
	fmt.Printf("   sops --encrypt --age <PUBLIC_KEY> --in-place %s\n", c.Opts.VaultFile)
	fmt.Println("5. Run the Gen0 installer with these configuration files")
	fmt.Println()

	return nil
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
		c.Opts.CephHosts = []CephHostConfig{{Hostname: "localhost", IPAddress: "127.0.0.1", IsMaster: true}}
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
		c.Opts.NonInteractive = true
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
		c.Opts.CephHosts = []CephHostConfig{
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
		c.Opts.CephHosts = []CephHostConfig{{Hostname: "localhost", IPAddress: "127.0.0.1", IsMaster: true}}
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
		c.Opts.NonInteractive = true
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

	var config Gen0Config
	decoder := yaml.NewDecoder(configFile)
	if err := decoder.Decode(&config); err != nil {
		return fmt.Errorf("failed to parse config.yaml: %w", err)
	}

	errors := []string{}

	if config.DataCenter.ID == 0 {
		errors = append(errors, "datacenter ID is required")
	}
	if config.DataCenter.Name == "" {
		errors = append(errors, "datacenter name is required")
	}

	if len(config.Ceph.Hosts) == 0 {
		errors = append(errors, "at least one Ceph host is required")
	}
	for _, host := range config.Ceph.Hosts {
		if !isValidIP(host.IPAddress) {
			errors = append(errors, fmt.Sprintf("invalid Ceph host IP: %s", host.IPAddress))
		}
	}

	if config.Kubernetes.ManagedByCodesphere {
		if len(config.Kubernetes.ControlPlanes) == 0 {
			errors = append(errors, "at least one K8s control plane node is required")
		}
	} else {
		if config.Kubernetes.PodCIDR == "" {
			errors = append(errors, "pod CIDR is required for external Kubernetes")
		}
		if config.Kubernetes.ServiceCIDR == "" {
			errors = append(errors, "service CIDR is required for external Kubernetes")
		}
	}

	if config.Codesphere.Domain == "" {
		errors = append(errors, "Codesphere domain is required")
	}

	if c.Opts.VaultFile != "" {
		fmt.Printf("Reading vault file: %s\n", c.Opts.VaultFile)
		vaultFile, err := c.FileWriter.Open(c.Opts.VaultFile)
		if err != nil {
			fmt.Printf("Warning: Could not open vault file: %v\n", err)
		} else {
			defer util.CloseFileIgnoreError(vaultFile)

			var vault Gen0Vault
			vaultDecoder := yaml.NewDecoder(vaultFile)
			if err := vaultDecoder.Decode(&vault); err != nil {
				errors = append(errors, fmt.Sprintf("failed to parse vault.yaml: %v", err))
			} else {
				requiredSecrets := []string{"cephSshPrivateKey", "selfSignedCaKeyPem", "domainAuthPrivateKey", "domainAuthPublicKey"}
				foundSecrets := make(map[string]bool)
				for _, secret := range vault.Secrets {
					foundSecrets[secret.Name] = true
				}
				for _, required := range requiredSecrets {
					if !foundSecrets[required] {
						errors = append(errors, fmt.Sprintf("required secret missing: %s", required))
					}
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

func isValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

func AddInitInstallConfigCmd(init *cobra.Command, opts *GlobalOptions) {
	c := InitInstallConfigCmd{
		cmd: &cobra.Command{
			Use:   "install-config",
			Short: "Initialize Codesphere Gen0 installer configuration files",
			Long: io.Long(`Initialize config.yaml and prod.vault.yaml for the Codesphere Gen0 installer.
			
			This command generates two files:
			- config.yaml: Main configuration (infrastructure, networking, plans)
			- prod.vault.yaml: Secrets file (keys, certificates, passwords)
			
			Supports configuration profiles for common scenarios:
			- dev: Single-node development setup
			- production: HA multi-node setup
			- minimal: Minimal testing setup`),
			Example: formatExamplesWithBinary("init install-config", []io.Example{
				{Cmd: "-c config.yaml -v prod.vault.yaml", Desc: "Create Gen0 config files interactively"},
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
	c.cmd.Flags().BoolVar(&c.Opts.NonInteractive, "non-interactive", false, "Use default values without prompting")
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
