// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files

import (
	"strings"

	"go.yaml.in/yaml/v3"
)

// Vault
type InstallVault struct {
	Secrets []SecretEntry `yaml:"secrets"`
}

func (v *InstallVault) Marshal() ([]byte, error) {
	return yaml.Marshal(v)
}

// GetSecret returns the entry with the given name, or nil if not found.
func (v *InstallVault) GetSecret(name string) *SecretEntry {
	for i := range v.Secrets {
		if v.Secrets[i].Name == name {
			return &v.Secrets[i]
		}
	}
	return nil
}

// SetSecret adds or updates a secret entry in the vault.
func (v *InstallVault) SetSecret(entry SecretEntry) {
	for i, s := range v.Secrets {
		if s.Name == entry.Name {
			v.Secrets[i] = entry
			return
		}
	}
	v.Secrets = append(v.Secrets, entry)
}

func (v *InstallVault) Unmarshal(data []byte) error {
	return yaml.Unmarshal(data, v)
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
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password"`
}

// RootConfig represents the relevant parts of the configuration file
type RootConfig struct {
	Datacenter             DatacenterConfig              `yaml:"dataCenter"`
	Secrets                SecretsConfig                 `yaml:"secrets"`
	Registry               *RegistryConfig               `yaml:"registry,omitempty"`
	Postgres               PostgresConfig                `yaml:"postgres"`
	Ceph                   CephConfig                    `yaml:"ceph"`
	Kubernetes             KubernetesConfig              `yaml:"kubernetes"`
	Cluster                ClusterConfig                 `yaml:"cluster"`
	MetalLB                *MetalLBConfig                `yaml:"metallb,omitempty"`
	Codesphere             CodesphereConfig              `yaml:"codesphere"`
	PcApps                 ChartValues                   `yaml:"pcApps,omitempty"`
	ManagedServiceBackends *ManagedServiceBackendsConfig `yaml:"managedServiceBackends,omitempty"`
	Operations             *OperationsConfig             `yaml:"operations,omitempty"`
}

type OperationsConfig struct {
	Skip []string `yaml:"skip"`
}

type DatacenterConfig struct {
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
	Mode          string                 `yaml:"mode,omitempty"`
	CACertPem     string                 `yaml:"caCertPem,omitempty"`
	Primary       *PostgresPrimaryConfig `yaml:"primary,omitempty"`
	Replica       *PostgresReplicaConfig `yaml:"replica,omitempty"`
	ServerAddress string                 `yaml:"serverAddress,omitempty"`
	AltName       string                 `yaml:"altName,omitempty"`
	Port          int                    `yaml:"port,omitempty"`
	Database      string                 `yaml:"database,omitempty"`
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
	Limit int    `yaml:"limit,omitempty"`
}

type CephDBDevices struct {
	Size  string `yaml:"size"`
	Limit int    `yaml:"limit,omitempty"`
}

type KubernetesConfig struct {
	ManagedByCodesphere bool      `yaml:"managedByCodesphere"`
	APIServerHost       string    `yaml:"apiServerHost,omitempty"`
	ControlPlanes       []K8sNode `yaml:"controlPlanes,omitempty"`
	Workers             []K8sNode `yaml:"workers,omitempty"`
	PodCIDR             string    `yaml:"podCidr,omitempty"`
	ServiceCIDR         string    `yaml:"serviceCidr,omitempty"`

	// Internal flag
	NeedsKubeConfig bool `yaml:"-"`
}

type K8sNode struct {
	IPAddress string `yaml:"ipAddress"`
}

type ClusterConfig struct {
	Certificates        ClusterCertificates        `yaml:"certificates"`
	CertManager         *CertManagerConfig         `yaml:"certManager,omitempty"`
	TrustManager        *TrustManagerConfig        `yaml:"trustManager,omitempty"`
	Monitoring          *MonitoringConfig          `yaml:"monitoring,omitempty"`
	Gateway             GatewayConfig              `yaml:"gateway"`
	PublicGateway       GatewayConfig              `yaml:"publicGateway"`
	RookExternalCluster *RookExternalClusterConfig `yaml:"rookExternalCluster,omitempty"`
	PgOperator          *PgOperatorConfig          `yaml:"pgOperator,omitempty"`
	BarmanCloudPlugin   *BarmanCloudPluginConfig   `yaml:"BarmanCloudPluginConfig,omitempty"`
	RgwLoadBalancer     *RgwLoadBalancerConfig     `yaml:"rgwLoadBalancer,omitempty"`
	Kyverno             *KyvernoConfig             `yaml:"kyverno,omitempty"`
}

type ClusterCertificates struct {
	CA       CAConfig      `yaml:"ca"`
	Override ChartOverride `yaml:"override,omitempty"`
}

type CAConfig struct {
	Algorithm   string `yaml:"algorithm"`
	KeySizeBits int    `yaml:"keySizeBits"`
	CertPem     string `yaml:"certPem"`
}

type ACMEConfig struct {
	Enabled              bool       `yaml:"enabled"`
	Name                 string     `yaml:"name,omitempty"`
	Email                string     `yaml:"email,omitempty"`
	Server               string     `yaml:"server,omitempty"`
	PrivateKeySecretName string     `yaml:"-"`
	Solver               ACMESolver `yaml:"-"`

	EABKeyID string `yaml:"eabKeyId,omitempty"`
}

type ACMESolver struct {
	DNS01 *ACMEDNS01Solver `yaml:"dns01,omitempty"`
}

type ACMEDNS01Solver struct {
	Provider string                 `yaml:"provider"`
	Config   map[string]interface{} `yaml:"config,omitempty"`

	Secrets map[string]string `yaml:"-"`
}

type GatewayConfig struct {
	ServiceType string            `yaml:"serviceType"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
	IPAddresses []string          `yaml:"ipAddresses,omitempty"`
	Override    ChartOverride     `yaml:"override,omitempty"`
}

type CertManagerConfig struct {
	Override ChartOverride `yaml:"override,omitempty"`
}

type TrustManagerConfig struct {
	Override ChartOverride `yaml:"override,omitempty"`
}

type RookExternalClusterConfig struct {
	Enabled bool `yaml:"enabled"`
}

type PgOperatorConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Override ChartOverride `yaml:"override,omitempty"`
}

type BarmanCloudPluginConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Override ChartOverride `yaml:"override,omitempty"`
}

type RgwLoadBalancerConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Override ChartOverride `yaml:"override,omitempty"`
}

type KyvernoConfig struct {
	Enabled bool `yaml:"enabled"`
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
	CertIssuer                 CertIssuerConfig       `yaml:"certIssuer"`
	CustomDomains              CustomDomainsConfig    `yaml:"customDomains"`
	DNSServers                 []string               `yaml:"dnsServers"`
	Internal                   []string               `yaml:"internal"`
	Preview                    map[string]bool        `yaml:"preview"`
	Features                   map[string]bool        `yaml:"features"`
	ClusterAdminEmail          string                 `yaml:"clusterAdminEmail,omitempty"`
	ExtraCAPem                 string                 `yaml:"extraCaPem,omitempty"`
	ExtraWorkspaceEnvVars      map[string]string      `yaml:"extraWorkspaceEnvVars,omitempty"`
	ExtraWorkspaceFiles        []ExtraWorkspaceFile   `yaml:"extraWorkspaceFiles,omitempty"`
	WorkspaceImages            *WorkspaceImagesConfig `yaml:"workspaceImages,omitempty"`
	DeployConfig               DeployConfig           `yaml:"deployConfig"`
	Plans                      PlansConfig            `yaml:"plans"`
	UnderprovisionFactors      *UnderprovisionFactors `yaml:"underprovisionFactors,omitempty"`
	GitProviders               *GitProvidersConfig    `yaml:"gitProviders,omitempty"`
	OAuth                      *OAuthProvidersConfig  `yaml:"oauth,omitempty"`
	ManagedServices            []ManagedServiceConfig `yaml:"managedServices,omitempty"`
	OpenBao                    *OpenBaoConfig         `yaml:"openBao,omitempty"`
	Migration                  *MigrationConfig       `yaml:"migration,omitempty"`
	TelemetryExport            *TelemetryExport       `yaml:"telemetryExport,omitempty"`
	Override                   ChartOverride          `yaml:"override,omitempty"`
}

type MigrationConfig struct {
	Postgres *MigrationPostgresConfig `yaml:"postgres,omitempty"`
}

type MigrationPostgresConfig struct {
	Host     string `yaml:"host,omitempty"`
	Port     int    `yaml:"port,omitempty"`
	Database string `yaml:"database,omitempty"`
	AltName  string `yaml:"altName,omitempty"`
}

type OpenBaoConfig struct {
	Engine string `yaml:"engine,omitempty"`
	URI    string `yaml:"uri,omitempty"`
	User   string `yaml:"user,omitempty"`
}

type OAuthProvidersConfig struct {
	Oidc *OidcOAuthProvider `yaml:"oidc,omitempty"`
}

type OidcOAuthProvider struct {
	Type      string   `yaml:"type"`
	Enabled   bool     `yaml:"enabled"`
	Name      string   `yaml:"name"`
	IssuerURL string   `yaml:"issuerUrl"`
	Scopes    []string `yaml:"scopes,omitempty"`
}

type CertIssuerType string

const (
	CertIssuerTypeSelfSigned CertIssuerType = "self-signed"
	CertIssuerTypeACME       CertIssuerType = "acme"
)

type CertIssuerConfig struct {
	Type CertIssuerType `yaml:"type,omitempty"`
	Acme *ACMEConfig    `yaml:"acme,omitempty"`
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

type DeployConfig struct {
	Images map[string]ImageConfig `yaml:"images"`
}

type ImageConfig struct {
	Name           string                  `yaml:"name"`
	SupportedUntil string                  `yaml:"supportedUntil"`
	Flavors        map[string]FlavorConfig `yaml:"flavors"`
}

type FlavorConfig struct {
	// Image can be a referenced image or a plain string
	Image ImageRef    `yaml:"image"`
	Pool  map[int]int `yaml:"pool"`
}

type TelemetryExport struct {
	RemoteEndpoint string `yaml:"remoteEndpoint"`
	RemoteExport   bool   `yaml:"remoteExport"`
	Traces         bool   `yaml:"traces"`
	TraceEndpoint  string `yaml:"traceEndpoint,omitempty"`
	SpanMetrics    bool   `yaml:"spanMetrics"`
}

type ChartOverride = map[string]interface{}
type ChartValues = map[string]interface{}

type ImageRef struct {
	BomRef     string `yaml:"bomRef,omitempty"`
	Dockerfile string `yaml:"dockerfile,omitempty"`
	// ImageName Contains the image name when it's just a plain string
	ImageName string `yaml:"-"`
}

// Type alias to avoid recursion in yaml handling
type imageRefAlias ImageRef

// UnmarshalYAML implements custom unmarshaling to support both string and object formats
func (i *ImageRef) UnmarshalYAML(node *yaml.Node) error {
	// Try to unmarshal as a plain string first
	var imageStr string
	if err := node.Decode(&imageStr); err == nil {
		i.ImageName = imageStr
		return nil
	}

	// If that fails, unmarshal as a struct with BomRef/Dockerfile
	var ref imageRefAlias
	if err := node.Decode(&ref); err != nil {
		return err
	}
	i.BomRef = ref.BomRef
	i.Dockerfile = ref.Dockerfile
	return nil
}

// MarshalYAML implements custom marshaling
func (i ImageRef) MarshalYAML() (interface{}, error) {
	// If it's a plain string, marshal as string
	if i.ImageName != "" {
		return i.ImageName, nil
	}
	// Otherwise, marshal as object
	return imageRefAlias(i), nil
}

// GetImageReference returns the actual image reference
func (i *ImageRef) GetImageReference() string {
	if i.ImageName != "" {
		return i.ImageName
	}
	return i.BomRef
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
	RedirectURI           string `yaml:"redirectUri,omitempty"`
	InstallationURI       string `yaml:"installationUri,omitempty"`
}

type ManagedServiceConfig struct {
	Name          string                 `yaml:"name"`
	API           ManagedServiceAPI      `yaml:"api,omitempty"`
	Author        string                 `yaml:"author,omitempty"`
	Category      string                 `yaml:"category,omitempty"`
	ConfigSchema  map[string]interface{} `yaml:"configSchema,omitempty"`
	DetailsSchema map[string]interface{} `yaml:"detailsSchema,omitempty"`
	SecretsSchema map[string]interface{} `yaml:"secretsSchema,omitempty"`
	Description   string                 `yaml:"description,omitempty"`
	DisplayName   string                 `yaml:"displayName,omitempty"`
	IconURL       string                 `yaml:"iconUrl,omitempty"`
	Plans         []ServicePlan          `yaml:"plans,omitempty"`
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
	Postgres         *PgManagedServiceConfig         `yaml:"postgres,omitempty"`
	S3               *S3ManagedServiceConfig         `yaml:"s3,omitempty"`
	RabbitMqOperator *RabbitMqOperatorConfig         `yaml:"rabbitMqOperator,omitempty"`
	K8sBackend       *K8sBackendManagedServiceConfig `yaml:"k8sBackend,omitempty"`
}

type MonitoringConfig struct {
	Prometheus        *PrometheusConfig       `yaml:"prometheus,omitempty"`
	BlackboxExporter  *BlackboxExporterConfig `yaml:"blackboxExporter,omitempty"`
	PushGateway       *PushGatewayConfig      `yaml:"pushGateway,omitempty"`
	Loki              *LokiConfig             `yaml:"loki,omitempty"`
	Grafana           *GrafanaConfig          `yaml:"grafana,omitempty"`
	GrafanaAlloy      *GrafanaAlloyConfig     `yaml:"grafanaAlloy,omitempty"`
	CentralOtelExport *CentralOtelConfig      `yaml:"centralOtelExport,omitempty"`
}

type PrometheusConfig struct {
	RemoteWrite *RemoteWriteConfig `yaml:"remoteWrite,omitempty"`
	Override    ChartOverride      `yaml:"override,omitempty"`
}

type RemoteWriteConfig struct {
	Enabled     bool   `yaml:"enabled"`
	ClusterName string `yaml:"clusterName,omitempty"`
	Url         string `yaml:"url,omitempty"`
	Username    string `yaml:"username,omitempty"`
	Password    string `yaml:"-"`
}

type BlackboxExporterConfig struct {
	Override ChartOverride `yaml:"override,omitempty"`
}

type PushGatewayConfig struct {
	Override ChartOverride `yaml:"override,omitempty"`
}

type LokiConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Override ChartOverride `yaml:"override,omitempty"`
}

type GrafanaConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Override ChartOverride `yaml:"override,omitempty"`
}

type GrafanaAlloyConfig struct {
	Enabled  bool                  `yaml:"enabled"`
	Loki     *LokiConnectionConfig `yaml:"loki,omitempty"`
	Override ChartOverride         `yaml:"override,omitempty"`
}

type LokiConnectionConfig struct {
	Endpoint string `yaml:"endpoint"`
	User     string `yaml:"user,omitempty"`

	Password string `yaml:"-"`
}

type CentralOtelConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Username string        `yaml:"username,omitempty"`
	Password string        `yaml:"-"`
	Override ChartOverride `yaml:"override,omitempty"`
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

type PgManagedServiceConfig struct {
	Override ChartOverride `yaml:"override,omitempty"`
}

type K8sBackendManagedServiceConfig struct {
	Override ChartOverride `yaml:"override,omitempty"`
}

type S3ManagedServiceConfig struct {
	Override ChartOverride `yaml:"override,omitempty"`
}

type RabbitMqOperatorConfig struct {
	Override ChartOverride `yaml:"override,omitempty"`
}

// Marshal serializes the RootConfig to YAML
func (c *RootConfig) Marshal() ([]byte, error) {
	c.buildACMEOverride()
	return yaml.Marshal(c)
}

// Unmarshal deserializes YAML data into the RootConfig
func (c *RootConfig) Unmarshal(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	c.extractACMESolverFromOverride()
	return nil
}

func NewRootConfig() RootConfig {
	return RootConfig{
		Registry:               &RegistryConfig{},
		MetalLB:                &MetalLBConfig{},
		PcApps:                 ChartValues{},
		ManagedServiceBackends: &ManagedServiceBackendsConfig{},
	}
}

func (c *RootConfig) ExtractBomRefs() []string {
	var bomRefs []string
	for _, imageConfig := range c.Codesphere.DeployConfig.Images {
		for _, flavor := range imageConfig.Flavors {
			if flavor.Image.BomRef != "" {
				bomRefs = append(bomRefs, flavor.Image.BomRef)
			}
		}
	}

	return bomRefs
}

func Capitalize(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "_", "")
	return strings.ToUpper(s[:1]) + s[1:]
}

// buildACMEOverride populates cluster.certificates.override with the ACME solver
// configuration from codesphere.certIssuer.acme.solver, matching the documented
// config.yaml structure.
func (c *RootConfig) buildACMEOverride() {
	if c.Codesphere.CertIssuer.Acme == nil || c.Codesphere.CertIssuer.Acme.Solver.DNS01 == nil {
		return
	}

	dns01 := c.Codesphere.CertIssuer.Acme.Solver.DNS01

	acmeOverride := map[string]interface{}{}

	// Build dnsSolver section
	if dns01.Provider != "" {
		solverConfig := map[string]interface{}{}
		if dns01.Config != nil {
			for k, v := range dns01.Config {
				solverConfig[k] = v
			}
		}
		acmeOverride["dnsSolver"] = map[string]interface{}{
			dns01.Provider: solverConfig,
		}
	}

	if c.Cluster.Certificates.Override == nil {
		c.Cluster.Certificates.Override = map[string]interface{}{}
	}

	issuers, ok := c.Cluster.Certificates.Override["issuers"].(map[string]interface{})
	if !ok {
		issuers = map[string]interface{}{}
	}

	// Merge with existing acme override (don't clobber user-provided fields like solverSecret)
	existingAcme, ok := issuers["acme"].(map[string]interface{})
	if !ok {
		existingAcme = map[string]interface{}{}
	}
	for k, v := range acmeOverride {
		existingAcme[k] = v
	}

	issuers["acme"] = existingAcme
	c.Cluster.Certificates.Override["issuers"] = issuers
}

// extractACMESolverFromOverride populates the ACMEConfig.Solver from
// cluster.certificates.override.issuers.acme.dnsSolver after unmarshaling.
func (c *RootConfig) extractACMESolverFromOverride() {
	if c.Codesphere.CertIssuer.Acme == nil {
		return
	}

	override := c.Cluster.Certificates.Override
	if override == nil {
		return
	}

	issuers, ok := override["issuers"].(map[string]interface{})
	if !ok {
		return
	}

	acmeIssuer, ok := issuers["acme"].(map[string]interface{})
	if !ok {
		return
	}

	dnsSolver, ok := acmeIssuer["dnsSolver"].(map[string]interface{})
	if !ok {
		return
	}

	// The dnsSolver map has the provider name as key
	for provider, cfg := range dnsSolver {
		solver := &ACMEDNS01Solver{
			Provider: provider,
		}
		if cfgMap, ok := cfg.(map[string]interface{}); ok && len(cfgMap) > 0 {
			solver.Config = cfgMap
		}
		c.Codesphere.CertIssuer.Acme.Solver.DNS01 = solver
		break // only one provider expected
	}
}
