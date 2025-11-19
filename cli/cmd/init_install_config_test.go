// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("ApplyProfile", func() {
	DescribeTable("profile application",
		func(profile string, wantErr bool, checkDatacenterName string) {
			icg := installer.NewInstallConfigManager()

			err := icg.ApplyProfile(profile)
			if wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				config := icg.GetInstallConfig()
				Expect(config.Datacenter.Name).To(Equal(checkDatacenterName))
			}
		},
		Entry("dev profile", "dev", false, "dev"),
		Entry("development profile", "development", false, "dev"),
		Entry("prod profile", "prod", false, "production"),
		Entry("production profile", "production", false, "production"),
		Entry("minimal profile", "minimal", false, "minimal"),
		Entry("invalid profile", "invalid", true, ""),
	)

	Context("dev profile details", func() {
		It("sets correct dev profile configuration", func() {
			icg := installer.NewInstallConfigManager()

			err := icg.ApplyProfile("dev")
			Expect(err).NotTo(HaveOccurred())
			config := icg.GetInstallConfig()
			Expect(config.Datacenter.ID).To(Equal(1))
			Expect(config.Datacenter.Name).To(Equal("dev"))
			Expect(config.Postgres.Mode).To(Equal("install"))
			Expect(config.Kubernetes.ManagedByCodesphere).To(BeTrue())
		})
	})
})

var _ = Describe("ValidateConfig", func() {
	var (
		configFile  *os.File
		vaultFile   *os.File
		validConfig string
		validVault  string
	)

	BeforeEach(func() {
		var err error
		configFile, err = os.CreateTemp("", "config-*.yaml")
		Expect(err).NotTo(HaveOccurred())

		vaultFile, err = os.CreateTemp("", "vault-*.yaml")
		Expect(err).NotTo(HaveOccurred())

		validConfig = `dataCenter:
  id: 1
  name: test
  city: Berlin
  countryCode: DE
secrets:
  baseDir: /root/secrets
postgres:
  mode: external
  serverAddress: postgres.example.com:5432
ceph:
  cephAdmSshKey:
    publicKey: ssh-rsa TEST
  nodesSubnet: 10.53.101.0/24
  hosts:
    - hostname: ceph-1
      ipAddress: 10.53.101.2
      isMaster: true
  osds: []
kubernetes:
  managedByCodesphere: false
  podCidr: 100.96.0.0/11
  serviceCidr: 100.64.0.0/13
cluster:
  certificates:
    ca:
      algorithm: RSA
      keySizeBits: 2048
      certPem: "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----"
  gateway:
    serviceType: LoadBalancer
  publicGateway:
    serviceType: LoadBalancer
codesphere:
  domain: codesphere.example.com
  workspaceHostingBaseDomain: ws.example.com
  publicIp: 1.2.3.4
  customDomains:
    cNameBaseDomain: custom.example.com
  dnsServers:
    - 8.8.8.8
  experiments: []
  deployConfig:
    images:
      ubuntu-24.04:
        name: Ubuntu 24.04
        supportedUntil: "2028-05-31"
        flavors:
          default:
            image:
              bomRef: workspace-agent-24.04
            pool:
              1: 1
  plans:
    hostingPlans:
      1:
        cpuTenth: 10
        gpuParts: 0
        memoryMb: 2048
        storageMb: 20480
        tempStorageMb: 1024
    workspacePlans:
      1:
        name: Standard
        hostingPlanId: 1
        maxReplicas: 3
        onDemand: true
`

		validVault = `secrets:
  - name: cephSshPrivateKey
    file:
      name: id_rsa
      content: "-----BEGIN RSA PRIVATE KEY-----\nTEST\n-----END RSA PRIVATE KEY-----"
  - name: selfSignedCaKeyPem
    file:
      name: key.pem
      content: "-----BEGIN RSA PRIVATE KEY-----\nCA\n-----END RSA PRIVATE KEY-----"
  - name: domainAuthPrivateKey
    file:
      name: key.pem
      content: "-----BEGIN EC PRIVATE KEY-----\nDOMAIN\n-----END EC PRIVATE KEY-----"
  - name: domainAuthPublicKey
    file:
      name: key.pem
      content: "-----BEGIN PUBLIC KEY-----\nDOMAIN-PUB\n-----END PUBLIC KEY-----"
`
	})

	AfterEach(func() {
		_ = os.Remove(configFile.Name())
		_ = os.Remove(vaultFile.Name())
	})

	Context("valid configuration", func() {
		It("validates successfully", func() {
			_, err := configFile.WriteString(validConfig)
			Expect(err).NotTo(HaveOccurred())
			err = configFile.Close()
			Expect(err).NotTo(HaveOccurred())

			_, err = vaultFile.WriteString(validVault)
			Expect(err).NotTo(HaveOccurred())
			err = vaultFile.Close()
			Expect(err).NotTo(HaveOccurred())

			c := &InitInstallConfigCmd{
				Opts: &InitInstallConfigOpts{
					ConfigFile:   configFile.Name(),
					VaultFile:    vaultFile.Name(),
					ValidateOnly: true,
				},
				FileWriter: util.NewFilesystemWriter(),
			}

			icg := installer.NewInstallConfigManager()
			err = c.validateOnly(icg)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("invalid datacenter", func() {
		It("fails validation", func() {
			invalidConfig := `dataCenter:
  id: 0
  name: ""
secrets:
  baseDir: /root/secrets
postgres:
  serverAddress: postgres.example.com:5432
ceph:
  hosts: []
kubernetes:
  managedByCodesphere: true
cluster:
  certificates:
    ca:
      algorithm: RSA
      keySizeBits: 2048
      certPem: "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----"
  gateway:
    serviceType: LoadBalancer
  publicGateway:
    serviceType: LoadBalancer
codesphere:
  domain: ""
  deployConfig:
    images: {}
  plans:
    hostingPlans: {}
    workspacePlans: {}
`

			_, err := configFile.WriteString(invalidConfig)
			Expect(err).NotTo(HaveOccurred())
			err = configFile.Close()
			Expect(err).NotTo(HaveOccurred())

			c := &InitInstallConfigCmd{
				Opts: &InitInstallConfigOpts{
					ConfigFile:   configFile.Name(),
					ValidateOnly: true,
				},
				FileWriter: util.NewFilesystemWriter(),
			}

			icg := installer.NewInstallConfigManager()
			err = c.validateOnly(icg)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("invalid IP address", func() {
		It("fails validation", func() {
			configWithInvalidIP := `dataCenter:
  id: 1
  name: test
  city: Berlin
  countryCode: DE
secrets:
  baseDir: /root/secrets
postgres:
  serverAddress: postgres.example.com:5432
ceph:
  cephAdmSshKey:
    publicKey: ssh-rsa TEST
  nodesSubnet: 10.53.101.0/24
  hosts:
    - hostname: ceph-1
      ipAddress: invalid-ip-address
      isMaster: true
  osds: []
kubernetes:
  managedByCodesphere: true
  controlPlanes:
    - ipAddress: 10.0.0.1
cluster:
  certificates:
    ca:
      algorithm: RSA
      keySizeBits: 2048
      certPem: "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----"
  gateway:
    serviceType: LoadBalancer
  publicGateway:
    serviceType: LoadBalancer
codesphere:
  domain: codesphere.example.com
  deployConfig:
    images: {}
  plans:
    hostingPlans: {}
    workspacePlans: {}
`

			_, err := configFile.WriteString(configWithInvalidIP)
			Expect(err).NotTo(HaveOccurred())
			err = configFile.Close()
			Expect(err).NotTo(HaveOccurred())

			c := &InitInstallConfigCmd{
				Opts: &InitInstallConfigOpts{
					ConfigFile:   configFile.Name(),
					ValidateOnly: true,
				},
				FileWriter: util.NewFilesystemWriter(),
			}

			icg := installer.NewInstallConfigManager()
			err = c.validateOnly(icg)
			Expect(err).To(HaveOccurred())
		})
	})
})
