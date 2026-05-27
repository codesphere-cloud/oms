// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
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

func (b *GCPBootstrapper) UpdateInstallConfig() error {
	// Update install config with necessary values
	b.Env.InstallConfig.Datacenter.ID = b.Env.DatacenterID
	if b.Env.DatacenterName == "" {
		b.Env.DatacenterName = "dev"
	}
	b.Env.InstallConfig.Datacenter.Name = b.Env.DatacenterName
	b.Env.InstallConfig.Datacenter.City = "Karlsruhe"
	b.Env.InstallConfig.Datacenter.CountryCode = "DE"
	b.Env.InstallConfig.Secrets.BaseDir = b.Env.SecretsDir
	if b.Env.RegistryType != RegistryTypeGitHub {
		b.Env.InstallConfig.Registry.ReplaceImagesInBom = true
		b.Env.InstallConfig.Registry.LoadContainerImages = true
	}

	if b.Env.InstallConfig.Postgres.Primary == nil {
		b.Env.InstallConfig.Postgres.Primary = &files.PostgresPrimaryConfig{
			Hostname: b.Env.PostgreSQLNode.GetName(),
		}
	}

	previousPrimaryIP := b.Env.InstallConfig.Postgres.Primary.IP
	previousPrimaryHostname := b.Env.InstallConfig.Postgres.Primary.Hostname
	b.Env.InstallConfig.Postgres.Primary.IP = b.Env.PostgreSQLNode.GetInternalIP()
	b.Env.InstallConfig.Postgres.Primary.Hostname = b.Env.PostgreSQLNode.GetName()

	b.Env.InstallConfig.Ceph.CsiKubeletDir = "/var/lib/k0s/kubelet"
	b.Env.InstallConfig.Ceph.NodesSubnet = "10.10.0.0/20"
	b.Env.InstallConfig.Ceph.Hosts = []files.CephHost{
		{
			Hostname:  b.Env.CephNodes[0].GetName(),
			IsMaster:  true,
			IPAddress: b.Env.CephNodes[0].GetInternalIP(),
		},
		{
			Hostname:  b.Env.CephNodes[1].GetName(),
			IPAddress: b.Env.CephNodes[1].GetInternalIP(),
		},
		{
			Hostname:  b.Env.CephNodes[2].GetName(),
			IPAddress: b.Env.CephNodes[2].GetInternalIP(),
		},
	}
	b.Env.InstallConfig.Ceph.OSDs = []files.CephOSD{
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

	b.Env.InstallConfig.Kubernetes = files.KubernetesConfig{
		ManagedByCodesphere: true,
		APIServerHost:       b.Env.ControlPlaneNodes[0].GetInternalIP(),
		ControlPlanes: []files.K8sNode{
			{
				IPAddress: b.Env.ControlPlaneNodes[0].GetInternalIP(),
			},
		},
		Workers: []files.K8sNode{
			{
				IPAddress: b.Env.ControlPlaneNodes[0].GetInternalIP(),
			},

			{
				IPAddress: b.Env.ControlPlaneNodes[1].GetInternalIP(),
			},
			{
				IPAddress: b.Env.ControlPlaneNodes[2].GetInternalIP(),
			},
		},
	}

	b.Env.InstallConfig.Cluster.Kyverno = &files.KyvernoConfig{
		Enabled: false,
	}

	b.Env.InstallConfig.Cluster.Gateway.ServiceType = "LoadBalancer"
	b.Env.InstallConfig.Cluster.Gateway.Annotations = map[string]string{
		"cloud.google.com/load-balancer-ipv4": b.Env.GatewayIP,
	}
	b.Env.InstallConfig.Cluster.PublicGateway.ServiceType = "LoadBalancer"
	b.Env.InstallConfig.Cluster.PublicGateway.Annotations = map[string]string{
		"cloud.google.com/load-balancer-ipv4": b.Env.PublicGatewayIP,
	}

	dnsProject := b.Env.DNSProjectID
	if b.Env.DNSProjectID == "" {
		dnsProject = b.Env.ProjectID
	}
	b.Env.InstallConfig.Cluster.Certificates.Override = map[string]interface{}{
		"issuers": map[string]interface{}{
			"letsEncryptHttp": map[string]interface{}{
				"enabled": !b.Env.GoogleACMEIssuer,
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
	acmeConfig := &files.ACMEConfig{
		Enabled: true,
		Email:   "oms-testing@" + b.Env.BaseDomain,
		Server:  "https://acme-v02.api.letsencrypt.org/directory",
	}
	if b.Env.GoogleACMEIssuer {
		keyID, b64MacKey, err := b.GCPClient.CreatePublicCAExternalAccountKey(b.Env.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to obtain Google Public CA EAB credentials: %w", err)
		}
		acmeConfig.Server = "https://dv.acme-v02.api.pki.goog/directory"
		acmeConfig.EABKeyID = keyID
		acmeConfig.EABMacKey = b64MacKey
	}
	b.Env.InstallConfig.Codesphere.CertIssuer = files.CertIssuerConfig{
		Type: "acme",
		Acme: acmeConfig,
	}

	b.Env.InstallConfig.Codesphere.Domain = "cs." + b.Env.BaseDomain
	b.Env.InstallConfig.Codesphere.WorkspaceHostingBaseDomain = "ws." + b.Env.BaseDomain
	b.Env.InstallConfig.Codesphere.PublicIP = b.Env.ControlPlaneNodes[1].GetExternalIP()
	b.Env.InstallConfig.Codesphere.CustomDomains = files.CustomDomainsConfig{
		CNameBaseDomain: "ws." + b.Env.BaseDomain,
	}
	b.Env.InstallConfig.Codesphere.DNSServers = []string{"8.8.8.8"}
	b.Env.InstallConfig.Codesphere.DeployConfig = bootstrap.DefaultCodesphereDeployConfig()
	b.Env.InstallConfig.Codesphere.Plans = bootstrap.DefaultCodespherePlans()

	b.Env.InstallConfig.Codesphere.GitProviders = &files.GitProvidersConfig{}
	if b.Env.GitHubAppName != "" && b.Env.GitHubAppClientID != "" && b.Env.GitHubAppClientSecret != "" {
		b.Env.InstallConfig.Codesphere.GitProviders.GitHub = &files.GitProviderConfig{
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
	if b.Env.GitLabAppClientID != "" && b.Env.GitLabAppClientSecret != "" {
		b.Env.InstallConfig.Codesphere.GitProviders.GitLab = &files.GitProviderConfig{
			Enabled: true,
			URL:     "https://gitlab.com",
			API: files.APIConfig{
				BaseURL: "https://gitlab.com",
			},
			OAuth: files.OAuthConfig{
				Issuer:                "https://gitlab.com",
				AuthorizationEndpoint: "https://gitlab.com/oauth/authorize",
				TokenEndpoint:         "https://gitlab.com/oauth/token",
				ClientAuthMethod:      "client_secret_post",
				RedirectURI:           "https://cs." + b.Env.BaseDomain + "/ide/auth/gitlab/callback",
				ClientID:              b.Env.GitLabAppClientID,
				ClientSecret:          b.Env.GitLabAppClientSecret,
			},
		}
	}
	if b.Env.BitbucketAppClientID != "" && b.Env.BitbucketAppClientSecret != "" {
		b.Env.InstallConfig.Codesphere.GitProviders.Bitbucket = &files.GitProviderConfig{
			Enabled: true,
			URL:     "https://bitbucket.org",
			API: files.APIConfig{
				BaseURL: "https://api.bitbucket.org/2.0",
			},
			OAuth: files.OAuthConfig{
				Issuer:                "https://bitbucket.org",
				AuthorizationEndpoint: "https://bitbucket.org/site/oauth2/authorize",
				TokenEndpoint:         "https://bitbucket.org/site/oauth2/access_token",
				ClientAuthMethod:      "client_secret_post",
				RedirectURI:           "https://cs." + b.Env.BaseDomain + "/ide/auth/bitbucket/callback",
				ClientID:              b.Env.BitbucketAppClientID,
				ClientSecret:          b.Env.BitbucketAppClientSecret,
			},
		}
	}
	if b.Env.AzureDevOpsAppClientID != "" && b.Env.AzureDevOpsAppClientSecret != "" {
		b.Env.InstallConfig.Codesphere.GitProviders.AzureDevOps = &files.GitProviderConfig{
			Enabled: true,
			URL:     "https://dev.azure.com",
			API: files.APIConfig{
				BaseURL: "https://dev.azure.com",
			},
			OAuth: files.OAuthConfig{
				Issuer:                "https://login.microsoftonline.com/common/v2.0",
				AuthorizationEndpoint: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
				TokenEndpoint:         "https://login.microsoftonline.com/common/oauth2/v2.0/token",
				ClientAuthMethod:      "client_secret_post",
				RedirectURI:           "https://cs." + b.Env.BaseDomain + "/ide/auth/azure-dev-ops/callback",
				Scope:                 "openid offline_access https://app.vssps.visualstudio.com/vso.code_full",
				ClientID:              b.Env.AzureDevOpsAppClientID,
				ClientSecret:          b.Env.AzureDevOpsAppClientSecret,
			},
		}
	}
	if b.Env.OidcIssuerURL != "" && b.Env.OidcClientID != "" && b.Env.OidcClientSecret != "" {
		name := b.Env.OidcProviderName
		if name == "" {
			name = "OIDC"
		}
		b.Env.InstallConfig.Codesphere.OAuth = &files.OAuthProvidersConfig{
			Oidc: &files.OidcOAuthProvider{
				Type:         "oidc",
				Enabled:      true,
				Name:         name,
				IssuerURL:    b.Env.OidcIssuerURL,
				Scopes:       []string{"openid", "profile", "email"},
				ClientID:     b.Env.OidcClientID,
				ClientSecret: b.Env.OidcClientSecret,
			},
		}
	}

	if b.Env.CentralOtelPassword != "" || b.Env.LocalTraceEndpoint != "" {
		b.Env.InstallConfig.Codesphere.TelemetryExport = &files.TelemetryExport{
			RemoteEndpoint: b.Env.CentralOtelEndpoint,
			RemoteExport:   b.Env.CentralOtelPassword != "",
			Traces:         b.Env.LocalTraceEndpoint != "",
			TraceEndpoint:  b.Env.LocalTraceEndpoint,
			SpanMetrics:    b.Env.CentralOtelSpanMetrics,
		}
	}

	b.Env.InstallConfig.Codesphere.Experiments = b.Env.Experiments
	b.Env.InstallConfig.Codesphere.Features = b.Env.FeatureFlags
	b.applyExternalLokiConfig()
	b.applyPrometheusRemoteWriteConfig()

	if !b.Env.ExistingConfigUsed {
		err := b.icg.GenerateSecrets()
		if err != nil {
			return fmt.Errorf("failed to generate secrets: %w", err)
		}
	} else {
		if err := b.regeneratePostgresCerts(previousPrimaryIP, previousPrimaryHostname); err != nil {
			return err
		}
	}

	if b.Env.CentralOtelUsername != "" && b.Env.CentralOtelPassword != "" {
		if b.Env.InstallConfig.Cluster.Monitoring == nil {
			b.Env.InstallConfig.Cluster.Monitoring = &files.MonitoringConfig{}
		}
		b.Env.InstallConfig.Cluster.Monitoring.CentralOtelExport = &files.CentralOtelConfig{
			Enabled:  true,
			Username: b.Env.CentralOtelUsername,
			Password: b.Env.CentralOtelPassword,
		}
	}

	if b.Env.OpenBaoURI != "" {
		b.Env.InstallConfig.Codesphere.OpenBao = &files.OpenBaoConfig{
			Engine:   b.Env.OpenBaoEngine,
			URI:      b.Env.OpenBaoURI,
			User:     b.Env.OpenBaoUser,
			Password: b.Env.OpenBaoPassword,
		}
	}

	if err := b.icg.WriteInstallConfig(b.Env.InstallConfigPath, true); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if err := b.icg.WriteVault(b.Env.SecretsFilePath, true); err != nil {
		return fmt.Errorf("failed to write vault file: %w", err)
	}

	err := b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, b.Env.InstallConfigPath, remoteInstallConfigPath)
	if err != nil {
		return fmt.Errorf("failed to copy install config to jumpbox: %w", err)
	}

	err = b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, b.Env.SecretsFilePath, b.Env.SecretsDir+"/prod.vault.yaml")
	if err != nil {
		return fmt.Errorf("failed to copy secrets file to jumpbox: %w", err)
	}

	return nil
}

func (b *GCPBootstrapper) applyExternalLokiConfig() {
	if b.Env.ExternalLokiEndpoint == "" {
		return
	}

	if b.Env.InstallConfig.Cluster.Monitoring == nil {
		b.Env.InstallConfig.Cluster.Monitoring = &files.MonitoringConfig{}
	}
	if b.Env.InstallConfig.Cluster.Monitoring.GrafanaAlloy == nil {
		b.Env.InstallConfig.Cluster.Monitoring.GrafanaAlloy = &files.GrafanaAlloyConfig{}
	}

	loki := &files.LokiConnectionConfig{
		Endpoint: b.Env.ExternalLokiEndpoint,
		User:     b.Env.ExternalLokiUser,
		Password: b.Env.ExternalLokiSecret,
	}

	b.Env.InstallConfig.Cluster.Monitoring.GrafanaAlloy.Enabled = true
	b.Env.InstallConfig.Cluster.Monitoring.GrafanaAlloy.Loki = loki
}

func (b *GCPBootstrapper) applyPrometheusRemoteWriteConfig() {
	if b.Env.PrometheusRemoteWriteURL == "" {
		return
	}

	if b.Env.InstallConfig.Cluster.Monitoring == nil {
		b.Env.InstallConfig.Cluster.Monitoring = &files.MonitoringConfig{}
	}
	if b.Env.InstallConfig.Cluster.Monitoring.Prometheus == nil {
		b.Env.InstallConfig.Cluster.Monitoring.Prometheus = &files.PrometheusConfig{}
	}
	if b.Env.InstallConfig.Cluster.Monitoring.Prometheus.RemoteWrite == nil {
		b.Env.InstallConfig.Cluster.Monitoring.Prometheus.RemoteWrite = &files.RemoteWriteConfig{}
	}

	b.Env.InstallConfig.Cluster.Monitoring.Prometheus.RemoteWrite.Enabled = true
	b.Env.InstallConfig.Cluster.Monitoring.Prometheus.RemoteWrite.Url = b.Env.PrometheusRemoteWriteURL
	b.Env.InstallConfig.Cluster.Monitoring.Prometheus.RemoteWrite.ClusterName = b.Env.ProjectName
	b.Env.InstallConfig.Cluster.Monitoring.Prometheus.RemoteWrite.Username = b.Env.PrometheusRemoteWriteUser
	b.Env.InstallConfig.Cluster.Monitoring.Prometheus.RemoteWrite.Password = b.Env.PrometheusRemoteWritePassword
}

// regeneratePostgresCerts regenerates PostgreSQL TLS certificates when the IP/hostname
// changed or no private key was loaded from the vault.
func (b *GCPBootstrapper) regeneratePostgresCerts(previousPrimaryIP, previousPrimaryHostname string) error {
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
