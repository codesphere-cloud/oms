// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

type InstallConfigContent struct {
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

type collectedConfig struct {
	// Datacenter
	dcID           int
	dcName         string
	dcCity         string
	dcCountry      string
	secretsBaseDir string

	// Registry
	registryServer            string
	registryReplaceImages     bool
	registryLoadContainerImgs bool

	// PostgreSQL
	pgMode        string
	pgPrimaryIP   string
	pgPrimaryHost string
	pgReplicaIP   string
	pgReplicaName string
	pgExternal    string

	// Ceph
	cephSubnet string
	cephHosts  []CephHost

	// Kubernetes
	k8sManaged      bool
	k8sAPIServer    string
	k8sControlPlane []string
	k8sWorkers      []string
	k8sPodCIDR      string
	k8sServiceCIDR  string

	// Cluster Gateway
	gatewayType       string
	gatewayIPs        []string
	publicGatewayType string
	publicGatewayIPs  []string

	// MetalLB
	metalLBEnabled bool
	metalLBPools   []MetalLBPoolDef

	// Codesphere
	codesphereDomain        string
	workspaceDomain         string
	publicIP                string
	customDomain            string
	dnsServers              []string
	workspaceImageBomRef    string
	hostingPlanCPU          int
	hostingPlanMemory       int
	hostingPlanStorage      int
	hostingPlanTempStorage  int
	workspacePlanName       string
	workspacePlanMaxReplica int
}
