// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("GenerateSecrets", func() {
	var mgr *installer.InstallConfig

	BeforeEach(func() {
		mgr = &installer.InstallConfig{
			Config: &files.RootConfig{},
			Vault:  &files.InstallVault{},
		}
	})

	Context("with basic configuration (no postgres)", func() {
		It("succeeds", func() {
			Expect(mgr.GenerateSecrets()).To(Succeed())
		})

		It("writes token key pair to vault", func() {
			Expect(mgr.GenerateSecrets()).To(Succeed())

			tokenPriv := mgr.Vault.GetSecret("tokenPrivateKey")
			tokenPub := mgr.Vault.GetSecret("tokenPublicKey")
			Expect(tokenPriv).NotTo(BeNil())
			Expect(tokenPub).NotTo(BeNil())
			Expect(tokenPriv.File.Content).To(ContainSubstring("BEGIN PRIVATE KEY"))
			Expect(tokenPub.File.Content).To(ContainSubstring("BEGIN PUBLIC KEY"))
		})

		It("writes domain auth key pair to vault", func() {
			Expect(mgr.GenerateSecrets()).To(Succeed())

			priv := mgr.Vault.GetSecret("domainAuthPrivateKey")
			pub := mgr.Vault.GetSecret("domainAuthPublicKey")
			Expect(priv).NotTo(BeNil())
			Expect(pub).NotTo(BeNil())
			Expect(priv.File.Content).To(ContainSubstring("BEGIN PRIVATE KEY"))
			Expect(pub.File.Content).To(ContainSubstring("BEGIN PUBLIC KEY"))
		})

		It("writes ingress CA private key to vault and cert PEM to config", func() {
			Expect(mgr.GenerateSecrets()).To(Succeed())

			caKey := mgr.Vault.GetSecret("selfSignedCaKeyPem")
			Expect(caKey).NotTo(BeNil())
			Expect(caKey.File.Content).To(ContainSubstring("BEGIN PRIVATE KEY"))

			Expect(mgr.Config.Cluster.Certificates.CA.CertPem).To(ContainSubstring("BEGIN CERTIFICATE"))
		})

		It("writes Ceph SSH private key to vault and public key to config", func() {
			Expect(mgr.GenerateSecrets()).To(Succeed())

			cephKey := mgr.Vault.GetSecret("cephSshPrivateKey")
			Expect(cephKey).NotTo(BeNil())
			Expect(cephKey.File.Content).To(ContainSubstring("BEGIN RSA PRIVATE KEY"))

			Expect(mgr.Config.Ceph.CephAdmSSHKey.PublicKey).To(ContainSubstring("ssh-rsa"))
		})

		It("does not write postgres secrets when primary is nil", func() {
			Expect(mgr.GenerateSecrets()).To(Succeed())

			Expect(mgr.Vault.GetSecret("postgresPassword")).To(BeNil())
			Expect(mgr.Vault.GetSecret("postgresCaKeyPem")).To(BeNil())
		})
	})

	Context("with postgres primary configuration", func() {
		BeforeEach(func() {
			mgr.Config.Postgres = files.PostgresConfig{
				Primary: &files.PostgresPrimaryConfig{
					IP:       "10.50.0.2",
					Hostname: "pg-primary",
				},
			}
		})

		It("writes postgres CA key to vault and CA cert PEM to config", func() {
			Expect(mgr.GenerateSecrets()).To(Succeed())

			caKey := mgr.Vault.GetSecret("postgresCaKeyPem")
			Expect(caKey).NotTo(BeNil())
			Expect(caKey.File.Content).To(ContainSubstring("BEGIN PRIVATE KEY"))
			Expect(mgr.Config.Postgres.CACertPem).To(ContainSubstring("BEGIN CERTIFICATE"))
		})

		It("writes admin and replica passwords to vault", func() {
			Expect(mgr.GenerateSecrets()).To(Succeed())

			admin := mgr.Vault.GetSecret("postgresPassword")
			replica := mgr.Vault.GetSecret("postgresReplicaPassword")
			Expect(admin).NotTo(BeNil())
			Expect(replica).NotTo(BeNil())
			Expect(admin.Fields.Password).To(HaveLen(32))
			Expect(replica.Fields.Password).To(HaveLen(32))
			Expect(admin.Fields.Password).NotTo(Equal(replica.Fields.Password))
		})

		It("writes primary server key to vault and cert PEM to config", func() {
			Expect(mgr.GenerateSecrets()).To(Succeed())

			primaryKey := mgr.Vault.GetSecret("postgresPrimaryServerKeyPem")
			Expect(primaryKey).NotTo(BeNil())
			Expect(primaryKey.File.Content).To(ContainSubstring("BEGIN PRIVATE KEY"))
			Expect(mgr.Config.Postgres.Primary.SSLConfig.ServerCertPem).To(ContainSubstring("BEGIN CERTIFICATE"))
		})

		It("writes all service passwords to vault", func() {
			Expect(mgr.GenerateSecrets()).To(Succeed())

			for _, service := range []string{"Auth", "Deployment", "Ide", "Marketplace", "Payment", "Publicapi", "Team", "Workspace"} {
				secret := mgr.Vault.GetSecret("postgresPassword" + service)
				Expect(secret).NotTo(BeNil(), "missing postgresPassword%s", service)
				Expect(secret.Fields.Password).To(HaveLen(32))
			}
		})
	})

	Context("with postgres primary and replica", func() {
		BeforeEach(func() {
			mgr.Config.Postgres = files.PostgresConfig{
				Primary: &files.PostgresPrimaryConfig{
					IP:       "10.50.0.2",
					Hostname: "pg-primary",
				},
				Replica: &files.PostgresReplicaConfig{
					IP:   "10.50.0.3",
					Name: "replica1",
				},
			}
		})

		It("writes replica server key to vault and cert PEM to config", func() {
			Expect(mgr.GenerateSecrets()).To(Succeed())

			replicaKey := mgr.Vault.GetSecret("postgresReplicaServerKeyPem")
			Expect(replicaKey).NotTo(BeNil())
			Expect(replicaKey.File.Content).To(ContainSubstring("BEGIN PRIVATE KEY"))
			Expect(mgr.Config.Postgres.Replica.SSLConfig.ServerCertPem).To(ContainSubstring("BEGIN CERTIFICATE"))
		})

		It("generates valid primary and replica certificates signed by the same CA", func() {
			Expect(mgr.GenerateSecrets()).To(Succeed())

			Expect(strings.HasPrefix(mgr.Config.Postgres.Primary.SSLConfig.ServerCertPem, "-----BEGIN CERTIFICATE-----")).To(BeTrue())
			Expect(strings.HasPrefix(mgr.Config.Postgres.Replica.SSLConfig.ServerCertPem, "-----BEGIN CERTIFICATE-----")).To(BeTrue())
		})
	})

	Context("idempotency", func() {
		It("does not regenerate existing secrets on second call", func() {
			Expect(mgr.GenerateSecrets()).To(Succeed())
			firstKey := mgr.Vault.GetSecret("tokenPrivateKey").File.Content
			firstCA := mgr.Vault.GetSecret("selfSignedCaKeyPem").File.Content

			Expect(mgr.GenerateSecrets()).To(Succeed())
			Expect(mgr.Vault.GetSecret("tokenPrivateKey").File.Content).To(Equal(firstKey))
			Expect(mgr.Vault.GetSecret("selfSignedCaKeyPem").File.Content).To(Equal(firstCA))
		})
	})

	Context("uniqueness", func() {
		It("generates different secrets for different instances", func() {
			mgr2 := &installer.InstallConfig{
				Config: &files.RootConfig{},
				Vault:  &files.InstallVault{},
			}
			Expect(mgr.GenerateSecrets()).To(Succeed())
			Expect(mgr2.GenerateSecrets()).To(Succeed())

			Expect(mgr.Vault.GetSecret("cephSshPrivateKey").File.Content).NotTo(
				Equal(mgr2.Vault.GetSecret("cephSshPrivateKey").File.Content))
			Expect(mgr.Vault.GetSecret("selfSignedCaKeyPem").File.Content).NotTo(
				Equal(mgr2.Vault.GetSecret("selfSignedCaKeyPem").File.Content))
		})
	})
})
