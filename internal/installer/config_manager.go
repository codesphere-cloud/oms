// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"net"

	"github.com/codesphere-cloud/oms/internal/util"
	"gopkg.in/yaml.v3"
)

type InstallConfigManager interface {
	CollectConfiguration(opts *ConfigOptions) (*InstallConfigContent, error)
	WriteConfigAndVault(configPath, vaultPath string, withComments bool) error
}

type InstallConfig struct {
	Interactive bool
	configOpts  *ConfigOptions
	config      *InstallConfigContent
	fileIO      util.FileIO
}

func NewConfigGenerator(interactive bool) InstallConfigManager {
	return &InstallConfig{
		Interactive: interactive,
		fileIO:      &util.FilesystemWriter{},
	}
}

func (g *InstallConfig) CollectConfiguration(opts *ConfigOptions) (*InstallConfigContent, error) {
	g.configOpts = opts

	collectedOpts, err := g.collectConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to collect configuration: %w", err)
	}

	config, err := g.convertConfig(collectedOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to convert configuration: %w", err)
	}

	if err := g.generateSecrets(config); err != nil {
		return nil, fmt.Errorf("failed to generate secrets: %w", err)
	}

	g.config = config

	return config, nil
}

func (g *InstallConfig) convertConfig(collected *collectedConfig) (*InstallConfigContent, error) {
	config := &InstallConfigContent{
		DataCenter: DataCenterConfig{
			ID:          collected.dcID,
			Name:        collected.dcName,
			City:        collected.dcCity,
			CountryCode: collected.dcCountry,
		},
		Secrets: SecretsConfig{
			BaseDir: collected.secretsBaseDir,
		},
	}

	if collected.registryServer != "" {
		config.Registry = &RegistryConfig{
			Server:              collected.registryServer,
			ReplaceImagesInBom:  collected.registryReplaceImages,
			LoadContainerImages: collected.registryLoadContainerImgs,
		}
	}

	if collected.pgMode == "install" {
		config.Postgres = PostgresConfig{
			Primary: &PostgresPrimaryConfig{
				IP:       collected.pgPrimaryIP,
				Hostname: collected.pgPrimaryHost,
			},
		}

		if collected.pgReplicaIP != "" {
			config.Postgres.Replica = &PostgresReplicaConfig{
				IP:   collected.pgReplicaIP,
				Name: collected.pgReplicaName,
			}
		}
	} else {
		config.Postgres = PostgresConfig{
			ServerAddress: collected.pgExternal,
		}
	}

	config.Ceph = CephConfig{
		NodesSubnet: collected.cephSubnet,
		Hosts:       collected.cephHosts,
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

	config.Kubernetes = KubernetesConfig{
		ManagedByCodesphere: collected.k8sManaged,
	}

	if collected.k8sManaged {
		config.Kubernetes.APIServerHost = collected.k8sAPIServer
		config.Kubernetes.ControlPlanes = make([]K8sNode, len(collected.k8sControlPlane))
		for i, ip := range collected.k8sControlPlane {
			config.Kubernetes.ControlPlanes[i] = K8sNode{IPAddress: ip}
		}
		config.Kubernetes.Workers = make([]K8sNode, len(collected.k8sWorkers))
		for i, ip := range collected.k8sWorkers {
			config.Kubernetes.Workers[i] = K8sNode{IPAddress: ip}
		}
		config.Kubernetes.needsKubeConfig = false
	} else {
		config.Kubernetes.PodCIDR = collected.k8sPodCIDR
		config.Kubernetes.ServiceCIDR = collected.k8sServiceCIDR
		config.Kubernetes.needsKubeConfig = true
	}

	config.Cluster = ClusterConfig{
		Certificates: ClusterCertificates{
			CA: CAConfig{
				Algorithm:   "RSA",
				KeySizeBits: 2048,
			},
		},
		Gateway: GatewayConfig{
			ServiceType: collected.gatewayType,
			IPAddresses: collected.gatewayIPs,
		},
		PublicGateway: GatewayConfig{
			ServiceType: collected.publicGatewayType,
			IPAddresses: collected.publicGatewayIPs,
		},
	}

	if collected.metalLBEnabled {
		config.MetalLB = &MetalLBConfig{
			Enabled: true,
			Pools:   collected.metalLBPools,
		}
	}

	config.Codesphere = CodesphereConfig{
		Domain:                     collected.codesphereDomain,
		WorkspaceHostingBaseDomain: collected.workspaceDomain,
		PublicIP:                   collected.publicIP,
		CustomDomains: CustomDomainsConfig{
			CNameBaseDomain: collected.customDomain,
		},
		DNSServers:  collected.dnsServers,
		Experiments: []string{},
		DeployConfig: DeployConfig{
			Images: map[string]DeployImage{
				"ubuntu-24.04": {
					Name:           "Ubuntu 24.04",
					SupportedUntil: "2028-05-31",
					Flavors: map[string]DeployFlavor{
						"default": {
							Image: ImageRef{
								BomRef: collected.workspaceImageBomRef,
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
					CPUTenth:      collected.hostingPlanCPU,
					GPUParts:      0,
					MemoryMb:      collected.hostingPlanMemory,
					StorageMb:     collected.hostingPlanStorage,
					TempStorageMb: collected.hostingPlanTempStorage,
				},
			},
			WorkspacePlans: map[int]WorkspacePlan{
				1: {
					Name:          collected.workspacePlanName,
					HostingPlanID: 1,
					MaxReplicas:   collected.workspacePlanMaxReplica,
					OnDemand:      true,
				},
			},
		},
	}

	config.ManagedServiceBackends = &ManagedServiceBackendsConfig{
		Postgres: make(map[string]interface{}),
	}

	return config, nil
}

func (g *InstallConfig) WriteConfigAndVault(configPath, vaultPath string, withComments bool) error {
	if g.config == nil {
		return fmt.Errorf("no configuration collected - call CollectConfiguration first")
	}

	configYAML, err := MarshalConfig(g.config)
	if err != nil {
		return fmt.Errorf("failed to marshal config.yaml: %w", err)
	}

	if withComments {
		configYAML = AddConfigComments(configYAML)
	}

	if err := g.fileIO.CreateAndWrite(configPath, configYAML, "Configuration"); err != nil {
		return err
	}

	vault := g.config.ExtractVault()
	vaultYAML, err := MarshalVault(vault)
	if err != nil {
		return fmt.Errorf("failed to marshal vault.yaml: %w", err)
	}

	if withComments {
		vaultYAML = AddVaultComments(vaultYAML)
	}

	if err := g.fileIO.CreateAndWrite(vaultPath, vaultYAML, "Secrets"); err != nil {
		return err
	}

	return nil
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

func ValidateConfig(config *InstallConfigContent) []string {
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

func MarshalConfig(config *InstallConfigContent) ([]byte, error) {
	return yaml.Marshal(config)
}

func MarshalVault(vault *InstallVault) ([]byte, error) {
	return yaml.Marshal(vault)
}

func UnmarshalConfig(data []byte) (*InstallConfigContent, error) {
	var config InstallConfigContent
	err := yaml.Unmarshal(data, &config)
	return &config, err
}

func UnmarshalVault(data []byte) (*InstallVault, error) {
	var vault InstallVault
	err := yaml.Unmarshal(data, &vault)
	return &vault, err
}
