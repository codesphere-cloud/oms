// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"log"
	"strings"

	csio "github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/spf13/cobra"
)

type UpdateInstallConfigCmd struct {
	cmd        *cobra.Command
	Opts       *UpdateInstallConfigOpts
	FileWriter util.FileIO
}

type UpdateInstallConfigOpts struct {
	*GlobalOptions

	ConfigFile string
	VaultFile  string

	WithComments bool

	// Fields that can be updated
	PostgresPrimaryIP       string
	PostgresPrimaryHostname string
	PostgresReplicaIP       string
	PostgresReplicaName     string
	PostgresServerAddress   string

	CephNodesSubnet string

	KubernetesAPIServerHost string
	KubernetesPodCIDR       string
	KubernetesServiceCIDR   string

	ClusterGatewayServiceType       string
	ClusterGatewayIPAddresses       []string
	ClusterPublicGatewayServiceType string
	ClusterPublicGatewayIPAddresses []string

	ACMEEnabled       bool
	ACMEIssuerName    string
	ACMEEmail         string
	ACMEServer        string
	ACMEEABKeyID      string
	ACMEEABMacKey     string
	ACMEDNS01Provider string

	CodesphereDomain                       string
	CodespherePublicIP                     string
	CodesphereWorkspaceHostingBaseDomain   string
	CodesphereCustomDomainsCNameBaseDomain string
	CodesphereDNSServers                   []string
}

func (c *UpdateInstallConfigCmd) RunE(_ *cobra.Command, args []string) error {
	icg := installer.NewInstallConfigManager()

	return c.UpdateInstallConfig(icg)
}

func AddUpdateInstallConfigCmd(update *cobra.Command, opts *GlobalOptions) {
	c := UpdateInstallConfigCmd{
		cmd: &cobra.Command{
			Use:   "install-config",
			Short: "Update an existing Codesphere installer configuration",
			Long: csio.Long(`Update fields in an existing install-config after generating one initially.
			
			This command allows you to modify specific configuration fields in an existing
			config.yaml and prod.vault.yaml without regenerating everything. OMS will
			automatically detect which dependent secrets and certificates need to be
			regenerated based on the changes made.
			
			For example, updating the PostgreSQL primary IP will trigger regeneration
			of the PostgreSQL server certificates that include that IP address.`),
			Example: formatExamplesWithBinary("update install-config", []csio.Example{
				{Cmd: "--postgres-primary-ip 10.10.0.4 --config config.yaml --vault prod.vault.yaml", Desc: "Update PostgreSQL primary IP and regenerate certificates"},
				{Cmd: "--domain new.example.com --config config.yaml --vault prod.vault.yaml", Desc: "Update Codesphere domain"},
				{Cmd: "--k8s-api-server 10.0.0.10 --config config.yaml --vault prod.vault.yaml", Desc: "Update Kubernetes API server host"},
			}, "oms-cli"),
		},
		Opts:       &UpdateInstallConfigOpts{GlobalOptions: opts},
		FileWriter: util.NewFilesystemWriter(),
	}

	c.cmd.Flags().StringVarP(&c.Opts.ConfigFile, "config", "c", "config.yaml", "Path to existing config.yaml file")
	c.cmd.Flags().StringVar(&c.Opts.VaultFile, "vault", "prod.vault.yaml", "Path to existing prod.vault.yaml file")

	c.cmd.Flags().BoolVar(&c.Opts.WithComments, "with-comments", false, "Add helpful comments to the generated YAML files")

	// PostgreSQL update flags
	c.cmd.Flags().StringVar(&c.Opts.PostgresPrimaryIP, "postgres-primary-ip", "", "Primary PostgreSQL server IP")
	c.cmd.Flags().StringVar(&c.Opts.PostgresPrimaryHostname, "postgres-primary-hostname", "", "Primary PostgreSQL server hostname")
	c.cmd.Flags().StringVar(&c.Opts.PostgresReplicaIP, "postgres-replica-ip", "", "Replica PostgreSQL server IP")
	c.cmd.Flags().StringVar(&c.Opts.PostgresReplicaName, "postgres-replica-name", "", "Replica PostgreSQL server name")
	c.cmd.Flags().StringVar(&c.Opts.PostgresServerAddress, "postgres-server-address", "", "PostgreSQL server address (for external mode)")

	// Ceph update flags
	c.cmd.Flags().StringVar(&c.Opts.CephNodesSubnet, "ceph-nodes-subnet", "", "Ceph nodes subnet")

	// Kubernetes update flags
	c.cmd.Flags().StringVar(&c.Opts.KubernetesAPIServerHost, "k8s-api-server", "", "Kubernetes API server host")
	c.cmd.Flags().StringVar(&c.Opts.KubernetesPodCIDR, "k8s-pod-cidr", "", "Kubernetes Pod CIDR")
	c.cmd.Flags().StringVar(&c.Opts.KubernetesServiceCIDR, "k8s-service-cidr", "", "Kubernetes Service CIDR")

	// Cluster Gateway update flags
	c.cmd.Flags().StringVar(&c.Opts.ClusterGatewayServiceType, "cluster-gateway-service-type", "", "Cluster gateway service type")
	c.cmd.Flags().StringSliceVar(&c.Opts.ClusterGatewayIPAddresses, "cluster-gateway-ips", []string{}, "Cluster gateway IP addresses (comma-separated)")
	c.cmd.Flags().StringVar(&c.Opts.ClusterPublicGatewayServiceType, "cluster-public-gateway-service-type", "", "Cluster public gateway service type")
	c.cmd.Flags().StringSliceVar(&c.Opts.ClusterPublicGatewayIPAddresses, "cluster-public-gateway-ips", []string{}, "Cluster public gateway IP addresses (comma-separated)")

	// ACME update flags
	c.cmd.Flags().BoolVar(&c.Opts.ACMEEnabled, "acme-enabled", false, "Enable ACME certificate issuer")
	c.cmd.Flags().StringVar(&c.Opts.ACMEIssuerName, "acme-issuer-name", "", "Name for the ACME ClusterIssuer")
	c.cmd.Flags().StringVar(&c.Opts.ACMEEmail, "acme-email", "", "Email address for ACME account registration")
	c.cmd.Flags().StringVar(&c.Opts.ACMEServer, "acme-server", "", "ACME server URL")
	c.cmd.Flags().StringVar(&c.Opts.ACMEEABKeyID, "acme-eab-key-id", "", "External Account Binding key ID (required by some ACME providers)")
	c.cmd.Flags().StringVar(&c.Opts.ACMEEABMacKey, "acme-eab-mac-key", "", "External Account Binding MAC key (required by some ACME providers)")
	c.cmd.Flags().StringVar(&c.Opts.ACMEDNS01Provider, "acme-dns01-provider", "", "DNS provider for DNS-01 solver")

	// Codesphere update flags
	c.cmd.Flags().StringVar(&c.Opts.CodesphereDomain, "domain", "", "Main Codesphere domain")
	c.cmd.Flags().StringVar(&c.Opts.CodespherePublicIP, "public-ip", "", "Codesphere public IP address")
	c.cmd.Flags().StringVar(&c.Opts.CodesphereWorkspaceHostingBaseDomain, "workspace-hosting-base-domain", "", "Workspace hosting base domain")
	c.cmd.Flags().StringVar(&c.Opts.CodesphereCustomDomainsCNameBaseDomain, "custom-domains-cname-base-domain", "", "Custom domains CNAME base domain")
	c.cmd.Flags().StringSliceVar(&c.Opts.CodesphereDNSServers, "dns-servers", []string{}, "DNS servers (comma-separated)")

	util.MarkFlagRequired(c.cmd, "config")
	util.MarkFlagRequired(c.cmd, "vault")

	c.cmd.RunE = c.RunE
	update.AddCommand(c.cmd)
}

func (c *UpdateInstallConfigCmd) UpdateInstallConfig(icg installer.InstallConfigManager) error {
	fmt.Printf("Loading existing configuration from: %s\n", c.Opts.ConfigFile)
	err := icg.LoadInstallConfigFromFile(c.Opts.ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to load config file: %w", err)
	}

	fmt.Printf("Loading existing vault from: %s\n", c.Opts.VaultFile)
	err = icg.LoadVaultFromFile(c.Opts.VaultFile)
	if err != nil {
		return fmt.Errorf("failed to load vault file: %w", err)
	}

	fmt.Println("Merging vault secrets into configuration...")
	err = icg.MergeVaultIntoConfig()
	if err != nil {
		return fmt.Errorf("failed to merge vault into config: %w", err)
	}

	tracker := NewSecretDependencyTracker()

	config := icg.GetInstallConfig()
	c.applyUpdates(config, tracker)

	errors := icg.ValidateInstallConfig()
	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed: %s", strings.Join(errors, ", "))
	}

	if tracker.HasChanges() {
		fmt.Println("\nRegenerating affected secrets and certificates...")
		if err := c.regenerateSecrets(config, tracker); err != nil {
			return fmt.Errorf("failed to regenerate secrets: %w", err)
		}
	} else {
		fmt.Println("\nNo changes detected that require secret regeneration.")
	}

	if err := icg.WriteInstallConfig(c.Opts.ConfigFile, c.Opts.WithComments); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if err := icg.WriteVault(c.Opts.VaultFile, c.Opts.WithComments); err != nil {
		return fmt.Errorf("failed to write vault file: %w", err)
	}

	c.printSuccessMessage(tracker)

	return nil
}

func (c *UpdateInstallConfigCmd) applyUpdates(config *files.RootConfig, tracker *SecretDependencyTracker) {
	// PostgreSQL updates
	if c.Opts.PostgresPrimaryIP != "" || c.Opts.PostgresPrimaryHostname != "" {
		if config.Postgres.Primary != nil {
			if c.Opts.PostgresPrimaryIP != "" && config.Postgres.Primary.IP != c.Opts.PostgresPrimaryIP {
				fmt.Printf("Updating PostgreSQL primary IP: %s -> %s\n", config.Postgres.Primary.IP, c.Opts.PostgresPrimaryIP)
				config.Postgres.Primary.IP = c.Opts.PostgresPrimaryIP
				tracker.MarkPostgresPrimaryCertNeedsRegen()
			}
			if c.Opts.PostgresPrimaryHostname != "" && config.Postgres.Primary.Hostname != c.Opts.PostgresPrimaryHostname {
				fmt.Printf("Updating PostgreSQL primary hostname: %s -> %s\n", config.Postgres.Primary.Hostname, c.Opts.PostgresPrimaryHostname)
				config.Postgres.Primary.Hostname = c.Opts.PostgresPrimaryHostname
				tracker.MarkPostgresPrimaryCertNeedsRegen()
			}
		}
	}

	if c.Opts.PostgresReplicaIP != "" || c.Opts.PostgresReplicaName != "" {
		if config.Postgres.Replica != nil {
			if c.Opts.PostgresReplicaIP != "" && config.Postgres.Replica.IP != c.Opts.PostgresReplicaIP {
				fmt.Printf("Updating PostgreSQL replica IP: %s -> %s\n", config.Postgres.Replica.IP, c.Opts.PostgresReplicaIP)
				config.Postgres.Replica.IP = c.Opts.PostgresReplicaIP
				tracker.MarkPostgresReplicaCertNeedsRegen()
			}
			if c.Opts.PostgresReplicaName != "" && config.Postgres.Replica.Name != c.Opts.PostgresReplicaName {
				fmt.Printf("Updating PostgreSQL replica name: %s -> %s\n", config.Postgres.Replica.Name, c.Opts.PostgresReplicaName)
				config.Postgres.Replica.Name = c.Opts.PostgresReplicaName
				tracker.MarkPostgresReplicaCertNeedsRegen()
			}
		}
	}

	if c.Opts.PostgresServerAddress != "" && config.Postgres.ServerAddress != c.Opts.PostgresServerAddress {
		fmt.Printf("Updating PostgreSQL server address: %s -> %s\n", config.Postgres.ServerAddress, c.Opts.PostgresServerAddress)
		config.Postgres.ServerAddress = c.Opts.PostgresServerAddress
	}

	// Ceph updates
	if c.Opts.CephNodesSubnet != "" && config.Ceph.NodesSubnet != c.Opts.CephNodesSubnet {
		fmt.Printf("Updating Ceph nodes subnet: %s -> %s\n", config.Ceph.NodesSubnet, c.Opts.CephNodesSubnet)
		config.Ceph.NodesSubnet = c.Opts.CephNodesSubnet
	}

	// Kubernetes updates
	if c.Opts.KubernetesAPIServerHost != "" && config.Kubernetes.APIServerHost != c.Opts.KubernetesAPIServerHost {
		fmt.Printf("Updating Kubernetes API server host: %s -> %s\n", config.Kubernetes.APIServerHost, c.Opts.KubernetesAPIServerHost)
		config.Kubernetes.APIServerHost = c.Opts.KubernetesAPIServerHost
	}

	if c.Opts.KubernetesPodCIDR != "" && config.Kubernetes.PodCIDR != c.Opts.KubernetesPodCIDR {
		fmt.Printf("Updating Kubernetes Pod CIDR: %s -> %s\n", config.Kubernetes.PodCIDR, c.Opts.KubernetesPodCIDR)
		config.Kubernetes.PodCIDR = c.Opts.KubernetesPodCIDR
	}

	if c.Opts.KubernetesServiceCIDR != "" && config.Kubernetes.ServiceCIDR != c.Opts.KubernetesServiceCIDR {
		fmt.Printf("Updating Kubernetes Service CIDR: %s -> %s\n", config.Kubernetes.ServiceCIDR, c.Opts.KubernetesServiceCIDR)
		config.Kubernetes.ServiceCIDR = c.Opts.KubernetesServiceCIDR
	}

	// Cluster Gateway updates
	if c.Opts.ClusterGatewayServiceType != "" && config.Cluster.Gateway.ServiceType != c.Opts.ClusterGatewayServiceType {
		fmt.Printf("Updating cluster gateway service type: %s -> %s\n", config.Cluster.Gateway.ServiceType, c.Opts.ClusterGatewayServiceType)
		config.Cluster.Gateway.ServiceType = c.Opts.ClusterGatewayServiceType
	}

	if len(c.Opts.ClusterGatewayIPAddresses) > 0 {
		fmt.Printf("Updating cluster gateway IP addresses\n")
		config.Cluster.Gateway.IPAddresses = c.Opts.ClusterGatewayIPAddresses
	}

	if c.Opts.ClusterPublicGatewayServiceType != "" && config.Cluster.PublicGateway.ServiceType != c.Opts.ClusterPublicGatewayServiceType {
		fmt.Printf("Updating cluster public gateway service type: %s -> %s\n", config.Cluster.PublicGateway.ServiceType, c.Opts.ClusterPublicGatewayServiceType)
		config.Cluster.PublicGateway.ServiceType = c.Opts.ClusterPublicGatewayServiceType
	}

	if len(c.Opts.ClusterPublicGatewayIPAddresses) > 0 {
		fmt.Printf("Updating cluster public gateway IP addresses\n")
		config.Cluster.PublicGateway.IPAddresses = c.Opts.ClusterPublicGatewayIPAddresses
	}

	// ACME updates
	acmeChanged := false
	if c.Opts.ACMEEnabled {
		if config.Cluster.Certificates.ACME == nil {
			config.Cluster.Certificates.ACME = &files.ACMEConfig{}
		}

		if !config.Cluster.Certificates.ACME.Enabled {
			log.Printf("Enabling ACME certificate issuer\n")
			config.Cluster.Certificates.ACME.Enabled = true
			acmeChanged = true
		}

		if c.Opts.ACMEIssuerName != "" && config.Cluster.Certificates.ACME.Name != c.Opts.ACMEIssuerName {
			log.Printf("Updating ACME issuer name: %s -> %s\n", config.Cluster.Certificates.ACME.Name, c.Opts.ACMEIssuerName)
			config.Cluster.Certificates.ACME.Name = c.Opts.ACMEIssuerName
			acmeChanged = true
		}

		if c.Opts.ACMEEmail != "" && config.Cluster.Certificates.ACME.Email != c.Opts.ACMEEmail {
			log.Printf("Updating ACME email: %s -> %s\n", config.Cluster.Certificates.ACME.Email, c.Opts.ACMEEmail)
			config.Cluster.Certificates.ACME.Email = c.Opts.ACMEEmail
			acmeChanged = true
		}

		if c.Opts.ACMEServer != "" && config.Cluster.Certificates.ACME.Server != c.Opts.ACMEServer {
			log.Printf("Updating ACME server: %s -> %s\n", config.Cluster.Certificates.ACME.Server, c.Opts.ACMEServer)
			config.Cluster.Certificates.ACME.Server = c.Opts.ACMEServer
			acmeChanged = true
		}

		if c.Opts.ACMEEABKeyID != "" && config.Cluster.Certificates.ACME.EABKeyID != c.Opts.ACMEEABKeyID {
			log.Printf("Updating ACME EAB key ID: %s -> %s\n", config.Cluster.Certificates.ACME.EABKeyID, c.Opts.ACMEEABKeyID)
			config.Cluster.Certificates.ACME.EABKeyID = c.Opts.ACMEEABKeyID
			acmeChanged = true
		}

		if c.Opts.ACMEEABMacKey != "" && config.Cluster.Certificates.ACME.EABMacKey != c.Opts.ACMEEABMacKey {
			log.Printf("Updating ACME EAB MAC key\n")
			config.Cluster.Certificates.ACME.EABMacKey = c.Opts.ACMEEABMacKey
			acmeChanged = true
		}

		// Update DNS-01 solver configuration
		if c.Opts.ACMEDNS01Provider != "" {
			if config.Cluster.Certificates.ACME.Solver.DNS01 == nil {
				config.Cluster.Certificates.ACME.Solver.DNS01 = &files.ACMEDNS01Solver{}
			}
			if config.Cluster.Certificates.ACME.Solver.DNS01.Provider != c.Opts.ACMEDNS01Provider {
				log.Printf("Updating ACME DNS-01 provider: %s -> %s\n",
					config.Cluster.Certificates.ACME.Solver.DNS01.Provider, c.Opts.ACMEDNS01Provider)
				config.Cluster.Certificates.ACME.Solver.DNS01.Provider = c.Opts.ACMEDNS01Provider
				acmeChanged = true
			}
		}

		if acmeChanged {
			tracker.MarkACMEConfigChanged()
		}
	}

	// Codesphere updates
	if c.Opts.CodesphereDomain != "" && config.Codesphere.Domain != c.Opts.CodesphereDomain {
		fmt.Printf("Updating Codesphere domain: %s -> %s\n", config.Codesphere.Domain, c.Opts.CodesphereDomain)
		config.Codesphere.Domain = c.Opts.CodesphereDomain
	}

	if c.Opts.CodespherePublicIP != "" && config.Codesphere.PublicIP != c.Opts.CodespherePublicIP {
		fmt.Printf("Updating Codesphere public IP: %s -> %s\n", config.Codesphere.PublicIP, c.Opts.CodespherePublicIP)
		config.Codesphere.PublicIP = c.Opts.CodespherePublicIP
	}

	if c.Opts.CodesphereWorkspaceHostingBaseDomain != "" && config.Codesphere.WorkspaceHostingBaseDomain != c.Opts.CodesphereWorkspaceHostingBaseDomain {
		fmt.Printf("Updating workspace hosting base domain: %s -> %s\n", config.Codesphere.WorkspaceHostingBaseDomain, c.Opts.CodesphereWorkspaceHostingBaseDomain)
		config.Codesphere.WorkspaceHostingBaseDomain = c.Opts.CodesphereWorkspaceHostingBaseDomain
	}

	if c.Opts.CodesphereCustomDomainsCNameBaseDomain != "" && config.Codesphere.CustomDomains.CNameBaseDomain != c.Opts.CodesphereCustomDomainsCNameBaseDomain {
		fmt.Printf("Updating custom domains CNAME base domain: %s -> %s\n", config.Codesphere.CustomDomains.CNameBaseDomain, c.Opts.CodesphereCustomDomainsCNameBaseDomain)
		config.Codesphere.CustomDomains.CNameBaseDomain = c.Opts.CodesphereCustomDomainsCNameBaseDomain
	}

	if len(c.Opts.CodesphereDNSServers) > 0 {
		fmt.Printf("Updating DNS servers\n")
		config.Codesphere.DNSServers = c.Opts.CodesphereDNSServers
	}
}

func (c *UpdateInstallConfigCmd) regenerateSecrets(config *files.RootConfig, tracker *SecretDependencyTracker) error {
	if tracker.NeedsPostgresPrimaryCertRegen() {
		fmt.Println("  - Regenerating PostgreSQL primary server certificate...")
		var err error
		config.Postgres.Primary.PrivateKey, config.Postgres.Primary.SSLConfig.ServerCertPem, err = installer.GenerateServerCertificate(
			config.Postgres.CaCertPrivateKey,
			config.Postgres.CACertPem,
			config.Postgres.Primary.Hostname,
			[]string{config.Postgres.Primary.IP},
		)
		if err != nil {
			return fmt.Errorf("failed to regenerate primary PostgreSQL certificate: %w", err)
		}
	}

	if tracker.NeedsPostgresReplicaCertRegen() && config.Postgres.Replica != nil {
		fmt.Println("  - Regenerating PostgreSQL replica server certificate...")
		var err error
		config.Postgres.ReplicaPrivateKey, config.Postgres.Replica.SSLConfig.ServerCertPem, err = installer.GenerateServerCertificate(
			config.Postgres.CaCertPrivateKey,
			config.Postgres.CACertPem,
			config.Postgres.Replica.Name,
			[]string{config.Postgres.Replica.IP},
		)
		if err != nil {
			return fmt.Errorf("failed to regenerate replica PostgreSQL certificate: %w", err)
		}
	}

	return nil
}

func (c *UpdateInstallConfigCmd) printSuccessMessage(tracker *SecretDependencyTracker) {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("Configuration successfully updated!")
	fmt.Println(strings.Repeat("=", 70))

	if tracker.HasChanges() {
		fmt.Println("\nRegenerated secrets:")
		if tracker.NeedsPostgresPrimaryCertRegen() {
			fmt.Println("  ✓ PostgreSQL primary server certificate")
		}
		if tracker.NeedsPostgresReplicaCertRegen() {
			fmt.Println("  ✓ PostgreSQL replica server certificate")
		}
		if tracker.ACMEConfigChanged() {
			log.Println("  ✓ ACME configuration updated")
		}
	}

	fmt.Println("\nIMPORTANT: The vault file has been updated with new secrets.")
	fmt.Println("   Remember to re-encrypt it with SOPS before storing.")
	fmt.Println()
}

type SecretDependencyTracker struct {
	postgresPrimaryCertNeedsRegen bool
	postgresReplicaCertNeedsRegen bool
	acmeConfigChanged             bool
}

func NewSecretDependencyTracker() *SecretDependencyTracker {
	return &SecretDependencyTracker{}
}

func (t *SecretDependencyTracker) MarkPostgresPrimaryCertNeedsRegen() {
	t.postgresPrimaryCertNeedsRegen = true
}

func (t *SecretDependencyTracker) MarkPostgresReplicaCertNeedsRegen() {
	t.postgresReplicaCertNeedsRegen = true
}

func (t *SecretDependencyTracker) MarkACMEConfigChanged() {
	t.acmeConfigChanged = true
}

func (t *SecretDependencyTracker) NeedsPostgresPrimaryCertRegen() bool {
	return t.postgresPrimaryCertNeedsRegen
}

func (t *SecretDependencyTracker) NeedsPostgresReplicaCertRegen() bool {
	return t.postgresReplicaCertNeedsRegen
}

func (t *SecretDependencyTracker) ACMEConfigChanged() bool {
	return t.acmeConfigChanged
}

func (t *SecretDependencyTracker) HasChanges() bool {
	return t.postgresPrimaryCertNeedsRegen || t.postgresReplicaCertNeedsRegen || t.acmeConfigChanged
}
