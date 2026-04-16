// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/node"
)

const (
	remoteInstallConfigPath string = "/etc/codesphere/config.yaml"
)

// EnsureInstallConfig uses the local config or recovers it from an existing jumpbox if desired.
// Else it applies the minimal profile to a new config.
func (b *GCPBootstrapper) EnsureInstallConfig() error {
	// recovery will overwrite local config or create a new file
	if b.Env.RecoverConfig {
		err := b.recoverConfig()
		if err != nil {
			return fmt.Errorf("failed to recover config: %w", err)
		}
	}

	if b.fw.Exists(b.Env.InstallConfigPath) {
		err := b.icg.LoadInstallConfigFromFile(b.Env.InstallConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}

		b.Env.ExistingConfigUsed = true
	} else {
		err := b.icg.ApplyProfile("minimal")
		if err != nil {
			return fmt.Errorf("failed to apply profile: %w", err)
		}
	}

	b.Env.InstallConfig = b.icg.GetInstallConfig()

	return nil
}

// recoverConfig downloads the config and secrets from the jumpbox if it exists.
// Since recovery is done when the project or VMs are not ensured, we need to search for the jumpbox IP first.
// Returns an error if project or jumpbox does not exist or downloading fails.
func (b *GCPBootstrapper) recoverConfig() error {
	existingProject, err := b.GCPClient.GetProjectByName(b.Env.FolderID, b.Env.ProjectName)
	if err != nil {
		return fmt.Errorf("failed to find gcp project for config recovery: %w", err)
	}
	b.Env.ProjectID = existingProject.ProjectId

	jumpbox, err := b.GetNodeByName("jumpbox")
	if err != nil {
		return fmt.Errorf("failed to find jumpbox node for config recovery: %w", err)
	}
	b.Env.Jumpbox = jumpbox

	err = b.Env.Jumpbox.NodeClient.DownloadFile(jumpbox, remoteInstallConfigPath, b.Env.InstallConfigPath)
	if err != nil {
		return fmt.Errorf("failed to download install config from jumpbox: %w", err)
	}

	err = b.recoverVault()
	if err != nil {
		return fmt.Errorf("failed to recover vault: %w", err)
	}

	return nil
}

// recoverVault unencrypts the secrets file on the jumpbox and download the file to the local destination
func (b *GCPBootstrapper) recoverVault() error {
	const vaultCopyPath string = "/tmp/prod.vault.yaml"
	defer func() {
		err := b.Env.Jumpbox.RunSSHCommand("root", "rm -f "+vaultCopyPath)
		if err != nil {
			b.stlog.Logf("failed to remove unencrypted vault file for recovery: %s", err.Error())
		}
	}()

	err := b.decryptVault(vaultCopyPath)
	if err != nil {
		return fmt.Errorf("failed to create decrypted vault for recovery: %w", err)
	}

	err = b.Env.Jumpbox.NodeClient.DownloadFile(b.Env.Jumpbox, vaultCopyPath, b.Env.SecretsFilePath)
	if err != nil {
		return fmt.Errorf("failed to download secrets file from jumpbox: %w", err)
	}

	return nil
}

// updateInstallConfig update the install config, generates new secrets and writes the new config locally and to the jumpbox.
func (b *GCPBootstrapper) UpdateInstallConfig() error {
	return b.updateInstallConfig(true, true)
}

// CreateInstallConfigCheckpoint updates the install config and writes the new config locally.
// The resulting local config be used for cleanups or manual interaction on failed bootstraps.
func (b *GCPBootstrapper) CreateInstallConfigCheckpoint() error {
	return b.updateInstallConfig(false, false)
}

// updateInstallConfig update the install config, generates new secrets and writes the new config locally and to the jumpbox.
// Generating secrets and writing to jumpbox can be skipped if not needed (e.g. when creating a checkpoint and VM's are not present).
// Returns an error if any step fails.
func (b *GCPBootstrapper) updateInstallConfig(generateSecrets, copyToJumpbox bool) error {
	previousPrimaryIP := b.Env.InstallConfig.Postgres.Primary.IP
	previousPrimaryHostname := b.Env.InstallConfig.Postgres.Primary.Hostname

	installConfig, err := b.buildInstallConfig()
	if err != nil {
		return fmt.Errorf("failed to build install config: %w", err)
	}
	b.Env.InstallConfig = &installConfig

	if generateSecrets {
		if !b.Env.ExistingConfigUsed {
			err := b.icg.GenerateSecrets()
			if err != nil {
				return fmt.Errorf("failed to generate secrets: %w", err)
			}
		} else {
			if err := b.regeneratePostgresCerts(previousPrimaryIP, previousPrimaryHostname); err != nil {
				return fmt.Errorf("failed to regenerate postgres certs: %w", err)
			}
		}
	}

	if err := b.icg.WriteInstallConfig(b.Env.InstallConfigPath, true); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if err := b.icg.WriteVault(b.Env.SecretsFilePath, true); err != nil {
		return fmt.Errorf("failed to write vault file: %w", err)
	}

	if copyToJumpbox {
		err = b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, b.Env.InstallConfigPath, remoteInstallConfigPath)
		if err != nil {
			return fmt.Errorf("failed to copy install config to jumpbox: %w", err)
		}

		err = b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, b.Env.SecretsFilePath, b.Env.SecretsDir+"/prod.vault.yaml")
		if err != nil {
			return fmt.Errorf("failed to copy secrets file to jumpbox: %w", err)
		}
	}

	return nil
}

func (b *GCPBootstrapper) buildInstallConfig() (files.RootConfig, error) {
	// Copy existing config
	installConfig := *b.Env.InstallConfig

	installConfig.Datacenter.ID = b.Env.DatacenterID
	installConfig.Datacenter.City = "Karlsruhe"
	installConfig.Datacenter.CountryCode = "DE"
	installConfig.Secrets.BaseDir = b.Env.SecretsDir
	if b.Env.RegistryType != RegistryTypeGitHub {
		installConfig.Registry.ReplaceImagesInBom = true
		installConfig.Registry.LoadContainerImages = true
	}

	if installConfig.Postgres.Primary == nil {
		installConfig.Postgres.Primary = &files.PostgresPrimaryConfig{
			Hostname: b.Env.PostgreSQLNode.GetName(),
		}
	}

	if b.Env.PostgreSQLNode != nil {
		installConfig.Postgres.Primary.IP = b.Env.PostgreSQLNode.GetInternalIP()
		installConfig.Postgres.Primary.Hostname = b.Env.PostgreSQLNode.GetName()
	}

	// Ceph
	installConfig.Ceph.CsiKubeletDir = "/var/lib/k0s/kubelet"
	installConfig.Ceph.NodesSubnet = "10.10.0.0/20"
	installConfig.Ceph.Hosts = buildCephHostsConfig(b.Env.CephNodes)
	installConfig.Ceph.OSDs = []files.CephOSD{
		{
			SpecID: "default",
			Placement: files.CephPlacement{
				HostPattern: "*",
			},
			DataDevices: files.CephDataDevices{
				Size:  "50G:",
				Limit: 1,
			},
			DBDevices: files.CephDBDevices{
				Size:  "10G:50G",
				Limit: 1,
			},
		},
	}

	// K8s
	installConfig.Kubernetes = buildKubernetesConfig(b.Env.ControlPlaneNodes)

	installConfig.Cluster.Gateway.ServiceType = "LoadBalancer"
	installConfig.Cluster.Gateway.Annotations = map[string]string{
		"cloud.google.com/load-balancer-ipv4": b.Env.GatewayIP,
	}
	installConfig.Cluster.PublicGateway.ServiceType = "LoadBalancer"
	installConfig.Cluster.PublicGateway.Annotations = map[string]string{
		"cloud.google.com/load-balancer-ipv4": b.Env.PublicGatewayIP,
	}

	dnsProject := b.Env.DNSProjectID
	if b.Env.DNSProjectID == "" {
		dnsProject = b.Env.ProjectID
	}
	installConfig.Cluster.Certificates.Override = map[string]interface{}{
		"issuers": map[string]interface{}{
			"letsEncryptHttp": map[string]interface{}{
				"enabled": true,
			},
			"acme": map[string]interface{}{
				"dnsSolver": map[string]interface{}{
					"config": map[string]interface{}{
						"cloudDNS": map[string]interface{}{
							"project": dnsProject,
						},
					},
				},
			},
		},
	}
	installConfig.Codesphere.CertIssuer = files.CertIssuerConfig{
		Type: "acme",
		Acme: &files.ACMEConfig{
			Email:  "oms-testing@" + b.Env.BaseDomain,
			Server: "https://acme-v02.api.letsencrypt.org/directory",
		},
	}

	installConfig.Codesphere.Domain = "cs." + b.Env.BaseDomain
	installConfig.Codesphere.WorkspaceHostingBaseDomain = "ws." + b.Env.BaseDomain
	if len(b.Env.ControlPlaneNodes) > 1 {
		installConfig.Codesphere.PublicIP = b.Env.ControlPlaneNodes[1].GetExternalIP()
	}

	installConfig.Codesphere.CustomDomains = files.CustomDomainsConfig{
		CNameBaseDomain: "ws." + b.Env.BaseDomain,
	}
	installConfig.Codesphere.DNSServers = []string{"8.8.8.8"}
	installConfig.Codesphere.DeployConfig = bootstrap.DefaultCodesphereDeployConfig()
	installConfig.Codesphere.Plans = bootstrap.DefaultCodespherePlans()

	installConfig.Codesphere.GitProviders = &files.GitProvidersConfig{}
	if b.Env.GitHubAppName != "" && b.Env.GitHubAppClientID != "" && b.Env.GitHubAppClientSecret != "" {
		installConfig.Codesphere.GitProviders.GitHub = &files.GitProviderConfig{
			Enabled: true,
			URL:     "https://github.com",
			API: files.APIConfig{
				BaseURL: "https://api.github.com",
			},
			OAuth: files.OAuthConfig{
				Issuer:                "https://github.com",
				AuthorizationEndpoint: "https://github.com/login/oauth/authorize",
				TokenEndpoint:         "https://github.com/login/oauth/access_token",
				ClientAuthMethod:      "client_secret_post",
				RedirectURI:           "https://cs." + b.Env.BaseDomain + "/ide/auth/github/callback",
				InstallationURI:       "https://github.com/apps/" + b.Env.GitHubAppName + "/installations/new",

				ClientID:     b.Env.GitHubAppClientID,
				ClientSecret: b.Env.GitHubAppClientSecret,
			},
		}
	}
	installConfig.Codesphere.Experiments = b.Env.Experiments
	installConfig.Codesphere.Features = b.Env.FeatureFlags

	if b.Env.OpenBaoURI != "" {
		installConfig.Codesphere.OpenBao = &files.OpenBaoConfig{
			Engine:   b.Env.OpenBaoEngine,
			URI:      b.Env.OpenBaoURI,
			User:     b.Env.OpenBaoUser,
			Password: b.Env.OpenBaoPassword,
		}
	}

	return installConfig, nil
}

func buildCephHostsConfig(nodes []*node.Node) []files.CephHost {
	hosts := make([]files.CephHost, len(nodes))
	for i, node := range nodes {
		hosts[i] = files.CephHost{
			Hostname:  node.GetName(),
			IPAddress: node.GetInternalIP(),
			IsMaster:  i == 0,
		}
	}
	return hosts
}

func buildKubernetesConfig(controlPlaneNodes []*node.Node) files.KubernetesConfig {
	if len(controlPlaneNodes) == 0 {
		return files.KubernetesConfig{
			ControlPlanes: []files.K8sNode{},
			Workers:       []files.K8sNode{},
		}
	}

	k8sConfig := files.KubernetesConfig{
		ManagedByCodesphere: true,
		APIServerHost:       controlPlaneNodes[0].GetInternalIP(),
		ControlPlanes: []files.K8sNode{
			{
				IPAddress: controlPlaneNodes[0].GetInternalIP(),
			},
		},
	}

	for _, workerNode := range controlPlaneNodes {
		k8sConfig.Workers = append(k8sConfig.Workers, files.K8sNode{
			IPAddress: workerNode.GetInternalIP(),
		})
	}

	return k8sConfig
}

// regeneratePostgresCerts regenerates PostgreSQL TLS certificates when the IP/hostname
// changed or no private key was loaded from the vault.
func (b *GCPBootstrapper) regeneratePostgresCerts(previousPrimaryIP, previousPrimaryHostname string) error {
	if b.Env.InstallConfig.Postgres.Primary == nil {
		return fmt.Errorf("primary postgres config is not set")
	}

	// Only regenerate postgres certificates if the IP or hostname changed,
	// or if the private key was not loaded from the vault.
	primaryNeedsRegen := b.Env.InstallConfig.Postgres.Primary.PrivateKey == "" ||
		previousPrimaryIP != b.Env.InstallConfig.Postgres.Primary.IP ||
		previousPrimaryHostname != b.Env.InstallConfig.Postgres.Primary.Hostname

	if primaryNeedsRegen {
		var err error
		b.Env.InstallConfig.Postgres.Primary.PrivateKey, b.Env.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem, err = installer.GenerateServerCertificate(
			b.Env.InstallConfig.Postgres.CaCertPrivateKey,
			b.Env.InstallConfig.Postgres.CACertPem,
			b.Env.InstallConfig.Postgres.Primary.Hostname,
			[]string{b.Env.InstallConfig.Postgres.Primary.IP})
		if err != nil {
			return fmt.Errorf("failed to generate primary server certificate: %w", err)
		}
		if err := installer.ValidateCertKeyPair(b.Env.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem, b.Env.InstallConfig.Postgres.Primary.PrivateKey); err != nil {
			return fmt.Errorf("primary PostgreSQL cert/key validation failed: %w", err)
		}
	}
	// Replica certificates only regenerate if the key is missing from the vault.
	if b.Env.InstallConfig.Postgres.Replica != nil && (b.Env.InstallConfig.Postgres.ReplicaPrivateKey == "") {
		var err error
		b.Env.InstallConfig.Postgres.ReplicaPrivateKey, b.Env.InstallConfig.Postgres.Replica.SSLConfig.ServerCertPem, err = installer.GenerateServerCertificate(
			b.Env.InstallConfig.Postgres.CaCertPrivateKey,
			b.Env.InstallConfig.Postgres.CACertPem,
			b.Env.InstallConfig.Postgres.Replica.Name,
			[]string{b.Env.InstallConfig.Postgres.Replica.IP})
		if err != nil {
			return fmt.Errorf("failed to generate replica server certificate: %w", err)
		}
		if err := installer.ValidateCertKeyPair(b.Env.InstallConfig.Postgres.Replica.SSLConfig.ServerCertPem, b.Env.InstallConfig.Postgres.ReplicaPrivateKey); err != nil {
			return fmt.Errorf("replica PostgreSQL cert/key validation failed: %w", err)
		}
	}
	return nil
}

func (b *GCPBootstrapper) EnsureAgeKey() error {
	hasKey := b.Env.Jumpbox.NodeClient.HasFile(b.Env.Jumpbox, b.Env.SecretsDir+"/age_key.txt")
	if hasKey {
		return nil
	}

	err := b.Env.Jumpbox.RunSSHCommand("root", fmt.Sprintf("mkdir -p %s; age-keygen -o %s/age_key.txt", b.Env.SecretsDir, b.Env.SecretsDir))
	if err != nil {
		return fmt.Errorf("failed to generate age key on jumpbox: %w", err)
	}

	return nil
}

func (b *GCPBootstrapper) EnsureSecrets() error {
	if b.fw.Exists(b.Env.SecretsFilePath) {
		err := b.icg.LoadVaultFromFile(b.Env.SecretsFilePath)
		if err != nil {
			return fmt.Errorf("failed to load vault file: %w", err)
		}
		err = b.icg.MergeVaultIntoConfig()
		if err != nil {
			return fmt.Errorf("failed to merge vault into config: %w", err)
		}
	}

	b.Env.Secrets = b.icg.GetVault()

	return nil
}

func (b *GCPBootstrapper) EncryptVault() error {
	err := b.Env.Jumpbox.RunSSHCommand("root", "cp "+b.Env.SecretsDir+"/prod.vault.yaml{,.bak}")
	if err != nil {
		return fmt.Errorf("failed backup vault on jumpbox: %w", err)
	}

	err = b.Env.Jumpbox.RunSSHCommand("root", "sops --encrypt --in-place --age $(age-keygen -y "+b.Env.SecretsDir+"/age_key.txt) "+b.Env.SecretsDir+"/prod.vault.yaml")
	if err != nil {
		return fmt.Errorf("failed to encrypt vault on jumpbox: %w", err)
	}

	return nil
}

// decryptVault creates an unencrypted copy of the vault in dst on the jumpbox
// Make sure to delete the unencrypted file when not needed anymore.
func (b *GCPBootstrapper) decryptVault(dst string) error {
	err := b.Env.Jumpbox.RunSSHCommand("root", "cp "+b.Env.SecretsDir+"/prod.vault.yaml "+dst)
	if err != nil {
		return fmt.Errorf("failed to create tmp vault on jumpbox: %w", err)
	}

	err = b.Env.Jumpbox.NodeClient.RunCommand(b.Env.Jumpbox, "root", "chmod 600 "+dst)
	if err != nil {
		return fmt.Errorf("failed to make vault file readable only for root on jumpbox: %w", err)
	}

	err = b.Env.Jumpbox.RunSSHCommand("root", "SOPS_AGE_KEY_FILE="+b.Env.SecretsDir+"/age_key.txt sops --decrypt --in-place "+dst)
	if err != nil {
		return fmt.Errorf("failed to decrypt vault on jumpbox: %w", err)
	}

	return nil
}
