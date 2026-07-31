// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/bootstrap/datacenter"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	"github.com/codesphere-cloud/oms/internal/util"
)

const (
	remoteInstallConfigPath string = "/etc/codesphere/config.yaml"
)

// EnsureInstallConfig prepares the primary data center's install config.
func (b *GCPBootstrapper) EnsureInstallConfig() error {
	b.ensureDataCenters()

	return b.ensureInstallConfig(b.primaryDC())
}

// ensureInstallConfig uses the data center's local config or recovers it from an existing
// jumpbox if desired. Else it applies the minimal profile to a new config.
func (b *GCPBootstrapper) ensureInstallConfig(dc *datacenter.DataCenter) error {
	// recovery will overwrite local config or create a new file
	if b.Env.RecoverConfig {
		err := b.recoverConfig(dc)
		if err != nil {
			return fmt.Errorf("failed to recover config: %w", err)
		}
	}

	if b.fw.Exists(dc.InstallConfigPath) {
		if err := b.loadVaultForConfigTemplating(dc); err != nil {
			return fmt.Errorf("failed to load vault templating: %w", err)
		}

		err := dc.ConfigManager.LoadInstallConfigFromFile(dc.InstallConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}

		dc.ExistingConfigUsed = true
	} else {
		err := dc.ConfigManager.ApplyProfile("minimal")
		if err != nil {
			return fmt.Errorf("failed to apply profile: %w", err)
		}
	}

	dc.InstallConfig = dc.ConfigManager.GetInstallConfig()

	b.mirrorPrimaryDataCenter()

	return nil
}

func (b *GCPBootstrapper) loadVaultForConfigTemplating(dc *datacenter.DataCenter) error {
	if !b.fw.Exists(dc.SecretsFilePath) {
		return nil
	}

	// during bootstrapping, the vault is not yet encrpyted
	if err := dc.ConfigManager.LoadVaultFromUnecryptedFile(dc.SecretsFilePath); err != nil {
		return fmt.Errorf("failed to load vault from file: %w", err)
	}

	return nil
}

// recoverConfig downloads the config and secrets from the jumpbox if it exists.
// Since recovery is done when the project or VMs are not ensured, we need to search for the jumpbox IP first.
// Returns an error if project or jumpbox does not exist or downloading fails.
func (b *GCPBootstrapper) recoverConfig(dc *datacenter.DataCenter) error {
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

	err = b.Env.Jumpbox.NodeClient.DownloadFile(jumpbox, dc.RemoteConfigPath, dc.InstallConfigPath)
	if err != nil {
		return fmt.Errorf("failed to download install config from jumpbox: %w", err)
	}

	err = b.recoverVault(dc)
	if err != nil {
		return fmt.Errorf("failed to recover vault: %w", err)
	}

	return nil
}

// recoverVault unencrypts the secrets file on the jumpbox and download the file to the local destination
func (b *GCPBootstrapper) recoverVault(dc *datacenter.DataCenter) error {
	vaultCopyPath := fmt.Sprintf("/tmp/prod%s.vault.yaml", dc.Suffix)
	defer func() {
		err := b.Env.Jumpbox.RunSSHCommand("root", "rm -f "+vaultCopyPath)
		if err != nil {
			b.stlog.Logf("failed to remove unencrypted vault file for recovery: %s", err.Error())
		}
	}()

	err := b.decryptVault(dc, vaultCopyPath)
	if err != nil {
		return fmt.Errorf("failed to create decrypted vault for recovery: %w", err)
	}

	err = b.Env.Jumpbox.NodeClient.DownloadFile(b.Env.Jumpbox, vaultCopyPath, dc.SecretsFilePath)
	if err != nil {
		return fmt.Errorf("failed to download secrets file from jumpbox: %w", err)
	}

	return nil
}

// UpdateInstallConfig writes the bootstrapped infrastructure into the primary data center's
// install config.
func (b *GCPBootstrapper) UpdateInstallConfig() error {
	b.ensureDataCenters()

	return b.updateInstallConfig(b.primaryDC())
}

func (b *GCPBootstrapper) updateInstallConfig(dc *datacenter.DataCenter) error {
	// Update install config with necessary values
	dc.InstallConfig.Datacenter.ID = dc.ID
	dc.InstallConfig.Datacenter.Name = dc.Name
	dc.InstallConfig.Datacenter.City = "Karlsruhe"
	dc.InstallConfig.Datacenter.CountryCode = "DE"
	// Each data center reads and writes its own vault. The installer resolves the vault from
	// secrets.baseDir, so sharing a directory would let one data center's ceph and kubernetes
	// steps overwrite another's credentials.
	dc.InstallConfig.Secrets.BaseDir = dc.SecretsDir
	if b.Env.RegistryType != RegistryTypeGitHub {
		dc.InstallConfig.Registry.ReplaceImagesInBom = true
		dc.InstallConfig.Registry.LoadContainerImages = true
	}

	if dc.InstallConfig.Postgres.Primary == nil {
		dc.InstallConfig.Postgres.Primary = &files.PostgresPrimaryConfig{
			Hostname: b.Env.PostgreSQLNode.GetName(),
		}
	}

	previousPrimaryIP := dc.InstallConfig.Postgres.Primary.IP
	previousPrimaryHostname := dc.InstallConfig.Postgres.Primary.Hostname
	dc.InstallConfig.Postgres.Primary.IP = b.Env.PostgreSQLNode.GetInternalIP()
	dc.InstallConfig.Postgres.Primary.Hostname = b.Env.PostgreSQLNode.GetName()

	dc.InstallConfig.Ceph.CsiKubeletDir = "/var/lib/k0s/kubelet"
	// All data centers share the project's subnet; their Ceph clusters stay separate because
	// each has its own hosts, monitors and FSID.
	dc.InstallConfig.Ceph.NodesSubnet = vpcSubnetCIDR
	dc.InstallConfig.Ceph.Hosts = []files.CephHost{
		{
			Hostname:  dc.CephNodes[0].GetName(),
			IsMaster:  true,
			IPAddress: dc.CephNodes[0].GetInternalIP(),
		},
		{
			Hostname:  dc.CephNodes[1].GetName(),
			IPAddress: dc.CephNodes[1].GetInternalIP(),
		},
		{
			Hostname:  dc.CephNodes[2].GetName(),
			IPAddress: dc.CephNodes[2].GetInternalIP(),
		},
	}
	dc.InstallConfig.Ceph.OSDs = []files.CephOSD{
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

	dc.InstallConfig.Kubernetes = files.KubernetesConfig{
		ManagedByCodesphere: true,
		APIServerHost:       dc.ControlPlaneNodes[0].GetInternalIP(),
		ControlPlanes: []files.K8sNode{
			{
				IPAddress: dc.ControlPlaneNodes[0].GetInternalIP(),
			},
		},
		Workers: []files.K8sNode{
			{
				IPAddress: dc.ControlPlaneNodes[0].GetInternalIP(),
			},

			{
				IPAddress: dc.ControlPlaneNodes[1].GetInternalIP(),
			},
			{
				IPAddress: dc.ControlPlaneNodes[2].GetInternalIP(),
			},
		},
	}

	dc.InstallConfig.Cluster.Kyverno = &files.KyvernoConfig{
		Enabled: false,
	}

	dc.InstallConfig.Cluster.Gateway.ServiceType = "LoadBalancer"
	dc.InstallConfig.Cluster.Gateway.Annotations = map[string]string{
		"cloud.google.com/load-balancer-ipv4": dc.GatewayIP,
	}
	dc.InstallConfig.Cluster.PublicGateway.ServiceType = "LoadBalancer"
	dc.InstallConfig.Cluster.PublicGateway.Annotations = map[string]string{
		"cloud.google.com/load-balancer-ipv4": dc.PublicGatewayIP,
	}

	b.applySSHProxyConfig(dc)

	dnsProject := b.Env.DNSProjectID
	if b.Env.DNSProjectID == "" {
		dnsProject = b.Env.ProjectID
	}

	dc.InstallConfig.Cluster.Certificates.Override = map[string]interface{}{
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
	acmeServer := "https://acme-v02.api.letsencrypt.org/directory"
	if b.Env.ACMEStaging {
		acmeServer = "https://acme-staging-v02.api.letsencrypt.org/directory"
	}
	acmeConfig := &files.ACMEConfig{
		Enabled: true,
		Email:   "oms-testing@" + b.Env.BaseDomain,
		Server:  acmeServer,
	}
	if b.Env.GoogleACMEIssuer {
		keyID, b64MacKey, err := b.GCPClient.CreatePublicCAExternalAccountKey(b.Env.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to obtain Google Public CA EAB credentials: %w", err)
		}
		acmeConfig.Server = "https://dv.acme-v02.api.pki.goog/directory"
		acmeConfig.EABKeyID = keyID

		dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: files.SecretAcmeEabMacKey, Fields: &files.SecretFields{Password: b64MacKey}})
	}

	dc.InstallConfig.Codesphere.CertIssuer = &files.CertIssuerConfig{
		Type: "acme",
		Acme: acmeConfig,
	}

	// The platform is served from one domain shared by all data centers, while workspaces and
	// custom domains resolve to the data center hosting them.
	dc.InstallConfig.Codesphere.Domain = "cs." + b.Env.BaseDomain
	dc.InstallConfig.Codesphere.WorkspaceHostingBaseDomain = dc.WorkspaceHostingBaseDomain
	dc.InstallConfig.Codesphere.CustomDomains = files.CustomDomainsConfig{
		CNameBaseDomain: dc.WorkspaceHostingBaseDomain,
	}
	dc.InstallConfig.Codesphere.PublicIP = dc.ControlPlaneNodes[1].GetExternalIP()
	dc.InstallConfig.Codesphere.DNSServers = []string{"8.8.8.8"}
	dc.InstallConfig.Codesphere.DeployConfig = bootstrap.DefaultCodesphereDeployConfig()
	dc.InstallConfig.Codesphere.Plans = bootstrap.DefaultCodespherePlans()

	dc.InstallConfig.Codesphere.GitProviders = &files.GitProvidersConfig{}
	if b.Env.GitHubAppName != "" && b.Env.GitHubAppClientID != "" && b.Env.GitHubAppClientSecret != "" {
		dc.InstallConfig.Codesphere.GitProviders.GitHub = &files.GitProviderConfig{
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
			},
		}
		dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: files.SecretGithubAppsClientId, Fields: &files.SecretFields{Password: b.Env.GitHubAppClientID}})
		dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: files.SecretGithubAppsClientSecret, Fields: &files.SecretFields{Password: b.Env.GitHubAppClientSecret}})
	}
	if b.Env.GitLabAppClientID != "" && b.Env.GitLabAppClientSecret != "" {
		dc.InstallConfig.Codesphere.GitProviders.GitLab = &files.GitProviderConfig{
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
			},
		}
		dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: files.SecretGitlabAppClientId, Fields: &files.SecretFields{Password: b.Env.GitLabAppClientID}})
		dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: files.SecretGitlabAppClientSecret, Fields: &files.SecretFields{Password: b.Env.GitLabAppClientSecret}})
	}
	if b.Env.BitbucketAppClientID != "" && b.Env.BitbucketAppClientSecret != "" {
		dc.InstallConfig.Codesphere.GitProviders.Bitbucket = &files.GitProviderConfig{
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
			},
		}
		dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: files.SecretBitbucketAppsClientId, Fields: &files.SecretFields{Password: b.Env.BitbucketAppClientID}})
		dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: files.SecretBitbucketAppsClientSecret, Fields: &files.SecretFields{Password: b.Env.BitbucketAppClientSecret}})
	}
	if b.Env.AzureDevOpsAppClientID != "" && b.Env.AzureDevOpsAppClientSecret != "" {
		dc.InstallConfig.Codesphere.GitProviders.AzureDevOps = &files.GitProviderConfig{
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
			},
		}
		dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: files.SecretAzureDevOpsAppClientId, Fields: &files.SecretFields{Password: b.Env.AzureDevOpsAppClientID}})
		dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: files.SecretAzureDevOpsAppClientSecret, Fields: &files.SecretFields{Password: b.Env.AzureDevOpsAppClientSecret}})
	}
	if b.Env.OidcIssuerURL != "" && b.Env.OidcClientID != "" && b.Env.OidcClientSecret != "" {
		name := b.Env.OidcProviderName
		if name == "" {
			name = "OIDC"
		}

		dc.InstallConfig.Codesphere.OAuth = &files.OAuthProvidersConfig{
			Oidc: &files.OidcOAuthProvider{
				Type:      "oidc",
				Enabled:   true,
				Name:      name,
				IssuerURL: b.Env.OidcIssuerURL,
				Scopes:    []string{"openid", "profile", "email"},
			},
		}
		dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: files.SecretOidcClientId, Fields: &files.SecretFields{Password: b.Env.OidcClientID}})
		dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: files.SecretOidcClientSecret, Fields: &files.SecretFields{Password: b.Env.OidcClientSecret}})
	}

	if b.Env.CentralOtelPassword != "" || b.Env.LocalTraceEndpoint != "" {
		dc.InstallConfig.Codesphere.TelemetryExport = &files.TelemetryExport{
			RemoteEndpoint: b.Env.CentralOtelEndpoint,
			RemoteExport:   b.Env.CentralOtelPassword != "",
			Traces:         b.Env.LocalTraceEndpoint != "",
			TraceEndpoint:  b.Env.LocalTraceEndpoint,
			SpanMetrics:    b.Env.CentralOtelSpanMetrics,
		}
	}

	dc.InstallConfig.Codesphere.Internal = b.Env.InternalFlags
	dc.InstallConfig.Codesphere.Preview = util.StringSliceToBoolMap(b.Env.PreviewFlags)
	dc.InstallConfig.Codesphere.Features = util.StringSliceToBoolMap(b.Env.FeatureFlags)
	// Only set when the flag is provided so a recovered config keeps its value on re-runs.
	if b.Env.ClusterAdminEmail != "" {
		dc.InstallConfig.Codesphere.ClusterAdminEmail = b.Env.ClusterAdminEmail
	}

	b.applyExternalLokiConfig(dc)
	b.applyPrometheusRemoteWriteConfig(dc)

	// Secret generation is idempotent and also backfills secrets introduced
	// after an existing vault was created (for example the auth keys required by
	// the ArgoCD pre-step).
	if err := dc.ConfigManager.GenerateSecrets(); err != nil {
		return fmt.Errorf("failed to generate secrets: %w", err)
	}

	if dc.ExistingConfigUsed {
		if err := b.regeneratePostgresCerts(dc, previousPrimaryIP, previousPrimaryHostname); err != nil {
			return err
		}
	}

	if b.Env.CentralOtelUsername != "" && b.Env.CentralOtelPassword != "" {
		if dc.InstallConfig.Cluster.Monitoring == nil {
			dc.InstallConfig.Cluster.Monitoring = &files.MonitoringConfig{}
		}

		dc.InstallConfig.Cluster.Monitoring.CentralOtelExport = &files.CentralOtelConfig{
			Enabled:  true,
			Username: b.Env.CentralOtelUsername,
			Password: b.Env.CentralOtelPassword,
		}
		dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: files.SecretCentralOtelCreds, Fields: &files.SecretFields{Username: b.Env.CentralOtelUsername, Password: b.Env.CentralOtelPassword}})
	}

	if b.Env.OpenBaoURI != "" {
		dc.InstallConfig.Codesphere.OpenBao = &files.OpenBaoConfig{
			Engine: b.Env.OpenBaoEngine,
			URI:    b.Env.OpenBaoURI,
			User:   b.Env.OpenBaoUser,
		}
		dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: files.SecretOpenBaoPassword, Fields: &files.SecretFields{Password: b.Env.OpenBaoPassword}})
	}

	if err := dc.ConfigManager.WriteInstallConfig(dc.InstallConfigPath, true); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if err := dc.ConfigManager.WriteUnencryptedVault(dc.SecretsFilePath, true); err != nil {
		return fmt.Errorf("failed to write vault file: %w", err)
	}

	// CopyFile creates the destination directory, so a secondary data center's secrets
	// directory does not need to exist yet.
	err := b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, dc.InstallConfigPath, dc.RemoteConfigPath)
	if err != nil {
		return fmt.Errorf("failed to copy install config to jumpbox: %w", err)
	}

	err = b.Env.Jumpbox.NodeClient.CopyFile(b.Env.Jumpbox, dc.SecretsFilePath, dc.RemoteVaultPath())
	if err != nil {
		return fmt.Errorf("failed to copy secrets file to jumpbox: %w", err)
	}

	b.mirrorPrimaryDataCenter()

	return nil
}

func (b *GCPBootstrapper) applySSHProxyConfig(dc *datacenter.DataCenter) {
	dc.InstallConfig.PcApps = util.DeepMergeMaps(dc.InstallConfig.PcApps, files.ChartValues{
		"applications": map[string]any{
			"ssh-workspace-proxy": map[string]any{
				"enabled": true,
				"valuesObject": map[string]any{
					"service": map[string]any{
						"enabled":        true,
						"type":           "LoadBalancer",
						"loadBalancerIP": dc.SSHProxyIP,
						"annotations": map[string]any{
							"cloud.google.com/load-balancer-ipv4": dc.SSHProxyIP,
						},
					},
				},
			},
		},
	})
}

func (b *GCPBootstrapper) applyExternalLokiConfig(dc *datacenter.DataCenter) {
	if b.Env.ExternalLokiEndpoint == "" {
		return
	}

	if dc.InstallConfig.Cluster.Monitoring == nil {
		dc.InstallConfig.Cluster.Monitoring = &files.MonitoringConfig{}
	}

	if dc.InstallConfig.Cluster.Monitoring.GrafanaAlloy == nil {
		dc.InstallConfig.Cluster.Monitoring.GrafanaAlloy = &files.GrafanaAlloyConfig{}
	}

	loki := &files.LokiConnectionConfig{
		Endpoint: b.Env.ExternalLokiEndpoint,
		User:     b.Env.ExternalLokiUser,
		Password: b.Env.ExternalLokiSecret,
	}

	dc.InstallConfig.Cluster.Monitoring.GrafanaAlloy.Enabled = true
	dc.InstallConfig.Cluster.Monitoring.GrafanaAlloy.Loki = loki
	dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: files.SecretLokiGatewayBasicAuthPassword, Fields: &files.SecretFields{Password: b.Env.ExternalLokiSecret}})
}

func (b *GCPBootstrapper) applyPrometheusRemoteWriteConfig(dc *datacenter.DataCenter) {
	if b.Env.PrometheusRemoteWriteURL == "" {
		return
	}

	if dc.InstallConfig.Cluster.Monitoring == nil {
		dc.InstallConfig.Cluster.Monitoring = &files.MonitoringConfig{}
	}

	if dc.InstallConfig.Cluster.Monitoring.Prometheus == nil {
		dc.InstallConfig.Cluster.Monitoring.Prometheus = &files.PrometheusConfig{}
	}

	if dc.InstallConfig.Cluster.Monitoring.Prometheus.RemoteWrite == nil {
		dc.InstallConfig.Cluster.Monitoring.Prometheus.RemoteWrite = &files.RemoteWriteConfig{}
	}

	dc.InstallConfig.Cluster.Monitoring.Prometheus.RemoteWrite.Enabled = true
	dc.InstallConfig.Cluster.Monitoring.Prometheus.RemoteWrite.Url = b.Env.PrometheusRemoteWriteURL
	dc.InstallConfig.Cluster.Monitoring.Prometheus.RemoteWrite.ClusterName = dc.Name
	dc.InstallConfig.Cluster.Monitoring.Prometheus.RemoteWrite.Username = b.Env.PrometheusRemoteWriteUser
	dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: "promRemoteWritePassword", Fields: &files.SecretFields{Password: b.Env.PrometheusRemoteWritePassword}})
	dc.ConfigManager.GetVault().SetSecret(files.SecretEntry{Name: "promRemoteWriteUser", Fields: &files.SecretFields{Password: b.Env.PrometheusRemoteWriteUser}})
}

// regeneratePostgresCerts regenerates PostgreSQL TLS certificates when the IP/hostname
// changed or no private key was loaded from the vault.
func (b *GCPBootstrapper) regeneratePostgresCerts(dc *datacenter.DataCenter, previousPrimaryIP, previousPrimaryHostname string) error {
	vault := dc.ConfigManager.GetVault()
	primaryKeySecret := vault.GetSecret(files.SecretPostgresPrimaryServerKeyPem)
	primaryNeedsRegen := primaryKeySecret == nil || primaryKeySecret.File == nil ||
		previousPrimaryIP != dc.InstallConfig.Postgres.Primary.IP ||
		previousPrimaryHostname != dc.InstallConfig.Postgres.Primary.Hostname

	if primaryNeedsRegen {
		caSecret := vault.GetSecret(files.SecretPostgresCaKeyPem)
		if caSecret == nil || caSecret.File == nil {
			return fmt.Errorf("postgres CA key not found in vault")
		}
		primaryKeyPEM, primaryCertPEM, err := secrets.GenerateServerCertificate(
			caSecret.File.Content,
			dc.InstallConfig.Postgres.CACertPem,
			dc.InstallConfig.Postgres.Primary.Hostname,
			[]string{dc.InstallConfig.Postgres.Primary.IP})
		if err != nil {
			return fmt.Errorf("failed to generate primary server certificate: %w", err)
		}
		if err := secrets.ValidateCertKeyPair(primaryCertPEM, primaryKeyPEM); err != nil {
			return fmt.Errorf("primary PostgreSQL cert/key validation failed: %w", err)
		}
		vault.SetSecret(files.SecretEntry{Name: files.SecretPostgresPrimaryServerKeyPem, File: &files.SecretFile{Name: "primary.key", Content: primaryKeyPEM}})

		dc.InstallConfig.Postgres.Primary.SSLConfig.ServerCertPem = primaryCertPEM
	}

	if dc.InstallConfig.Postgres.Replica != nil {
		replicaKeySecret := vault.GetSecret(files.SecretPostgresReplicaServerKeyPem)
		if replicaKeySecret == nil || replicaKeySecret.File == nil {
			caSecret := vault.GetSecret(files.SecretPostgresCaKeyPem)
			if caSecret == nil || caSecret.File == nil {
				return fmt.Errorf("postgres CA key not found in vault")
			}
			replicaKeyPEM, replicaCertPEM, err := secrets.GenerateServerCertificate(
				caSecret.File.Content,
				dc.InstallConfig.Postgres.CACertPem,
				dc.InstallConfig.Postgres.Replica.Name,
				[]string{dc.InstallConfig.Postgres.Replica.IP})
			if err != nil {
				return fmt.Errorf("failed to generate replica server certificate: %w", err)
			}
			if err := secrets.ValidateCertKeyPair(replicaCertPEM, replicaKeyPEM); err != nil {
				return fmt.Errorf("replica PostgreSQL cert/key validation failed: %w", err)
			}
			vault.SetSecret(files.SecretEntry{Name: files.SecretPostgresReplicaServerKeyPem, File: &files.SecretFile{Name: "replica.key", Content: replicaKeyPEM}})

			dc.InstallConfig.Postgres.Replica.SSLConfig.ServerCertPem = replicaCertPEM
		}
	}
	return nil
}

// EnsureAgeKey generates the primary data center's age identity on the jumpbox.
func (b *GCPBootstrapper) EnsureAgeKey() error {
	b.ensureDataCenters()

	return b.ensureAgeKey(b.primaryDC())
}

func (b *GCPBootstrapper) ensureAgeKey(dc *datacenter.DataCenter) error {
	if b.Env.Jumpbox.NodeClient.HasFile(b.Env.Jumpbox, dc.RemoteAgeKeyPath()) {
		return nil
	}

	err := b.Env.Jumpbox.RunSSHCommand("root", fmt.Sprintf("mkdir -p %s; age-keygen -o %s", dc.SecretsDir, dc.RemoteAgeKeyPath()))
	if err != nil {
		return fmt.Errorf("failed to generate age key on jumpbox: %w", err)
	}

	return nil
}

// EnsureSecrets loads the primary data center's vault if it already exists locally.
func (b *GCPBootstrapper) EnsureSecrets() error {
	b.ensureDataCenters()

	return b.ensureSecrets(b.primaryDC())
}

func (b *GCPBootstrapper) ensureSecrets(dc *datacenter.DataCenter) error {
	if b.fw.Exists(dc.SecretsFilePath) {
		err := dc.ConfigManager.LoadVaultFromUnecryptedFile(dc.SecretsFilePath)
		if err != nil {
			return fmt.Errorf("failed to load vault file: %w", err)
		}
	}

	if dc.IsPrimary() {
		b.Env.Secrets = dc.ConfigManager.GetVault()
	}

	b.mirrorPrimaryDataCenter()
	return nil
}

// EncryptVault encrypts the primary data center's vault on the jumpbox.
func (b *GCPBootstrapper) EncryptVault() error {
	b.ensureDataCenters()

	return b.encryptVault(b.primaryDC())
}

func (b *GCPBootstrapper) encryptVault(dc *datacenter.DataCenter) error {
	err := b.Env.Jumpbox.RunSSHCommand("root", fmt.Sprintf("cp %s{,.bak}", dc.RemoteVaultPath()))
	if err != nil {
		return fmt.Errorf("failed backup vault on jumpbox: %w", err)
	}

	err = b.Env.Jumpbox.RunSSHCommand("root", fmt.Sprintf("sops --encrypt --in-place --age $(age-keygen -y %s) %s", dc.RemoteAgeKeyPath(), dc.RemoteVaultPath()))
	if err != nil {
		return fmt.Errorf("failed to encrypt vault on jumpbox: %w", err)
	}

	return nil
}

// decryptVault creates an unencrypted copy of the data center's vault in dst on the jumpbox.
// Make sure to delete the unencrypted file when not needed anymore.
func (b *GCPBootstrapper) decryptVault(dc *datacenter.DataCenter, dst string) error {
	err := b.Env.Jumpbox.RunSSHCommand("root", fmt.Sprintf("cp %s %s", dc.RemoteVaultPath(), dst))
	if err != nil {
		return fmt.Errorf("failed to create tmp vault on jumpbox: %w", err)
	}

	err = b.Env.Jumpbox.NodeClient.RunCommand(b.Env.Jumpbox, "root", "chmod 600 "+dst)
	if err != nil {
		return fmt.Errorf("failed to make vault file readable only for root on jumpbox: %w", err)
	}

	err = b.Env.Jumpbox.RunSSHCommand("root", fmt.Sprintf("SOPS_AGE_KEY_FILE=%s sops --decrypt --in-place %s", dc.RemoteAgeKeyPath(), dst))
	if err != nil {
		return fmt.Errorf("failed to decrypt vault on jumpbox: %w", err)
	}

	return nil
}
