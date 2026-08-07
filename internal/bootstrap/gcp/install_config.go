// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/codesphere-cloud/oms/internal/bootstrap"
	"github.com/codesphere-cloud/oms/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
	"github.com/codesphere-cloud/oms/internal/util"
)

const (
	remoteInstallConfigPath string = "/etc/codesphere/config.yaml"
	// sharedPostgresPort is the port the shared PostgreSQL server listens on. Secondary data
	// centers need it spelled out because they connect to it as an external server.
	sharedPostgresPort int = 5432
)

// errRemoteConfigMissing reports that the jumpbox holds no config for a data center. Recovering
// a secondary data center tolerates this, since --multi-dc can add one to an existing project.
var errRemoteConfigMissing = errors.New("no install config found on the jumpbox")

// EnsureInstallConfig prepares the primary data center's install config. Secondary data centers
// are handled separately in Bootstrap, after the primary's secrets exist.
func (b *GCPBootstrapper) EnsureInstallConfig() error {
	b.ensureDataCenters()

	return b.ensureInstallConfig(b.primaryDC())
}

// ensureInstallConfig uses the data center's local config or recovers it from an existing
// jumpbox if desired. Else it applies the minimal profile to a new config.
func (b *GCPBootstrapper) ensureInstallConfig(dc *DataCenter) error {
	// recovery will overwrite local config or create a new file
	if b.Env.RecoverConfig {
		err := b.recoverConfig(dc)
		if errors.Is(err, errRemoteConfigMissing) && !dc.IsPrimary() {
			// A secondary data center may not exist on the jumpbox yet, which is the case when
			// --multi-dc is used to add one to an existing single-DC project. Its config is
			// derived from the primary's instead.
			b.stlog.Logf("No config found on the jumpbox for data center %d, generating a new one", dc.ID)
		} else if err != nil {
			return fmt.Errorf("failed to recover config: %w", err)
		}
	}

	if b.fw.Exists(dc.InstallConfigPath) {
		if err := b.loadVaultForConfigTemplating(dc); err != nil {
			return fmt.Errorf("failed to load vault templating: %w", err)
		}

		err := dc.icg.LoadInstallConfigFromFile(dc.InstallConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}

		dc.ExistingConfigUsed = true
		dc.InstallConfig = dc.icg.GetInstallConfig()
	} else if dc.IsPrimary() {
		err := dc.icg.ApplyProfile("minimal")
		if err != nil {
			return fmt.Errorf("failed to apply profile: %w", err)
		}
		dc.InstallConfig = dc.icg.GetInstallConfig()
	}
	// A secondary data center without a config of its own is left unset here, so
	// seedSecondaryDataCenter can derive it from the primary data center instead of the profile.

	b.mirrorPrimaryDataCenter()

	return nil
}

func (b *GCPBootstrapper) loadVaultForConfigTemplating(dc *DataCenter) error {
	if !b.fw.Exists(dc.SecretsFilePath) {
		return nil
	}

	// during bootstrapping, the vault is not yet encrpyted
	if err := dc.icg.LoadVaultFromUnecryptedFile(dc.SecretsFilePath); err != nil {
		return fmt.Errorf("failed to load vault from file: %w", err)
	}

	return nil
}

// recoverConfig downloads the config and secrets from the jumpbox if it exists.
// Since recovery is done when the project or VMs are not ensured, we need to search for the jumpbox IP first.
// Returns an error if project or jumpbox does not exist or downloading fails.
func (b *GCPBootstrapper) recoverConfig(dc *DataCenter) error {
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

	// Only a secondary data center may legitimately be absent from the jumpbox, so only there is
	// it worth probing first; the primary's missing config stays a download failure.
	if !dc.IsPrimary() && !b.Env.Jumpbox.NodeClient.HasFile(jumpbox, dc.RemoteConfigPath) {
		return fmt.Errorf("%w at %s", errRemoteConfigMissing, dc.RemoteConfigPath)
	}

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
func (b *GCPBootstrapper) recoverVault(dc *DataCenter) error {
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
// install config. Secondary data centers go through updateInstallConfig directly, after their
// config and vault have been derived from the primary's.
func (b *GCPBootstrapper) UpdateInstallConfig() error {
	b.ensureDataCenters()

	return b.updateInstallConfig(b.primaryDC())
}

func (b *GCPBootstrapper) updateInstallConfig(dc *DataCenter) error {
	// Update install config with necessary values
	dc.InstallConfig.Datacenter = datacenterConfig(dc)
	// Each data center reads and writes its own vault. The installer resolves the vault from
	// secrets.baseDir, so sharing a directory would let one data center's ceph and kubernetes
	// steps overwrite another's credentials.
	dc.InstallConfig.Secrets.BaseDir = dc.SecretsDir
	if b.Env.ContainerRegistryURL != "" {
		dc.InstallConfig.Registry.Server = b.Env.ContainerRegistryURL
	}
	if b.Env.RegistryUsername != "" || b.Env.RegistryPassword != "" {
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretRegistryUsername, Fields: &files.SecretFields{Password: b.Env.RegistryUsername}})
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretRegistryPassword, Fields: &files.SecretFields{Password: b.Env.RegistryPassword}})
	}
	if b.Env.RegistryType == RegistryTypeGitHub {
		dc.InstallConfig.Registry.ReplaceImagesInBom = false
		dc.InstallConfig.Registry.LoadContainerImages = false
	} else {
		dc.InstallConfig.Registry.ReplaceImagesInBom = true
		dc.InstallConfig.Registry.LoadContainerImages = true
	}

	previousPrimaryIP, previousPrimaryHostname := b.applyPostgresConfig(dc)
	b.applyDataCenterTopology(dc)

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

	b.applySshProxyConfig(dc)

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
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretAcmeEabMacKey, Fields: &files.SecretFields{Password: b64MacKey}})
	}
	dc.InstallConfig.Codesphere.CertIssuer = files.CertIssuerConfig{
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
	if b.Env.MultiDC {
		dc.InstallConfig.Codesphere.PublicIP = dc.PublicGatewayIP
	} else {
		dc.InstallConfig.Codesphere.PublicIP = dc.ControlPlaneNodes[1].GetExternalIP()
	}
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
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretGithubAppsClientId, Fields: &files.SecretFields{Password: b.Env.GitHubAppClientID}})
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretGithubAppsClientSecret, Fields: &files.SecretFields{Password: b.Env.GitHubAppClientSecret}})
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
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretGitlabAppClientId, Fields: &files.SecretFields{Password: b.Env.GitLabAppClientID}})
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretGitlabAppClientSecret, Fields: &files.SecretFields{Password: b.Env.GitLabAppClientSecret}})
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
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretBitbucketAppsClientId, Fields: &files.SecretFields{Password: b.Env.BitbucketAppClientID}})
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretBitbucketAppsClientSecret, Fields: &files.SecretFields{Password: b.Env.BitbucketAppClientSecret}})
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
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretAzureDevOpsAppClientId, Fields: &files.SecretFields{Password: b.Env.AzureDevOpsAppClientID}})
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretAzureDevOpsAppClientSecret, Fields: &files.SecretFields{Password: b.Env.AzureDevOpsAppClientSecret}})
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
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretOidcClientId, Fields: &files.SecretFields{Password: b.Env.OidcClientID}})
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretOidcClientSecret, Fields: &files.SecretFields{Password: b.Env.OidcClientSecret}})
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

	// A secondary data center always generates: its vault was seeded from the primary's, so the
	// sentinels stop everything except its own ingress CA and cephadm key from being regenerated.
	if !dc.ExistingConfigUsed || !dc.IsPrimary() {
		err := dc.icg.GenerateSecrets()
		if err != nil {
			return fmt.Errorf("failed to generate secrets: %w", err)
		}
	} else {
		if err := b.regeneratePostgresCerts(dc, previousPrimaryIP, previousPrimaryHostname); err != nil {
			return err
		}
	}

	if !dc.IsPrimary() {
		if err := b.verifySecondaryDataCenterSecrets(b.primaryDC(), dc); err != nil {
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
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretCentralOtelCreds, Fields: &files.SecretFields{Username: b.Env.CentralOtelUsername, Password: b.Env.CentralOtelPassword}})
	}

	if b.Env.OpenBaoURI != "" {
		dc.InstallConfig.Codesphere.OpenBao = &files.OpenBaoConfig{
			Engine: b.Env.OpenBaoEngine,
			URI:    b.Env.OpenBaoURI,
			User:   b.Env.OpenBaoUser,
		}
		dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretOpenBaoPassword, Fields: &files.SecretFields{Password: b.Env.OpenBaoPassword}})
	}

	if err := dc.icg.WriteInstallConfig(dc.InstallConfigPath, true); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if err := dc.icg.WriteVault(dc.SecretsFilePath, true); err != nil {
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

// datacenterConfig describes a data center the way the install config does. All data centers of a
// bootstrapped instance live in the same GCP region, so city and country code are the same for all
// of them.
func datacenterConfig(dc *DataCenter) files.DatacenterConfig {
	return files.DatacenterConfig{
		ID:          dc.ID,
		Name:        dc.Name,
		City:        "Karlsruhe",
		CountryCode: "DE",
	}
}

// applyDataCenterTopology tells the platform about every data center of the installation. Without
// it, the installer defaults the list to the local data center, so each data center of a multi-DC
// instance would render as a single-data-center one. The list is identical in every data center's
// config; dataCenter stays the local one, so the platform still knows which data center it runs in.
//
// A single data center keeps the list unset and relies on the installer's default, so its config is
// unchanged from what OMS has always written.
func (b *GCPBootstrapper) applyDataCenterTopology(dc *DataCenter) {
	if len(b.Env.DataCenters) < 2 {
		return
	}

	all := make([]files.DatacenterConfig, 0, len(b.Env.DataCenters))
	for _, other := range b.Env.DataCenters {
		all = append(all, datacenterConfig(other))
	}
	dc.InstallConfig.DataCenters = all
	// Clients land in the primary data center, which is also the one cs.<base-domain> resolves to.
	dc.InstallConfig.DefaultDataCenterID = b.primaryDC().ID

	b.dropAvailableDataCentersOverride(dc)
}

// dropAvailableDataCentersOverride removes the chart override OMS used to write before the
// installer supported dataCenters in the config. A recovered config may still carry it, where it
// would shadow the list above with a bare ID list. Any other override content is left alone.
func (b *GCPBootstrapper) dropAvailableDataCentersOverride(dc *DataCenter) {
	global, ok := dc.InstallConfig.Codesphere.Override["global"].(map[string]interface{})
	if !ok {
		return
	}

	delete(global, "availableDataCenters")
	if len(global) == 0 {
		delete(dc.InstallConfig.Codesphere.Override, "global")
	}
	if len(dc.InstallConfig.Codesphere.Override) == 0 {
		dc.InstallConfig.Codesphere.Override = nil
	}
}

// applyPostgresConfig points the data center at its PostgreSQL server. The primary data center
// installs the server on its own node; every other data center connects to that same server as
// an external one and skips the postgres install step.
//
// Returns the primary IP and hostname the config held before, so regeneratePostgresCerts can
// tell whether the server's identity changed.
func (b *GCPBootstrapper) applyPostgresConfig(dc *DataCenter) (previousIP, previousHostname string) {
	if dc.ExternalPostgres {
		dc.InstallConfig.Postgres = files.PostgresConfig{
			Mode: "external",
			// The server certificate carries only an IP SAN, so the address must be the IP.
			ServerAddress: b.Env.PostgreSQLNode.GetInternalIP(),
			Port:          sharedPostgresPort,
			// The CA certificate lives in the config, not the vault, and cannot be re-derived
			// from the CA key — so it has to be copied from the primary data center.
			CACertPem: b.primaryDC().InstallConfig.Postgres.CACertPem,
			Primary:   nil,
			Replica:   nil,
		}
		b.skipInstallerStep(dc, "postgres")

		return "", ""
	}

	if dc.InstallConfig.Postgres.Primary == nil {
		dc.InstallConfig.Postgres.Primary = &files.PostgresPrimaryConfig{
			Hostname: b.Env.PostgreSQLNode.GetName(),
		}
	}

	previousIP = dc.InstallConfig.Postgres.Primary.IP
	previousHostname = dc.InstallConfig.Postgres.Primary.Hostname
	dc.InstallConfig.Postgres.Primary.IP = b.Env.PostgreSQLNode.GetInternalIP()
	dc.InstallConfig.Postgres.Primary.Hostname = b.Env.PostgreSQLNode.GetName()

	return previousIP, previousHostname
}

// skipInstallerStep persists a skipped installer step in the data center's config, so manual
// `oms install codesphere` re-runs on the jumpbox skip it too.
func (b *GCPBootstrapper) skipInstallerStep(dc *DataCenter, step string) {
	if dc.InstallConfig.Operations == nil {
		dc.InstallConfig.Operations = &files.OperationsConfig{}
	}
	if !slices.Contains(dc.InstallConfig.Operations.Skip, step) {
		dc.InstallConfig.Operations.Skip = append(dc.InstallConfig.Operations.Skip, step)
	}
}

func (b *GCPBootstrapper) applySshProxyConfig(dc *DataCenter) {
	dc.InstallConfig.PcApps = util.DeepMergeMaps(dc.InstallConfig.PcApps, files.ChartValues{
		"applications": map[string]any{
			"ssh-workspace-proxy": map[string]any{
				"enabled": true,
				"valuesObject": map[string]any{
					"service": map[string]any{
						"enabled":        true,
						"type":           "LoadBalancer",
						"loadBalancerIP": dc.SshProxyIP,
						"annotations": map[string]any{
							"cloud.google.com/load-balancer-ipv4": dc.SshProxyIP,
						},
					},
				},
			},
		},
	})
}

func (b *GCPBootstrapper) applyExternalLokiConfig(dc *DataCenter) {
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
	dc.icg.GetVault().SetSecret(files.SecretEntry{Name: files.SecretLokiGatewayBasicAuthPassword, Fields: &files.SecretFields{Password: b.Env.ExternalLokiSecret}})
}

func (b *GCPBootstrapper) applyPrometheusRemoteWriteConfig(dc *DataCenter) {
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
	dc.icg.GetVault().SetSecret(files.SecretEntry{Name: "promRemoteWritePassword", Fields: &files.SecretFields{Password: b.Env.PrometheusRemoteWritePassword}})
	dc.icg.GetVault().SetSecret(files.SecretEntry{Name: "promRemoteWriteUser", Fields: &files.SecretFields{Password: b.Env.PrometheusRemoteWriteUser}})
}

// regeneratePostgresCerts regenerates PostgreSQL TLS certificates when the IP/hostname
// changed or no private key was loaded from the vault. It is a no-op for a data center that
// connects to an external server, since that server owns its own certificates.
func (b *GCPBootstrapper) regeneratePostgresCerts(dc *DataCenter, previousPrimaryIP, previousPrimaryHostname string) error {
	if dc.InstallConfig.Postgres.Primary == nil {
		return nil
	}

	vault := dc.icg.GetVault()
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

func (b *GCPBootstrapper) ensureAgeKey(dc *DataCenter) error {
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

func (b *GCPBootstrapper) ensureSecrets(dc *DataCenter) error {
	if b.fw.Exists(dc.SecretsFilePath) {
		err := dc.icg.LoadVaultFromUnecryptedFile(dc.SecretsFilePath)
		if err != nil {
			return fmt.Errorf("failed to load vault file: %w", err)
		}
	}
	if dc.IsPrimary() {
		b.Env.Secrets = dc.icg.GetVault()
	}
	b.mirrorPrimaryDataCenter()
	return nil
}

// EncryptVault encrypts the primary data center's vault on the jumpbox.
func (b *GCPBootstrapper) EncryptVault() error {
	b.ensureDataCenters()

	return b.encryptVault(b.primaryDC())
}

func (b *GCPBootstrapper) encryptVault(dc *DataCenter) error {
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
func (b *GCPBootstrapper) decryptVault(dc *DataCenter, dst string) error {
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

// writeDataCenterConfig writes the data center's install config and vault, then places the
// encrypted vault and its age identity on the jumpbox.
func (b *GCPBootstrapper) writeDataCenterConfig(dc *DataCenter) error {
	err := b.stlog.Step(dc.StepName("Update install config"), func() error {
		return b.updateInstallConfig(dc)
	})
	if err != nil {
		return fmt.Errorf("failed to update install config of data center %d: %w", dc.ID, err)
	}

	err = b.stlog.Step(dc.StepName("Ensure age key"), func() error {
		return b.ensureAgeKey(dc)
	})
	if err != nil {
		return fmt.Errorf("failed to ensure age key of data center %d: %w", dc.ID, err)
	}

	err = b.stlog.Step(dc.StepName("Encrypt vault"), func() error {
		return b.encryptVault(dc)
	})
	if err != nil {
		return fmt.Errorf("failed to encrypt vault of data center %d: %w", dc.ID, err)
	}

	return nil
}

// seedSecondaryDataCenter derives a secondary data center's config and vault from the primary
// one's, for the parts it does not already have. Secrets tied to the shared database and to
// cross-data-center authentication are copied verbatim; the per-cluster ones are dropped so
// GenerateSecrets regenerates them for this data center.
//
// Anything the data center already loaded from its own files is kept, so re-runs do not rotate a
// live installation's secrets.
func (b *GCPBootstrapper) seedSecondaryDataCenter(primary, dc *DataCenter) error {
	if dc.InstallConfig == nil {
		config, err := secrets.DeriveDataCenterConfig(primary.InstallConfig)
		if err != nil {
			return fmt.Errorf("failed to derive config from data center %d: %w", primary.ID, err)
		}
		dc.icg.SetInstallConfig(config)
		dc.InstallConfig = dc.icg.GetInstallConfig()
	}

	if len(dc.icg.GetVault().Secrets) == 0 {
		dc.icg.SetVault(secrets.DeriveDataCenterVault(primary.icg.GetVault()))
	}

	return nil
}

// verifySecondaryDataCenterSecrets fails the bootstrap if a secondary data center's secrets
// diverged from the primary's where they must match. Divergent PostgreSQL roles would let one
// data center's install rotate credentials the other one is using, and a divergent token key
// would break cross-data-center authentication — both only visible long after the fact.
func (b *GCPBootstrapper) verifySecondaryDataCenterSecrets(primary, dc *DataCenter) error {
	primaryVault := primary.icg.GetVault()
	vault := dc.icg.GetVault()

	// openFgaPresharedKey is in here because a secondary data center that already has a vault is
	// not re-derived, but GenerateSecrets still runs for it — so a vault predating the key gets a
	// freshly generated one that the shared OpenFGA instance rejects.
	shared := []string{files.SecretTokenPrivateKey, files.SecretPostgresPassword, files.SecretOpenFgaPresharedKey}
	for _, svc := range codesphere.PostgresServices {
		shared = append(shared, files.PostgresUserSecretName(svc.Name), files.PostgresPasswordSecretName(svc.Name))
	}
	for _, name := range shared {
		expected := primaryVault.GetSecret(name)
		if expected == nil {
			continue
		}
		if !reflect.DeepEqual(vault.GetSecret(name), expected) {
			return fmt.Errorf("secret %q of data center %d differs from the primary data center, but both data centers share it", name, dc.ID)
		}
	}

	if dc.InstallConfig.Postgres.CACertPem == "" {
		return fmt.Errorf("data center %d has no postgres CA certificate and could not verify the shared server", dc.ID)
	}

	for _, name := range []string{files.SecretKubeConfig, files.SecretCephSshPrivateKey} {
		if vault.GetSecret(name) == nil {
			continue
		}
		if reflect.DeepEqual(vault.GetSecret(name), primaryVault.GetSecret(name)) {
			return fmt.Errorf("secret %q of data center %d is the primary data center's, but they run separate clusters", name, dc.ID)
		}
	}

	return nil
}
