// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files

import (
	"fmt"
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
	ManagedServiceBackends *ManagedServiceBackendsConfig `yaml:"managedServiceBackends,omitempty"`
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

	// Stored separately in vault
	Username string `yaml:"-"`
	Password string `yaml:"-"`
}

type PostgresConfig struct {
	Mode          string                 `yaml:"mode,omitempty"`
	CACertPem     string                 `yaml:"caCertPem,omitempty"`
	Primary       *PostgresPrimaryConfig `yaml:"primary,omitempty"`
	Replica       *PostgresReplicaConfig `yaml:"replica,omitempty"`
	ServerAddress string                 `yaml:"serverAddress,omitempty"`

	// Stored separately in vault
	CaCertPrivateKey  string            `yaml:"-"`
	AdminPassword     string            `yaml:"-"`
	ReplicaPassword   string            `yaml:"-"`
	ReplicaPrivateKey string            `yaml:"-"`
	UserPasswords     map[string]string `yaml:"-"`
}

type PostgresPrimaryConfig struct {
	SSLConfig SSLConfig `yaml:"sslConfig"`
	IP        string    `yaml:"ip"`
	Hostname  string    `yaml:"hostname"`

	PrivateKey string `yaml:"-"`
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

	SshPrivateKey string `yaml:"-"`
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
	NeedsKubeConfig bool `yaml:"-"`
}

type K8sNode struct {
	IPAddress string `yaml:"ipAddress"`
}

type ClusterConfig struct {
	Certificates  ClusterCertificates `yaml:"certificates"`
	Monitoring    *MonitoringConfig   `yaml:"monitoring,omitempty"`
	Gateway       GatewayConfig       `yaml:"gateway"`
	PublicGateway GatewayConfig       `yaml:"publicGateway"`

	IngressCAKey string `yaml:"-"`
}

type ClusterCertificates struct {
	CA   CAConfig    `yaml:"ca"`
	ACME *ACMEConfig `yaml:"acme,omitempty"`
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
	Solver               ACMESolver `yaml:"solver"`

	EABKeyID  string `yaml:"-"`
	EABMacKey string `yaml:"-"`
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

	DomainAuthPrivateKey string `yaml:"-"`
	DomainAuthPublicKey  string `yaml:"-"`
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

	ClientID     string `yaml:"-"`
	ClientSecret string `yaml:"-"`
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

type CephHostConfig struct {
	Hostname  string
	IPAddress string
	IsMaster  bool
}

type MetalLBPool struct {
	Name        string
	IPAddresses []string
}

// Marshal serializes the RootConfig to YAML
func (c *RootConfig) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}

// Unmarshal deserializes YAML data into the RootConfig
func (c *RootConfig) Unmarshal(data []byte) error {
	return yaml.Unmarshal(data, c)
}

func NewRootConfig() RootConfig {
	return RootConfig{
		Registry:               &RegistryConfig{},
		MetalLB:                &MetalLBConfig{},
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

func (c *RootConfig) ExtractWorkspaceDockerfiles() map[string]string {
	dockerfiles := make(map[string]string)
	for _, imageConfig := range c.Codesphere.DeployConfig.Images {
		for _, flavor := range imageConfig.Flavors {
			if flavor.Image.Dockerfile != "" {
				dockerfiles[flavor.Image.Dockerfile] = flavor.Image.BomRef
			}
		}
	}
	return dockerfiles
}

func (c *RootConfig) ExtractVault() *InstallVault {
	vault := &InstallVault{
		Secrets: []SecretEntry{},
	}

	c.addCodesphereSecrets(vault)
	c.addIngressCASecret(vault)
	c.addACMESecrets(vault)
	c.addCephSecrets(vault)
	c.addPostgresSecrets(vault)
	c.addManagedServiceSecrets(vault)
	c.addRegistrySecrets(vault)
	c.addKubeConfigSecret(vault)

	return vault
}

func (c *RootConfig) addCodesphereSecrets(vault *InstallVault) {
	if c.Codesphere.DomainAuthPrivateKey != "" {
		vault.Secrets = append(vault.Secrets,
			SecretEntry{
				Name: "domainAuthPrivateKey",
				File: &SecretFile{
					Name:    "key.pem",
					Content: c.Codesphere.DomainAuthPrivateKey,
				},
			},
			SecretEntry{
				Name: "domainAuthPublicKey",
				File: &SecretFile{
					Name:    "key.pem",
					Content: c.Codesphere.DomainAuthPublicKey,
				},
			},
		)
	}

	// GitHub secrets
	if c.Codesphere.GitProviders != nil && c.Codesphere.GitProviders.GitHub != nil {
		if c.Codesphere.GitProviders.GitHub.OAuth.ClientID != "" {
			vault.Secrets = append(vault.Secrets, SecretEntry{
				Name: "githubAppsClientId",
				Fields: &SecretFields{
					Password: c.Codesphere.GitProviders.GitHub.OAuth.ClientID,
				},
			})
		}
		if c.Codesphere.GitProviders.GitHub.OAuth.ClientSecret != "" {
			vault.Secrets = append(vault.Secrets, SecretEntry{
				Name: "githubAppsClientSecret",
				Fields: &SecretFields{
					Password: c.Codesphere.GitProviders.GitHub.OAuth.ClientSecret,
				},
			})
		}
	}
}

func (c *RootConfig) addIngressCASecret(vault *InstallVault) {
	if c.Cluster.IngressCAKey != "" {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "selfSignedCaKeyPem",
			File: &SecretFile{
				Name:    "key.pem",
				Content: c.Cluster.IngressCAKey,
			},
		})
	}
}

func (c *RootConfig) addACMESecrets(vault *InstallVault) {
	if c.Cluster.Certificates.ACME == nil || !c.Cluster.Certificates.ACME.Enabled {
		return
	}

	if c.Cluster.Certificates.ACME.EABKeyID != "" {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "acmeEabKeyId",
			Fields: &SecretFields{
				Password: c.Cluster.Certificates.ACME.EABKeyID,
			},
		})
	}

	if c.Cluster.Certificates.ACME.EABMacKey != "" {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "acmeEabMacKey",
			Fields: &SecretFields{
				Password: c.Cluster.Certificates.ACME.EABMacKey,
			},
		})
	}

	if c.Cluster.Certificates.ACME.Solver.DNS01 != nil {
		for key, value := range c.Cluster.Certificates.ACME.Solver.DNS01.Secrets {
			vault.Secrets = append(vault.Secrets, SecretEntry{
				Name: fmt.Sprintf("acmeDNS01%s", Capitalize(key)),
				Fields: &SecretFields{
					Password: value,
				},
			})
		}
	}
}

func (c *RootConfig) addCephSecrets(vault *InstallVault) {
	if c.Ceph.SshPrivateKey != "" {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "cephSshPrivateKey",
			File: &SecretFile{
				Name:    "id_rsa",
				Content: c.Ceph.SshPrivateKey,
			},
		})
	}
}

func (c *RootConfig) addPostgresSecrets(vault *InstallVault) {
	if c.Postgres.Primary == nil {
		return
	}

	if c.Postgres.CaCertPrivateKey != "" {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "postgresCaKeyPem",
			File: &SecretFile{
				Name:    "ca.key",
				Content: c.Postgres.CaCertPrivateKey,
			},
		})
	}

	if c.Postgres.AdminPassword != "" {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "postgresPassword",
			Fields: &SecretFields{
				Password: c.Postgres.AdminPassword,
			},
		})
	}

	if c.Postgres.Primary.PrivateKey != "" {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "postgresPrimaryServerKeyPem",
			File: &SecretFile{
				Name:    "primary.key",
				Content: c.Postgres.Primary.PrivateKey,
			},
		})
	}

	if c.Postgres.ReplicaPassword != "" {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "postgresReplicaPassword",
			Fields: &SecretFields{
				Password: c.Postgres.ReplicaPassword,
			},
		})
	}

	vault.Secrets = append(vault.Secrets, SecretEntry{
		Name: "postgresReplicaServerKeyPem",
		File: &SecretFile{
			Name:    "replica.key",
			Content: c.Postgres.ReplicaPrivateKey,
		},
	})

	services := []string{"auth", "deployment", "ide", "marketplace", "payment", "public_api", "team", "workspace"}
	for _, service := range services {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: fmt.Sprintf("postgresUser%s", Capitalize(service)),
			Fields: &SecretFields{
				Password: service + "_blue",
			},
		})
		if password, ok := c.Postgres.UserPasswords[service]; ok {
			vault.Secrets = append(vault.Secrets, SecretEntry{
				Name: fmt.Sprintf("postgresPassword%s", Capitalize(service)),
				Fields: &SecretFields{
					Password: password,
				},
			})
		}
	}
}

func (c *RootConfig) addManagedServiceSecrets(vault *InstallVault) {
	vault.Secrets = append(vault.Secrets, SecretEntry{
		Name: "managedServiceSecrets",
		Fields: &SecretFields{
			Password: "[]",
		},
	})
}

func (c *RootConfig) addRegistrySecrets(vault *InstallVault) {
	if c.Registry.Server != "" {
		vault.Secrets = append(vault.Secrets,
			SecretEntry{
				Name: "registryUsername",
				Fields: &SecretFields{
					Password: c.Registry.Username,
				},
			},
			SecretEntry{
				Name: "registryPassword",
				Fields: &SecretFields{
					Password: c.Registry.Password,
				},
			},
		)
	}
}

func (c *RootConfig) addKubeConfigSecret(vault *InstallVault) {
	if c.Kubernetes.NeedsKubeConfig {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "kubeConfig",
			File: &SecretFile{
				Name:    "kubeConfig",
				Content: "# YOUR KUBECONFIG CONTENT HERE\n# Replace this with your actual kubeconfig for the external cluster\n",
			},
		})
	}
}

func Capitalize(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "_", "")
	return strings.ToUpper(s[:1]) + s[1:]
}
