// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"net"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
	"gopkg.in/yaml.v3"
)

type InstallConfigManager interface {
	CollectInteractively() error

	SetConfig(config *files.RootConfig)

	SetProfileValues(profile string) error

	Validate() error

	GenerateSecrets() error

	WriteInstallConfig(configPath string, withComments bool) error

	WriteVault(vaultPath string, withComments bool) error
}

type InstallConfig struct {
	fileIO util.FileIO
	config *files.RootConfig
}

func NewConfigGenerator() InstallConfigManager {
	return &InstallConfig{
		fileIO: &util.FilesystemWriter{},
		config: nil,
	}
}

func (g *InstallConfig) CollectInteractively() error {
	prompter := NewPrompter(true)
	collected := &files.CollectedConfig{}
	g.collectAllConfigs(prompter, &files.ConfigOptions{}, collected)

	config, err := g.convertConfig(collected)
	if err != nil {
		return fmt.Errorf("failed to convert configuration: %w", err)
	}

	g.config = config
	return nil
}

func (g *InstallConfig) SetConfig(config *files.RootConfig) {
	g.config = config
}

func (g *InstallConfig) SetProfileValues(profile string) error {
	if profile == "" {
		return nil
	}

	if g.config == nil {
		return fmt.Errorf("config not set, cannot apply profile")
	}

	return nil
}

func (g *InstallConfig) Validate() error {
	if g.config == nil {
		return fmt.Errorf("config not set, cannot validate")
	}

	errors := ValidateConfig(g.config)
	if len(errors) > 0 {
		var errMsg strings.Builder
		errMsg.WriteString("configuration validation failed:\n")
		for _, err := range errors {
			errMsg.WriteString(fmt.Sprintf("  - %s\n", err))
		}
		return fmt.Errorf("%s", errMsg.String())
	}

	return nil
}

func (g *InstallConfig) GenerateSecrets() error {
	if g.config == nil {
		return fmt.Errorf("config not set, cannot generate secrets")
	}
	return g.generateSecrets(g.config)
}

func (g *InstallConfig) WriteInstallConfig(configPath string, withComments bool) error {
	if g.config == nil {
		return fmt.Errorf("no configuration provided - config is nil")
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

	return nil
}

func (g *InstallConfig) WriteVault(vaultPath string, withComments bool) error {
	if g.config == nil {
		return fmt.Errorf("no configuration provided - config is nil")
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

func FromConfigOptions(opts *files.ConfigOptions) (*files.RootConfig, error) {
	config := &files.RootConfig{
		DataCenter: files.DataCenterConfig{
			ID:          opts.DatacenterID,
			Name:        opts.DatacenterName,
			City:        opts.DatacenterCity,
			CountryCode: opts.DatacenterCountryCode,
		},
		Secrets: files.SecretsConfig{
			BaseDir: opts.SecretsBaseDir,
		},
	}

	if opts.RegistryServer != "" {
		config.Registry = files.RegistryConfig{
			Server:              opts.RegistryServer,
			ReplaceImagesInBom:  opts.RegistryReplaceImages,
			LoadContainerImages: opts.RegistryLoadContainerImgs,
		}
	}

	if opts.PostgresMode == "install" {
		config.Postgres = files.PostgresConfig{
			Primary: &files.PostgresPrimaryConfig{
				IP:       opts.PostgresPrimaryIP,
				Hostname: opts.PostgresPrimaryHost,
			},
		}

		if opts.PostgresReplicaIP != "" {
			config.Postgres.Replica = &files.PostgresReplicaConfig{
				IP:   opts.PostgresReplicaIP,
				Name: opts.PostgresReplicaName,
			}
		}
	} else if opts.PostgresExternal != "" {
		config.Postgres = files.PostgresConfig{
			ServerAddress: opts.PostgresExternal,
		}
	}

	cephHosts := make([]files.CephHost, len(opts.CephHosts))
	for i, host := range opts.CephHosts {
		cephHosts[i] = files.CephHost(host)
	}

	config.Ceph = files.CephConfig{
		NodesSubnet: opts.CephSubnet,
		Hosts:       cephHosts,
		OSDs: []files.CephOSD{
			{
				SpecID: "default",
				Placement: files.CephPlacement{
					HostPattern: "*",
				},
				DataDevices: files.CephDataDevices{
					Size:  "240G:300G",
					Limit: 1,
				},
				DBDevices: files.CephDBDevices{
					Size:  "120G:150G",
					Limit: 1,
				},
			},
		},
	}

	config.Kubernetes = files.KubernetesConfig{
		ManagedByCodesphere: opts.K8sManaged,
	}

	if opts.K8sManaged {
		config.Kubernetes.APIServerHost = opts.K8sAPIServer
		config.Kubernetes.ControlPlanes = make([]files.K8sNode, len(opts.K8sControlPlane))
		for i, ip := range opts.K8sControlPlane {
			config.Kubernetes.ControlPlanes[i] = files.K8sNode{IPAddress: ip}
		}
		config.Kubernetes.Workers = make([]files.K8sNode, len(opts.K8sWorkers))
		for i, ip := range opts.K8sWorkers {
			config.Kubernetes.Workers[i] = files.K8sNode{IPAddress: ip}
		}
		config.Kubernetes.NeedsKubeConfig = false
	} else {
		config.Kubernetes.PodCIDR = opts.K8sPodCIDR
		config.Kubernetes.ServiceCIDR = opts.K8sServiceCIDR
		config.Kubernetes.NeedsKubeConfig = true
	}

	config.Cluster = files.ClusterConfig{
		Certificates: files.ClusterCertificates{
			CA: files.CAConfig{
				Algorithm:   "RSA",
				KeySizeBits: 2048,
			},
		},
		Gateway: files.GatewayConfig{
			ServiceType: opts.ClusterGatewayType,
			IPAddresses: opts.ClusterGatewayIPs,
		},
		PublicGateway: files.GatewayConfig{
			ServiceType: opts.ClusterPublicGatewayType,
			IPAddresses: opts.ClusterPublicGatewayIPs,
		},
	}

	if opts.MetalLBEnabled {
		pools := make([]files.MetalLBPoolDef, len(opts.MetalLBPools))
		for i, pool := range opts.MetalLBPools {
			pools[i] = files.MetalLBPoolDef(pool)
		}
		config.MetalLB = &files.MetalLBConfig{
			Enabled: true,
			Pools:   pools,
		}
	}

	config.Codesphere = files.CodesphereConfig{
		Domain:                     opts.CodesphereDomain,
		WorkspaceHostingBaseDomain: opts.CodesphereWorkspaceBaseDomain,
		PublicIP:                   opts.CodespherePublicIP,
		CustomDomains: files.CustomDomainsConfig{
			CNameBaseDomain: opts.CodesphereCustomDomainBaseDomain,
		},
		DNSServers:  opts.CodesphereDNSServers,
		Experiments: []string{},
		DeployConfig: files.DeployConfig{
			Images: map[string]files.ImageConfig{
				"ubuntu-24.04": {
					Name:           "Ubuntu 24.04",
					SupportedUntil: "2028-05-31",
					Flavors: map[string]files.FlavorConfig{
						"default": {
							Image: files.ImageRef{
								BomRef: opts.CodesphereWorkspaceImageBomRef,
							},
							Pool: map[int]int{1: 1},
						},
					},
				},
			},
		},
		Plans: files.PlansConfig{
			HostingPlans: map[int]files.HostingPlan{
				1: {
					CPUTenth:      opts.CodesphereHostingPlanCPU,
					GPUParts:      0,
					MemoryMb:      opts.CodesphereHostingPlanMemory,
					StorageMb:     opts.CodesphereHostingPlanStorage,
					TempStorageMb: opts.CodesphereHostingPlanTempStorage,
				},
			},
			WorkspacePlans: map[int]files.WorkspacePlan{
				1: {
					Name:          opts.CodesphereWorkspacePlanName,
					HostingPlanID: 1,
					MaxReplicas:   opts.CodesphereWorkspacePlanMaxReplica,
					OnDemand:      true,
				},
			},
		},
	}

	config.ManagedServiceBackends = &files.ManagedServiceBackendsConfig{
		Postgres: make(map[string]interface{}),
	}

	return config, nil
}

func (g *InstallConfig) collectAllConfigs(prompter *Prompter, opts *files.ConfigOptions, collected *files.CollectedConfig) {
	g.collectDatacenterConfig(prompter, opts, collected)
	g.collectRegistryConfig(prompter, opts, collected)
	g.collectPostgresConfig(prompter, opts, collected)
	g.collectCephConfig(prompter, opts, collected)
	g.collectK8sConfig(prompter, opts, collected)
	g.collectGatewayConfig(prompter, opts, collected)
	g.collectMetalLBConfig(prompter, opts, collected)
	g.collectCodesphereConfig(prompter, opts, collected)
}

func (g *InstallConfig) convertConfig(collected *files.CollectedConfig) (*files.RootConfig, error) {
	config := &files.RootConfig{
		DataCenter: files.DataCenterConfig{
			ID:          collected.DcID,
			Name:        collected.DcName,
			City:        collected.DcCity,
			CountryCode: collected.DcCountry,
		},
		Secrets: files.SecretsConfig{
			BaseDir: collected.SecretsBaseDir,
		},
	}

	if collected.RegistryServer != "" {
		config.Registry = files.RegistryConfig{
			Server:              collected.RegistryServer,
			ReplaceImagesInBom:  collected.RegistryReplaceImages,
			LoadContainerImages: collected.RegistryLoadContainerImgs,
		}
	}

	if collected.PgMode == "install" {
		config.Postgres = files.PostgresConfig{
			Primary: &files.PostgresPrimaryConfig{
				IP:       collected.PgPrimaryIP,
				Hostname: collected.PgPrimaryHost,
			},
		}

		if collected.PgReplicaIP != "" {
			config.Postgres.Replica = &files.PostgresReplicaConfig{
				IP:   collected.PgReplicaIP,
				Name: collected.PgReplicaName,
			}
		}
	} else {
		config.Postgres = files.PostgresConfig{
			ServerAddress: collected.PgExternal,
		}
	}

	config.Ceph = files.CephConfig{
		NodesSubnet: collected.CephSubnet,
		Hosts:       collected.CephHosts,
		OSDs: []files.CephOSD{
			{
				SpecID: "default",
				Placement: files.CephPlacement{
					HostPattern: "*",
				},
				DataDevices: files.CephDataDevices{
					Size:  "240G:300G",
					Limit: 1,
				},
				DBDevices: files.CephDBDevices{
					Size:  "120G:150G",
					Limit: 1,
				},
			},
		},
	}

	config.Kubernetes = files.KubernetesConfig{
		ManagedByCodesphere: collected.K8sManaged,
	}

	if collected.K8sManaged {
		config.Kubernetes.APIServerHost = collected.K8sAPIServer
		config.Kubernetes.ControlPlanes = make([]files.K8sNode, len(collected.K8sControlPlane))
		for i, ip := range collected.K8sControlPlane {
			config.Kubernetes.ControlPlanes[i] = files.K8sNode{IPAddress: ip}
		}
		config.Kubernetes.Workers = make([]files.K8sNode, len(collected.K8sWorkers))
		for i, ip := range collected.K8sWorkers {
			config.Kubernetes.Workers[i] = files.K8sNode{IPAddress: ip}
		}
		config.Kubernetes.NeedsKubeConfig = false
	} else {
		config.Kubernetes.PodCIDR = collected.K8sPodCIDR
		config.Kubernetes.ServiceCIDR = collected.K8sServiceCIDR
		config.Kubernetes.NeedsKubeConfig = true
	}

	config.Cluster = files.ClusterConfig{
		Certificates: files.ClusterCertificates{
			CA: files.CAConfig{
				Algorithm:   "RSA",
				KeySizeBits: 2048,
			},
		},
		Gateway: files.GatewayConfig{
			ServiceType: collected.GatewayType,
			IPAddresses: collected.GatewayIPs,
		},
		PublicGateway: files.GatewayConfig{
			ServiceType: collected.PublicGatewayType,
			IPAddresses: collected.PublicGatewayIPs,
		},
	}

	if collected.MetalLBEnabled {
		config.MetalLB = &files.MetalLBConfig{
			Enabled: true,
			Pools:   collected.MetalLBPools,
		}
	}

	config.Codesphere = files.CodesphereConfig{
		Domain:                     collected.CodesphereDomain,
		WorkspaceHostingBaseDomain: collected.WorkspaceDomain,
		PublicIP:                   collected.PublicIP,
		CustomDomains: files.CustomDomainsConfig{
			CNameBaseDomain: collected.CustomDomain,
		},
		DNSServers:  collected.DnsServers,
		Experiments: []string{},
		DeployConfig: files.DeployConfig{
			Images: map[string]files.ImageConfig{
				"ubuntu-24.04": {
					Name:           "Ubuntu 24.04",
					SupportedUntil: "2028-05-31",
					Flavors: map[string]files.FlavorConfig{
						"default": {
							Image: files.ImageRef{
								BomRef: collected.WorkspaceImageBomRef,
							},
							Pool: map[int]int{1: 1},
						},
					},
				},
			},
		},
		Plans: files.PlansConfig{
			HostingPlans: map[int]files.HostingPlan{
				1: {
					CPUTenth:      collected.HostingPlanCPU,
					GPUParts:      0,
					MemoryMb:      collected.HostingPlanMemory,
					StorageMb:     collected.HostingPlanStorage,
					TempStorageMb: collected.HostingPlanTempStorage,
				},
			},
			WorkspacePlans: map[int]files.WorkspacePlan{
				1: {
					Name:          collected.WorkspacePlanName,
					HostingPlanID: 1,
					MaxReplicas:   collected.WorkspacePlanMaxReplica,
					OnDemand:      true,
				},
			},
		},
	}

	config.ManagedServiceBackends = &files.ManagedServiceBackendsConfig{
		Postgres: make(map[string]interface{}),
	}

	return config, nil
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

func ValidateConfig(config *files.RootConfig) []string {
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

func ValidateVault(vault *files.InstallVault) []string {
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

func MarshalConfig(config *files.RootConfig) ([]byte, error) {
	return yaml.Marshal(config)
}

func MarshalVault(vault *files.InstallVault) ([]byte, error) {
	return yaml.Marshal(vault)
}

func UnmarshalConfig(data []byte) (*files.RootConfig, error) {
	var config files.RootConfig
	err := yaml.Unmarshal(data, &config)
	return &config, err
}

func UnmarshalVault(data []byte) (*files.InstallVault, error) {
	var vault files.InstallVault
	err := yaml.Unmarshal(data, &vault)
	return &vault, err
}
