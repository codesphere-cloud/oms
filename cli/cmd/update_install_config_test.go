// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

func quoteYAMLString(s string) string {
	// Escape backslashes and quotes, then convert newlines to \n
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return `"` + s + `"`
}

var _ = Describe("UpdateInstallConfig", func() {
	var (
		configFile    *os.File
		vaultFile     *os.File
		initialConfig string
		initialVault  string
		cmd           *UpdateInstallConfigCmd
		opts          *UpdateInstallConfigOpts
		testCAKeyPem  string
		testCACertPem string
	)

	BeforeEach(func() {
		var err error
		configFile, err = os.CreateTemp("", "config-*.yaml")
		Expect(err).NotTo(HaveOccurred())

		vaultFile, err = os.CreateTemp("", "vault-*.yaml")
		Expect(err).NotTo(HaveOccurred())

		testCAKeyPem, testCACertPem, err = installer.GenerateCA("Test CA", "US", "Test City", "Test Org")
		Expect(err).NotTo(HaveOccurred())

		testPrimaryKeyPem, testPrimaryCertPem, err := installer.GenerateServerCertificate(
			testCAKeyPem, testCACertPem,
			"postgres-primary",
			[]string{"10.0.0.5"},
		)
		Expect(err).NotTo(HaveOccurred())

		testReplicaKeyPem, testReplicaCertPem, err := installer.GenerateServerCertificate(
			testCAKeyPem, testCACertPem,
			"postgres-replica",
			[]string{"10.0.0.6"},
		)
		Expect(err).NotTo(HaveOccurred())

		initialConfig = fmt.Sprintf(`dataCenter:
  id: 1
  name: test-dc
  city: Berlin
  countryCode: DE
secrets:
  baseDir: /root/secrets
postgres:
  mode: install
  caCertPem: %s
  primary:
    ip: 10.0.0.5
    hostname: postgres-primary
    sslConfig:
      serverCertPem: %s
  replica:
    ip: 10.0.0.6
    name: postgres-replica
    sslConfig:
      serverCertPem: %s
ceph:
  cephAdmSshKey:
    publicKey: ssh-rsa TEST_PUBLIC_KEY
  nodesSubnet: 10.53.101.0/24
  hosts:
    - hostname: ceph-1
      ipAddress: 10.53.101.2
      isMaster: true
  osds: []
kubernetes:
  managedByCodesphere: true
  apiServerHost: 10.0.0.10
  controlPlanes:
    - ipAddress: 10.0.0.10
  workers:
    - ipAddress: 10.0.0.11
cluster:
  certificates:
    ca:
      algorithm: RSA
      keySizeBits: 2048
      certPem: "-----BEGIN CERTIFICATE-----\nCLUSTER_CA_CERT\n-----END CERTIFICATE-----"
  gateway:
    serviceType: LoadBalancer
    ipAddresses:
      - 192.168.1.100
  publicGateway:
    serviceType: LoadBalancer
    ipAddresses:
      - 192.168.1.101
codesphere:
  domain: test.example.com
  workspaceHostingBaseDomain: ws.test.example.com
  publicIp: 203.0.113.1
  customDomains:
    cNameBaseDomain: custom.test.example.com
  dnsServers:
    - 8.8.8.8
    - 8.8.4.4
  experiments: []
  deployConfig:
    images: {}
  plans:
    hostingPlans:
      1:
        cpuTenth: 10
        memoryMb: 2048
        storageMb: 10240
        tempStorageMb: 5120
    workspacePlans:
      1:
        name: Free
        hostingPlanId: 1
        maxReplicas: 1
        onDemand: true
`, quoteYAMLString(testCACertPem), quoteYAMLString(testPrimaryCertPem), quoteYAMLString(testReplicaCertPem))

		initialVault = fmt.Sprintf(`secrets:
  - name: domainAuthPrivateKey
    file:
      name: key.pem
      content: "-----BEGIN EC PRIVATE KEY-----\nDOMAIN_AUTH_PRIVATE_KEY\n-----END EC PRIVATE KEY-----"
  - name: domainAuthPublicKey
    file:
      name: key.pem
      content: "-----BEGIN PUBLIC KEY-----\nDOMAIN_AUTH_PUBLIC_KEY\n-----END PUBLIC KEY-----"
  - name: selfSignedCaKeyPem
    file:
      name: key.pem
      content: "-----BEGIN RSA PRIVATE KEY-----\nINGRESS_CA_PRIVATE_KEY\n-----END RSA PRIVATE KEY-----"
  - name: cephSshPrivateKey
    file:
      name: id_rsa
      content: "-----BEGIN RSA PRIVATE KEY-----\nCEPH_SSH_PRIVATE_KEY\n-----END RSA PRIVATE KEY-----"
  - name: postgresCaKeyPem
    file:
      name: ca.key
      content: %s
  - name: postgresPassword
    fields:
      password: test_admin_password
  - name: postgresPrimaryServerKeyPem
    file:
      name: primary.key
      content: %s
  - name: postgresReplicaPassword
    fields:
      password: test_replica_password
  - name: postgresReplicaServerKeyPem
    file:
      name: replica.key
      content: %s
`, quoteYAMLString(testCAKeyPem), quoteYAMLString(testPrimaryKeyPem), quoteYAMLString(testReplicaKeyPem))

		err = os.WriteFile(configFile.Name(), []byte(initialConfig), 0644)
		Expect(err).NotTo(HaveOccurred())

		err = os.WriteFile(vaultFile.Name(), []byte(initialVault), 0644)
		Expect(err).NotTo(HaveOccurred())

		opts = &UpdateInstallConfigOpts{
			GlobalOptions: &GlobalOptions{},
			ConfigFile:    configFile.Name(),
			VaultFile:     vaultFile.Name(),
		}

		cmd = &UpdateInstallConfigCmd{
			Opts: opts,
		}
	})

	AfterEach(func() {
		_ = os.Remove(configFile.Name())
		_ = os.Remove(vaultFile.Name())
	})

	Context("when updating PostgreSQL configuration", func() {
		It("should update primary IP and hostname, and regenerate certificates", func() {
			opts.PostgresPrimaryIP = "10.10.0.4"
			opts.PostgresPrimaryHostname = "new-postgres-primary"

			icg := installer.NewInstallConfigManager()
			err := cmd.UpdateInstallConfig(icg)
			Expect(err).NotTo(HaveOccurred())

			config := icg.GetInstallConfig()
			Expect(config.Postgres.Primary.IP).To(Equal("10.10.0.4"))
			Expect(config.Postgres.Primary.Hostname).To(Equal("new-postgres-primary"))
			Expect(config.Postgres.Primary.PrivateKey).NotTo(BeEmpty())
			Expect(config.Postgres.Primary.SSLConfig.ServerCertPem).NotTo(BeEmpty())
		})

		It("should update replica IP and name, and regenerate certificates", func() {
			opts.PostgresReplicaIP = "10.10.0.7"
			opts.PostgresReplicaName = "new_replica"

			icg := installer.NewInstallConfigManager()
			err := cmd.UpdateInstallConfig(icg)
			Expect(err).NotTo(HaveOccurred())

			config := icg.GetInstallConfig()
			Expect(config.Postgres.Replica.IP).To(Equal("10.10.0.7"))
			Expect(config.Postgres.Replica.Name).To(Equal("new_replica"))
			Expect(config.Postgres.Replica.PrivateKey).NotTo(BeEmpty())
			Expect(config.Postgres.Replica.SSLConfig.ServerCertPem).NotTo(BeEmpty())
		})
	})

	Context("when updating multiple fields simultaneously", func() {
		It("should update all specified fields and regenerate affected certificates", func() {
			opts.PostgresPrimaryIP = "10.10.0.4"
			opts.PostgresReplicaIP = "10.10.0.7"
			opts.CodesphereDomain = "new.example.com"
			opts.CodespherePublicIP = "203.0.113.100"
			opts.KubernetesPodCIDR = "10.244.0.0/16"

			icg := installer.NewInstallConfigManager()
			err := cmd.UpdateInstallConfig(icg)
			Expect(err).NotTo(HaveOccurred())

			config := icg.GetInstallConfig()
			Expect(config.Postgres.Primary.IP).To(Equal("10.10.0.4"))
			Expect(config.Postgres.Replica.IP).To(Equal("10.10.0.7"))
			Expect(config.Codesphere.Domain).To(Equal("new.example.com"))
			Expect(config.Codesphere.PublicIP).To(Equal("203.0.113.100"))
			Expect(config.Kubernetes.PodCIDR).To(Equal("10.244.0.0/16"))

			Expect(config.Postgres.Primary.PrivateKey).NotTo(BeEmpty())
			Expect(config.Postgres.Replica.PrivateKey).NotTo(BeEmpty())
		})
	})

	Context("when updating Kubernetes configuration", func() {
		It("should update API server host and CIDRs", func() {
			opts.KubernetesAPIServerHost = "10.0.0.20"
			opts.KubernetesPodCIDR = "100.96.0.0/11"
			opts.KubernetesServiceCIDR = "100.64.0.0/13"

			icg := installer.NewInstallConfigManager()
			err := cmd.UpdateInstallConfig(icg)
			Expect(err).NotTo(HaveOccurred())

			config := icg.GetInstallConfig()
			Expect(config.Kubernetes.APIServerHost).To(Equal("10.0.0.20"))
			Expect(config.Kubernetes.PodCIDR).To(Equal("100.96.0.0/11"))
			Expect(config.Kubernetes.ServiceCIDR).To(Equal("100.64.0.0/13"))
		})
	})

	Context("when updating cluster gateway configuration", func() {
		It("should update service type and IP addresses", func() {
			opts.ClusterGatewayServiceType = "NodePort"
			opts.ClusterGatewayIPAddresses = []string{"192.168.1.200", "192.168.1.201"}

			icg := installer.NewInstallConfigManager()
			err := cmd.UpdateInstallConfig(icg)
			Expect(err).NotTo(HaveOccurred())

			config := icg.GetInstallConfig()
			Expect(config.Cluster.Gateway.ServiceType).To(Equal("NodePort"))
			Expect(config.Cluster.Gateway.IPAddresses).To(Equal([]string{"192.168.1.200", "192.168.1.201"}))
		})
	})

	Context("when updating Codesphere configuration", func() {
		It("should update domain, DNS servers, and base domains", func() {
			opts.CodesphereDomain = "updated.example.com"
			opts.CodesphereDNSServers = []string{"1.1.1.1", "1.0.0.1"}
			opts.CodesphereWorkspaceHostingBaseDomain = "workspaces.updated.example.com"

			icg := installer.NewInstallConfigManager()
			err := cmd.UpdateInstallConfig(icg)
			Expect(err).NotTo(HaveOccurred())

			config := icg.GetInstallConfig()
			Expect(config.Codesphere.Domain).To(Equal("updated.example.com"))
			Expect(config.Codesphere.DNSServers).To(Equal([]string{"1.1.1.1", "1.0.0.1"}))
			Expect(config.Codesphere.WorkspaceHostingBaseDomain).To(Equal("workspaces.updated.example.com"))
		})
	})

	Context("when updating Ceph configuration", func() {
		It("should update Ceph nodes subnet", func() {
			opts.CephNodesSubnet = "10.53.102.0/24"

			icg := installer.NewInstallConfigManager()
			err := cmd.UpdateInstallConfig(icg)
			Expect(err).NotTo(HaveOccurred())

			config := icg.GetInstallConfig()
			Expect(config.Ceph.NodesSubnet).To(Equal("10.53.102.0/24"))
		})
	})

	Context("when no changes are made", func() {
		It("should not regenerate any secrets", func() {
			tracker := NewSecretDependencyTracker()
			Expect(tracker.HasChanges()).To(BeFalse())
		})
	})

	Context("when loading invalid config file", func() {
		It("should return an error", func() {
			opts.ConfigFile = "/nonexistent/config.yaml"

			icg := installer.NewInstallConfigManager()
			err := cmd.UpdateInstallConfig(icg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to load config file"))
		})
	})

	Context("when loading invalid vault file", func() {
		It("should return an error", func() {
			opts.VaultFile = "/nonexistent/vault.yaml"

			icg := installer.NewInstallConfigManager()
			err := cmd.UpdateInstallConfig(icg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to load vault file"))
		})
	})

	Context("vault preservation during updates", func() {
		var (
			initialVaultContent []byte
		)

		BeforeEach(func() {
			var err error
			initialVaultContent, err = os.ReadFile(vaultFile.Name())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should preserve all vault entries during non-certificate update", func() {
			initialVault := &files.InstallVault{}
			err := initialVault.Unmarshal(initialVaultContent)
			Expect(err).NotTo(HaveOccurred())
			initialSecrets := make(map[string]files.SecretEntry)
			for _, secret := range initialVault.Secrets {
				initialSecrets[secret.Name] = secret
			}

			opts.CodesphereDomain = "updated.example.com"
			icg := installer.NewInstallConfigManager()
			err = cmd.UpdateInstallConfig(icg)
			Expect(err).NotTo(HaveOccurred())

			updatedVaultContent, err := os.ReadFile(vaultFile.Name())
			Expect(err).NotTo(HaveOccurred())
			updatedVault := &files.InstallVault{}
			err = updatedVault.Unmarshal(updatedVaultContent)
			Expect(err).NotTo(HaveOccurred())

			// Verify all initial secrets are still present with the same values
			for secretName, initialSecret := range initialSecrets {
				found := false
				for _, secret := range updatedVault.Secrets {
					if secret.Name == secretName {
						found = true
						Expect(secret.Fields).To(Equal(initialSecret.Fields), "Secret %s values should be preserved", secretName)
						break
					}
				}
				Expect(found).To(BeTrue(), "Initial secret %s should be preserved after update", secretName)
			}
		})

		It("should preserve all vault entries during certificate regeneration", func() {
			initialVault := &files.InstallVault{}
			err := initialVault.Unmarshal(initialVaultContent)
			Expect(err).NotTo(HaveOccurred())
			initialSecrets := make(map[string]files.SecretEntry)
			for _, secret := range initialVault.Secrets {
				initialSecrets[secret.Name] = secret
			}

			opts.PostgresPrimaryIP = "10.20.0.10"
			icg := installer.NewInstallConfigManager()
			err = cmd.UpdateInstallConfig(icg)
			Expect(err).NotTo(HaveOccurred())

			updatedVaultContent, err := os.ReadFile(vaultFile.Name())
			Expect(err).NotTo(HaveOccurred())
			updatedVault := &files.InstallVault{}
			err = updatedVault.Unmarshal(updatedVaultContent)
			Expect(err).NotTo(HaveOccurred())

			// Verify all initial secrets are still present with the same values
			passwordSecrets := map[string]bool{
				"postgresPassword":        true,
				"postgresReplicaPassword": true,
			}
			for secretName, initialSecret := range initialSecrets {
				found := false
				for _, secret := range updatedVault.Secrets {
					if secret.Name == secretName {
						found = true
						Expect(secret.Fields).To(Equal(initialSecret.Fields), "Secret %s values should be preserved", secretName)

						if passwordSecrets[secretName] {
							Expect(secret.Fields).NotTo(BeNil(), "Secret %s should have fields", secretName)
							Expect(secret.Fields.Password).NotTo(BeEmpty(), "Password for %s should not be empty", secretName)
						}
						break
					}
				}
				Expect(found).To(BeTrue(), "Initial secret %s should be preserved after certificate regeneration", secretName)
			}
		})
	})
})

var _ = Describe("SecretDependencyTracker", func() {
	var tracker *SecretDependencyTracker

	BeforeEach(func() {
		tracker = NewSecretDependencyTracker()
	})

	It("should start with no changes", func() {
		Expect(tracker.HasChanges()).To(BeFalse())
		Expect(tracker.NeedsPostgresPrimaryCertRegen()).To(BeFalse())
		Expect(tracker.NeedsPostgresReplicaCertRegen()).To(BeFalse())
	})

	It("should track primary and replica cert regeneration independently", func() {
		tracker.MarkPostgresPrimaryCertNeedsRegen()
		Expect(tracker.HasChanges()).To(BeTrue())
		Expect(tracker.NeedsPostgresPrimaryCertRegen()).To(BeTrue())
		Expect(tracker.NeedsPostgresReplicaCertRegen()).To(BeFalse())

		tracker.MarkPostgresReplicaCertNeedsRegen()
		Expect(tracker.NeedsPostgresReplicaCertRegen()).To(BeTrue())
	})
})
