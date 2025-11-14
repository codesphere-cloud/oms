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

var _ = Describe("Interactive profile usage", func() {
	Context("when using profile with interactive mode", func() {
		It("should use profile values as defaults", func() {
			icg := installer.NewInstallConfigManager()

			// Apply dev profile first (like the command does)
			err := icg.ApplyProfile("dev")
			Expect(err).NotTo(HaveOccurred())

			config := icg.GetInstallConfig()

			// Verify that profile values are set correctly
			Expect(config.Datacenter.ID).To(Equal(1))
			Expect(config.Datacenter.Name).To(Equal("dev"))
			Expect(config.Datacenter.City).To(Equal("Karlsruhe"))
			Expect(config.Datacenter.CountryCode).To(Equal("DE"))

			// Postgres should be set to install mode with localhost
			Expect(config.Postgres.Mode).To(Equal("install"))
			Expect(config.Postgres.Primary).NotTo(BeNil())
			Expect(config.Postgres.Primary.IP).To(Equal("127.0.0.1"))
			Expect(config.Postgres.Primary.Hostname).To(Equal("localhost"))

			// Ceph should be configured for localhost
			Expect(config.Ceph.NodesSubnet).To(Equal("127.0.0.1/32"))
			Expect(config.Ceph.Hosts).To(HaveLen(1))
			Expect(config.Ceph.Hosts[0].Hostname).To(Equal("localhost"))
			Expect(config.Ceph.Hosts[0].IPAddress).To(Equal("127.0.0.1"))
			Expect(config.Ceph.Hosts[0].IsMaster).To(BeTrue())

			// Kubernetes should be managed and use localhost
			Expect(config.Kubernetes.ManagedByCodesphere).To(BeTrue())
			Expect(config.Kubernetes.APIServerHost).To(Equal("127.0.0.1"))
			Expect(config.Kubernetes.ControlPlanes).To(HaveLen(1))
			Expect(config.Kubernetes.ControlPlanes[0].IPAddress).To(Equal("127.0.0.1"))
			Expect(config.Kubernetes.Workers).To(HaveLen(1))
			Expect(config.Kubernetes.Workers[0].IPAddress).To(Equal("127.0.0.1"))

			// Codesphere domain should be set
			Expect(config.Codesphere.Domain).To(Equal("codesphere.local"))
			Expect(config.Codesphere.WorkspaceHostingBaseDomain).To(Equal("ws.local"))
			Expect(config.Codesphere.CustomDomains.CNameBaseDomain).To(Equal("custom.local"))
		})

		It("should allow non-interactive collection to use profile defaults", func() {
			icg := installer.NewInstallConfigManager()

			// Apply dev profile
			err := icg.ApplyProfile("dev")
			Expect(err).NotTo(HaveOccurred())

			// In non-interactive mode, CollectInteractively would use defaults
			// We simulate this by checking that the prompter returns defaults
			// when interactive=false
			prompter := installer.NewPrompter(false)

			// Test that prompter returns defaults when not interactive
			Expect(prompter.String("Test", "default-value")).To(Equal("default-value"))
			Expect(prompter.Int("Test", 42)).To(Equal(42))
			Expect(prompter.Bool("Test", true)).To(BeTrue())
			Expect(prompter.Choice("Test", []string{"a", "b"}, "a")).To(Equal("a"))
			Expect(prompter.StringSlice("Test", []string{"1", "2"})).To(Equal([]string{"1", "2"}))
		})

		It("should generate valid config files with profile", func() {
			configFile, err := os.CreateTemp("", "config-*.yaml")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(configFile.Name())
			configFile.Close()

			vaultFile, err := os.CreateTemp("", "vault-*.yaml")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(vaultFile.Name())
			vaultFile.Close()

			c := &InitInstallConfigCmd{
				Opts: &InitInstallConfigOpts{
					GlobalOptions: &GlobalOptions{},
					ConfigFile:    configFile.Name(),
					VaultFile:     vaultFile.Name(),
					Profile:       "dev",
					Interactive:   false, // Non-interactive to avoid stdin issues
				},
				FileWriter: util.NewFilesystemWriter(),
			}

			icg := installer.NewInstallConfigManager()
			err = c.InitInstallConfig(icg)
			Expect(err).NotTo(HaveOccurred())

			// Verify files were created
			_, err = os.Stat(configFile.Name())
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(vaultFile.Name())
			Expect(err).NotTo(HaveOccurred())

			// Verify config content
			err = icg.LoadInstallConfigFromFile(configFile.Name())
			Expect(err).NotTo(HaveOccurred())

			config := icg.GetInstallConfig()
			Expect(config.Datacenter.Name).To(Equal("dev"))
			Expect(config.Codesphere.Domain).To(Equal("codesphere.local"))
		})
	})

	Context("when using production profile", func() {
		It("should set production-specific defaults", func() {
			icg := installer.NewInstallConfigManager()

			err := icg.ApplyProfile("production")
			Expect(err).NotTo(HaveOccurred())

			config := icg.GetInstallConfig()

			// Verify production-specific values
			Expect(config.Datacenter.Name).To(Equal("production"))
			Expect(config.Postgres.Primary.IP).To(Equal("10.50.0.2"))
			Expect(config.Postgres.Replica).NotTo(BeNil())
			Expect(config.Postgres.Replica.IP).To(Equal("10.50.0.3"))

			// Ceph should have 3 nodes
			Expect(config.Ceph.Hosts).To(HaveLen(3))
			Expect(config.Ceph.Hosts[0].Hostname).To(Equal("ceph-node-0"))
			Expect(config.Ceph.Hosts[0].IPAddress).To(Equal("10.53.101.2"))
			Expect(config.Ceph.Hosts[0].IsMaster).To(BeTrue())

			Expect(config.Ceph.Hosts[1].Hostname).To(Equal("ceph-node-1"))
			Expect(config.Ceph.Hosts[1].IsMaster).To(BeFalse())

			// Kubernetes should have multiple workers
			Expect(config.Kubernetes.ControlPlanes).To(HaveLen(1))
			Expect(config.Kubernetes.Workers).To(HaveLen(3))

			Expect(config.Codesphere.Domain).To(Equal("codesphere.yourcompany.com"))
		})
	})
})
