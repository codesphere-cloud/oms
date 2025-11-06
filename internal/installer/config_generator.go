// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"net"
	"strings"

	"gopkg.in/yaml.v3"
)

type ConfigGenerator struct {
	Interactive bool
	configOpts  *ConfigOptions
	config      *InstallConfig
}

func NewConfigGenerator(interactive bool) *ConfigGenerator {
	return &ConfigGenerator{
		Interactive: interactive,
	}
}

type InstallConfig struct {
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

	// Stored separately in vault
	caCertPrivateKey string            `yaml:"-"`
	adminPassword    string            `yaml:"-"`
	replicaPassword  string            `yaml:"-"`
	userPasswords    map[string]string `yaml:"-"`
}

type PostgresPrimaryConfig struct {
	SSLConfig SSLConfig `yaml:"sslConfig"`
	IP        string    `yaml:"ip"`
	Hostname  string    `yaml:"hostname"`

	privateKey string `yaml:"-"`
}

type PostgresReplicaConfig struct {
	IP        string    `yaml:"ip"`
	Name      string    `yaml:"name"`
	SSLConfig SSLConfig `yaml:"sslConfig"`

	privateKey string `yaml:"-"`
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

	sshPrivateKey string `yaml:"-"`
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

	// Internal flag
	needsKubeConfig bool `yaml:"-"`
}

type K8sNode struct {
	IPAddress string `yaml:"ipAddress"`
}

type ClusterConfig struct {
	Certificates  ClusterCertificates `yaml:"certificates"`
	Monitoring    *MonitoringConfig   `yaml:"monitoring,omitempty"`
	Gateway       GatewayConfig       `yaml:"gateway"`
	PublicGateway GatewayConfig       `yaml:"publicGateway"`

	ingressCAKey string `yaml:"-"`
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

	domainAuthPrivateKey string `yaml:"-"`
	domainAuthPublicKey  string `yaml:"-"`
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

type InstallVault struct {
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

type ConfigOptions struct {
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

	SecretsBaseDir string
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

// CollectConfiguration gathers all configuration needed for Codesphere installation.
// It uses the provided ConfigOptions to pre-fill values, and falls back to
// interactive prompts (if enabled) or defaults when options are missing.
// Returns a complete InstallConfig with generated secrets stored in yaml:"-" fields.
func (g *ConfigGenerator) CollectConfiguration(opts *ConfigOptions) (*InstallConfig, error) {
	// 1. collectConfig, 2. convertConfi, 3. generateSecrets,  4. writeYamlFile, 5 writeVaultFile

	prompter := NewPrompter(g.Interactive)

	fmt.Println("=== Datacenter Configuration ===")
	dcID := opts.DatacenterID
	if dcID == 0 {
		dcID = prompter.Int("Datacenter ID", 1)
	}
	dcName := opts.DatacenterName
	if dcName == "" {
		dcName = prompter.String("Datacenter name", "main")
	}
	dcCity := opts.DatacenterCity
	if dcCity == "" {
		dcCity = prompter.String("Datacenter city", "Karlsruhe")
	}
	dcCountry := opts.DatacenterCountryCode
	if dcCountry == "" {
		dcCountry = prompter.String("Country code", "DE")
	}

	secretsBaseDir := opts.SecretsBaseDir
	if secretsBaseDir == "" {
		secretsBaseDir = prompter.String("Secrets base directory", "/root/secrets")
	}

	fmt.Println("\n=== Container Registry Configuration ===")
	registryServer := opts.RegistryServer
	if registryServer == "" {
		registryServer = prompter.String("Container registry server (e.g., ghcr.io, leave empty to skip)", "")
	}
	var registryConfig *RegistryConfig
	if registryServer != "" {
		registryConfig = &RegistryConfig{
			Server:              registryServer,
			ReplaceImagesInBom:  opts.RegistryReplaceImages,
			LoadContainerImages: opts.RegistryLoadContainerImgs,
		}
		if g.Interactive {
			registryConfig.ReplaceImagesInBom = prompter.Bool("Replace images in BOM", true)
			registryConfig.LoadContainerImages = prompter.Bool("Load container images from installer", false)
		}
	}

	fmt.Println("\n=== PostgreSQL Configuration ===")
	pgMode := opts.PostgresMode
	if pgMode == "" {
		pgMode = prompter.Choice("PostgreSQL setup", []string{"install", "external"}, "install")
	}

	var postgresConfig PostgresConfig
	if pgMode == "install" {
		pgPrimaryIP := opts.PostgresPrimaryIP
		if pgPrimaryIP == "" {
			pgPrimaryIP = prompter.String("Primary PostgreSQL server IP", "10.50.0.2")
		}
		pgPrimaryHost := opts.PostgresPrimaryHost
		if pgPrimaryHost == "" {
			pgPrimaryHost = prompter.String("Primary PostgreSQL hostname", "pg-primary-node")
		}

		fmt.Println("Generating PostgreSQL certificates and passwords...")
		pgCAKey, pgCACert, err := GenerateCA("PostgreSQL CA", "DE", "Karlsruhe", "Codesphere")
		if err != nil {
			return nil, fmt.Errorf("failed to generate PostgreSQL CA: %w", err)
		}
		pgPrimaryKey, pgPrimaryCert, err := GenerateServerCertificate(pgCAKey, pgCACert, pgPrimaryHost, []string{pgPrimaryIP})
		if err != nil {
			return nil, fmt.Errorf("failed to generate primary PostgreSQL certificate: %w", err)
		}
		adminPassword := GeneratePassword(32)
		replicaPassword := GeneratePassword(32)

		postgresConfig = PostgresConfig{
			CACertPem:        pgCACert,
			caCertPrivateKey: pgCAKey,
			adminPassword:    adminPassword,
			replicaPassword:  replicaPassword,
			Primary: &PostgresPrimaryConfig{
				SSLConfig: SSLConfig{
					ServerCertPem: pgPrimaryCert,
				},
				IP:         pgPrimaryIP,
				Hostname:   pgPrimaryHost,
				privateKey: pgPrimaryKey,
			},
		}

		pgReplicaIP := opts.PostgresReplicaIP
		pgReplicaName := opts.PostgresReplicaName
		if g.Interactive {
			hasReplica := prompter.Bool("Configure PostgreSQL replica", true)
			if hasReplica {
				if pgReplicaIP == "" {
					pgReplicaIP = prompter.String("Replica PostgreSQL server IP", "10.50.0.3")
				}
				if pgReplicaName == "" {
					pgReplicaName = prompter.String("Replica name (lowercase alphanumeric + underscore only)", "replica1")
				}
			}
		}

		if pgReplicaIP != "" {
			pgReplicaKey, pgReplicaCert, err := GenerateServerCertificate(pgCAKey, pgCACert, pgReplicaName, []string{pgReplicaIP})
			if err != nil {
				return nil, fmt.Errorf("failed to generate replica PostgreSQL certificate: %w", err)
			}
			postgresConfig.Replica = &PostgresReplicaConfig{
				IP:   pgReplicaIP,
				Name: pgReplicaName,
				SSLConfig: SSLConfig{
					ServerCertPem: pgReplicaCert,
				},
				privateKey: pgReplicaKey,
			}
		}

		services := []string{"auth", "deployment", "ide", "marketplace", "payment", "public_api", "team", "workspace"}
		postgresConfig.userPasswords = make(map[string]string)
		for _, service := range services {
			postgresConfig.userPasswords[service] = GeneratePassword(32)
		}
	} else {
		pgExternal := opts.PostgresExternal
		if pgExternal == "" {
			pgExternal = prompter.String("External PostgreSQL server address", "postgres.example.com:5432")
		}
		postgresConfig = PostgresConfig{
			ServerAddress: pgExternal,
		}
	}

	fmt.Println("\n=== Ceph Configuration ===")
	cephSubnet := opts.CephSubnet
	if cephSubnet == "" {
		cephSubnet = prompter.String("Ceph nodes subnet (CIDR)", "10.53.101.0/24")
	}

	var cephHosts []CephHost
	if len(opts.CephHosts) == 0 {
		numHosts := prompter.Int("Number of Ceph hosts", 3)
		cephHosts = make([]CephHost, numHosts)
		for i := 0; i < numHosts; i++ {
			fmt.Printf("\nCeph Host %d:\n", i+1)
			cephHosts[i].Hostname = prompter.String("  Hostname (as shown by 'hostname' command)", fmt.Sprintf("ceph-node-%d", i))
			cephHosts[i].IPAddress = prompter.String("  IP address", fmt.Sprintf("10.53.101.%d", i+2))
			cephHosts[i].IsMaster = (i == 0)
		}
	} else {
		cephHosts = make([]CephHost, len(opts.CephHosts))
		for i, host := range opts.CephHosts {
			cephHosts[i] = CephHost(host)
		}
	}

	fmt.Println("Generating Ceph SSH keys...")
	cephSSHPub, cephSSHPriv, err := GenerateSSHKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate Ceph SSH keys: %w", err)
	}

	cephConfig := CephConfig{
		CephAdmSSHKey: CephSSHKey{
			PublicKey: cephSSHPub,
		},
		NodesSubnet:   cephSubnet,
		Hosts:         cephHosts,
		sshPrivateKey: cephSSHPriv,
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
	}

	fmt.Println("\n=== Kubernetes Configuration ===")
	k8sManaged := opts.K8sManaged
	if g.Interactive {
		k8sManaged = prompter.Bool("Use Codesphere-managed Kubernetes (k0s)", true)
	}

	var k8sConfig KubernetesConfig
	k8sConfig.ManagedByCodesphere = k8sManaged

	if k8sManaged {
		k8sAPIServer := opts.K8sAPIServer
		if k8sAPIServer == "" {
			k8sAPIServer = prompter.String("Kubernetes API server host (LB/DNS/IP)", "10.50.0.2")
		}
		var k8sControlPlane []string
		if len(opts.K8sControlPlane) == 0 {
			k8sControlPlane = prompter.StringSlice("Control plane IP addresses (comma-separated)", []string{"10.50.0.2"})
		} else {
			k8sControlPlane = opts.K8sControlPlane
		}
		var k8sWorkers []string
		if len(opts.K8sWorkers) == 0 {
			k8sWorkers = prompter.StringSlice("Worker node IP addresses (comma-separated)", []string{"10.50.0.2", "10.50.0.3", "10.50.0.4"})
		} else {
			k8sWorkers = opts.K8sWorkers
		}

		k8sConfig.APIServerHost = k8sAPIServer
		k8sConfig.ControlPlanes = make([]K8sNode, len(k8sControlPlane))
		for i, ip := range k8sControlPlane {
			k8sConfig.ControlPlanes[i] = K8sNode{IPAddress: ip}
		}
		k8sConfig.Workers = make([]K8sNode, len(k8sWorkers))
		for i, ip := range k8sWorkers {
			k8sConfig.Workers[i] = K8sNode{IPAddress: ip}
		}
		k8sConfig.needsKubeConfig = false
	} else {
		k8sPodCIDR := opts.K8sPodCIDR
		if k8sPodCIDR == "" {
			k8sPodCIDR = prompter.String("Pod CIDR of external cluster", "100.96.0.0/11")
		}
		k8sServiceCIDR := opts.K8sServiceCIDR
		if k8sServiceCIDR == "" {
			k8sServiceCIDR = prompter.String("Service CIDR of external cluster", "100.64.0.0/13")
		}
		k8sConfig.PodCIDR = k8sPodCIDR
		k8sConfig.ServiceCIDR = k8sServiceCIDR
		k8sConfig.needsKubeConfig = true
		fmt.Println("Note: You'll need to provide kubeconfig in the vault file for external Kubernetes")
	}

	fmt.Println("\n=== Cluster Gateway Configuration ===")
	gatewayType := opts.ClusterGatewayType
	if gatewayType == "" {
		gatewayType = prompter.Choice("Gateway service type", []string{"LoadBalancer", "ExternalIP"}, "LoadBalancer")
	}
	var gatewayIPs []string
	if gatewayType == "ExternalIP" && len(opts.ClusterGatewayIPs) == 0 {
		gatewayIPs = prompter.StringSlice("Gateway IP addresses (comma-separated)", []string{"10.51.0.2", "10.51.0.3"})
	} else {
		gatewayIPs = opts.ClusterGatewayIPs
	}

	publicGatewayType := opts.ClusterPublicGatewayType
	if publicGatewayType == "" {
		publicGatewayType = prompter.Choice("Public gateway service type", []string{"LoadBalancer", "ExternalIP"}, "LoadBalancer")
	}
	var publicGatewayIPs []string
	if publicGatewayType == "ExternalIP" && len(opts.ClusterPublicGatewayIPs) == 0 {
		publicGatewayIPs = prompter.StringSlice("Public gateway IP addresses (comma-separated)", []string{"10.52.0.2", "10.52.0.3"})
	} else {
		publicGatewayIPs = opts.ClusterPublicGatewayIPs
	}

	fmt.Println("Generating ingress CA certificate...")
	ingressCAKey, ingressCACert, err := GenerateCA("Cluster Ingress CA", "DE", "Karlsruhe", "Codesphere")
	if err != nil {
		return nil, fmt.Errorf("failed to generate ingress CA: %w", err)
	}

	clusterConfig := ClusterConfig{
		Certificates: ClusterCertificates{
			CA: CAConfig{
				Algorithm:   "RSA",
				KeySizeBits: 2048,
				CertPem:     ingressCACert,
			},
		},
		Gateway: GatewayConfig{
			ServiceType: gatewayType,
			IPAddresses: gatewayIPs,
		},
		PublicGateway: GatewayConfig{
			ServiceType: publicGatewayType,
			IPAddresses: publicGatewayIPs,
		},
		ingressCAKey: ingressCAKey,
	}

	fmt.Println("\n=== MetalLB Configuration (Optional) ===")
	var metalLBConfig *MetalLBConfig
	if g.Interactive {
		metalLBEnabled := prompter.Bool("Enable MetalLB", false)
		if metalLBEnabled {
			// TODO: configopttoxyaml
			numPools := prompter.Int("Number of MetalLB IP pools", 1)
			pools := make([]MetalLBPoolDef, numPools)
			for i := 0; i < numPools; i++ {
				fmt.Printf("\nMetalLB Pool %d:\n", i+1)
				poolName := prompter.String("  Pool name", fmt.Sprintf("pool-%d", i+1))
				poolIPs := prompter.StringSlice("  IP addresses/ranges (comma-separated)", []string{"10.10.10.100-10.10.10.200"})
				pools[i] = MetalLBPoolDef{
					Name:        poolName,
					IPAddresses: poolIPs,
				}
			}
			metalLBConfig = &MetalLBConfig{
				Enabled: true,
				Pools:   pools,
			}
		}
	} else if opts.MetalLBEnabled {
		pools := make([]MetalLBPoolDef, len(opts.MetalLBPools))
		for i, pool := range opts.MetalLBPools {
			pools[i] = MetalLBPoolDef(pool)
		}
		metalLBConfig = &MetalLBConfig{
			Enabled: true,
			Pools:   pools,
		}
	}

	fmt.Println("\n=== Codesphere Application Configuration ===")
	codesphereDomain := opts.CodesphereDomain
	// todo in opts.CodesphereDomain
	if codesphereDomain == "" {
		codesphereDomain = prompter.String("Main Codesphere domain", "codesphere.yourcompany.com")
	}
	workspaceDomain := opts.CodesphereWorkspaceBaseDomain
	if workspaceDomain == "" {
		workspaceDomain = prompter.String("Workspace base domain (*.domain should point to public gateway)", "ws.yourcompany.com")
	}
	publicIP := opts.CodespherePublicIP
	if publicIP == "" {
		publicIP = prompter.String("Primary public IP for workspaces", "")
	}
	customDomain := opts.CodesphereCustomDomainBaseDomain
	if customDomain == "" {
		customDomain = prompter.String("Custom domain CNAME base", "custom.yourcompany.com")
	}
	var dnsServers []string
	if len(opts.CodesphereDNSServers) == 0 {
		dnsServers = prompter.StringSlice("DNS servers (comma-separated)", []string{"1.1.1.1", "8.8.8.8"})
	} else {
		dnsServers = opts.CodesphereDNSServers
	}

	fmt.Println("\n=== Workspace Plans Configuration ===")
	workspaceImageBomRef := opts.CodesphereWorkspaceImageBomRef
	if workspaceImageBomRef == "" {
		workspaceImageBomRef = prompter.String("Workspace agent image BOM reference", "workspace-agent-24.04")
	}
	hostingPlanCPU := opts.CodesphereHostingPlanCPU
	if hostingPlanCPU == 0 {
		hostingPlanCPU = prompter.Int("Hosting plan CPU (tenths, e.g., 10 = 1 core)", 10)
	}
	hostingPlanMemory := opts.CodesphereHostingPlanMemory
	if hostingPlanMemory == 0 {
		hostingPlanMemory = prompter.Int("Hosting plan memory (MB)", 2048)
	}
	hostingPlanStorage := opts.CodesphereHostingPlanStorage
	if hostingPlanStorage == 0 {
		hostingPlanStorage = prompter.Int("Hosting plan storage (MB)", 20480)
	}
	hostingPlanTempStorage := opts.CodesphereHostingPlanTempStorage
	if hostingPlanTempStorage == 0 {
		hostingPlanTempStorage = prompter.Int("Hosting plan temp storage (MB)", 1024)
	}
	workspacePlanName := opts.CodesphereWorkspacePlanName
	if workspacePlanName == "" {
		workspacePlanName = prompter.String("Workspace plan name", "Standard Developer")
	}
	workspacePlanMaxReplica := opts.CodesphereWorkspacePlanMaxReplica
	if workspacePlanMaxReplica == 0 {
		workspacePlanMaxReplica = prompter.Int("Max replicas per workspace", 3)
	}

	fmt.Println("Generating domain authentication keys...")
	domainAuthPub, domainAuthPriv, err := GenerateECDSAKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate domain auth keys: %w", err)
	}

	codesphereConfig := CodesphereConfig{
		Domain:                     codesphereDomain,
		WorkspaceHostingBaseDomain: workspaceDomain,
		PublicIP:                   publicIP,
		CustomDomains: CustomDomainsConfig{
			CNameBaseDomain: customDomain,
		},
		DNSServers:           dnsServers,
		Experiments:          []string{},
		domainAuthPrivateKey: domainAuthPriv,
		domainAuthPublicKey:  domainAuthPub,
		DeployConfig: DeployConfig{
			Images: map[string]DeployImage{
				"ubuntu-24.04": {
					Name:           "Ubuntu 24.04",
					SupportedUntil: "2028-05-31",
					Flavors: map[string]DeployFlavor{
						"default": {
							Image: ImageRef{
								BomRef: workspaceImageBomRef,
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
					CPUTenth:      hostingPlanCPU,
					GPUParts:      0,
					MemoryMb:      hostingPlanMemory,
					StorageMb:     hostingPlanStorage,
					TempStorageMb: hostingPlanTempStorage,
				},
			},
			WorkspacePlans: map[int]WorkspacePlan{
				1: {
					Name:          workspacePlanName,
					HostingPlanID: 1,
					MaxReplicas:   workspacePlanMaxReplica,
					OnDemand:      true,
				},
			},
		},
	}

	config := &InstallConfig{
		DataCenter: DataCenterConfig{
			ID:          dcID,
			Name:        dcName,
			City:        dcCity,
			CountryCode: dcCountry,
		},
		Secrets: SecretsConfig{
			BaseDir: secretsBaseDir,
		},
		Registry:   registryConfig,
		Postgres:   postgresConfig,
		Ceph:       cephConfig,
		Kubernetes: k8sConfig,
		Cluster:    clusterConfig,
		MetalLB:    metalLBConfig,
		Codesphere: codesphereConfig,
		ManagedServiceBackends: &ManagedServiceBackendsConfig{
			Postgres: make(map[string]interface{}),
		},
	}

	return config, nil
}

// ExtractVault extracts all sensitive data from InstallConfig into a separate vault structure.
// This separates public configuration from secrets that should be encrypted and stored securely.
func (c *InstallConfig) ExtractVault() *InstallVault {
	vault := &InstallVault{
		Secrets: []SecretEntry{},
	}

	if c.Codesphere.domainAuthPrivateKey != "" {
		vault.Secrets = append(vault.Secrets,
			SecretEntry{
				Name: "domainAuthPrivateKey",
				File: &SecretFile{
					Name:    "key.pem",
					Content: c.Codesphere.domainAuthPrivateKey,
				},
			},
			SecretEntry{
				Name: "domainAuthPublicKey",
				File: &SecretFile{
					Name:    "key.pem",
					Content: c.Codesphere.domainAuthPublicKey,
				},
			},
		)
	}

	if c.Cluster.ingressCAKey != "" {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "selfSignedCaKeyPem",
			File: &SecretFile{
				Name:    "key.pem",
				Content: c.Cluster.ingressCAKey,
			},
		})
	}

	if c.Ceph.sshPrivateKey != "" {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "cephSshPrivateKey",
			File: &SecretFile{
				Name:    "id_rsa",
				Content: c.Ceph.sshPrivateKey,
			},
		})
	}

	if c.Postgres.Primary != nil {
		if c.Postgres.adminPassword != "" {
			vault.Secrets = append(vault.Secrets, SecretEntry{
				Name: "postgresPassword",
				Fields: &SecretFields{
					Password: c.Postgres.adminPassword,
				},
			})
		}

		if c.Postgres.Primary.privateKey != "" {
			vault.Secrets = append(vault.Secrets, SecretEntry{
				Name: "postgresPrimaryServerKeyPem",
				File: &SecretFile{
					Name:    "primary.key",
					Content: c.Postgres.Primary.privateKey,
				},
			})
		}

		if c.Postgres.Replica != nil {
			if c.Postgres.replicaPassword != "" {
				vault.Secrets = append(vault.Secrets, SecretEntry{
					Name: "postgresReplicaPassword",
					Fields: &SecretFields{
						Password: c.Postgres.replicaPassword,
					},
				})
			}

			if c.Postgres.Replica.privateKey != "" {
				vault.Secrets = append(vault.Secrets, SecretEntry{
					Name: "postgresReplicaServerKeyPem",
					File: &SecretFile{
						Name:    "replica.key",
						Content: c.Postgres.Replica.privateKey,
					},
				})
			}
		}

		services := []string{"auth", "deployment", "ide", "marketplace", "payment", "public_api", "team", "workspace"}
		for _, service := range services {
			vault.Secrets = append(vault.Secrets, SecretEntry{
				Name: fmt.Sprintf("postgresUser%s", Capitalize(service)),
				Fields: &SecretFields{
					Password: service + "_blue",
				},
			})
			if password, ok := c.Postgres.userPasswords[service]; ok {
				vault.Secrets = append(vault.Secrets, SecretEntry{
					Name: fmt.Sprintf("postgresPassword%s", Capitalize(service)),
					Fields: &SecretFields{
						Password: password,
					},
				})
			}
		}
	}

	vault.Secrets = append(vault.Secrets, SecretEntry{
		Name: "managedServiceSecrets",
		Fields: &SecretFields{
			Password: "[]",
		},
	})

	if c.Registry != nil {
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

	if c.Kubernetes.needsKubeConfig {
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

func Capitalize(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "_", "")
	return strings.ToUpper(s[:1]) + s[1:]
}

func AddConfigComments(yamlData []byte) []byte {
	header := `# Codesphere Installer Configuration
# Generated by OMS CLI
#
# This file contains the main configuration for installing Codesphere Private Cloud.
# Review and modify as needed before running the installer.
#
# For more information, see the installation documentation.

`
	return append([]byte(header), yamlData...)
}

func AddVaultComments(yamlData []byte) []byte {
	header := `# Codesphere Installer Secrets
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

func ValidateConfig(config *InstallConfig) []string {
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
		if !IsValidIP(host.IPAddress) {
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

	return errors
}

func ValidateVault(vault *InstallVault) []string {
	errors := []string{}
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

	return errors
}

func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

func MarshalConfig(config *InstallConfig) ([]byte, error) {
	return yaml.Marshal(config)
}

func MarshalVault(vault *InstallVault) ([]byte, error) {
	return yaml.Marshal(vault)
}

func UnmarshalConfig(data []byte) (*InstallConfig, error) {
	var config InstallConfig
	err := yaml.Unmarshal(data, &config)
	return &config, err
}

func UnmarshalVault(data []byte) (*InstallVault, error) {
	var vault InstallVault
	err := yaml.Unmarshal(data, &vault)
	return &vault, err
}
