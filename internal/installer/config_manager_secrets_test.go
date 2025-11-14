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

var _ = Describe("ConfigManagerSecrets", func() {
	var (
		configManager *installer.InstallConfig
	)

	BeforeEach(func() {
		configManager = &installer.InstallConfig{
			Config: &files.RootConfig{},
		}
	})

	Describe("GenerateSecrets", func() {
		Context("with basic configuration", func() {
			BeforeEach(func() {
				configManager.Config = &files.RootConfig{
					Postgres: files.PostgresConfig{
						Primary: &files.PostgresPrimaryConfig{
							IP:       "10.50.0.2",
							Hostname: "pg-primary",
						},
					},
				}
			})

			It("should generate secrets without error", func() {
				err := configManager.GenerateSecrets()
				Expect(err).ToNot(HaveOccurred())
			})

			It("should generate domain auth ECDSA key pair", func() {
				err := configManager.GenerateSecrets()
				Expect(err).ToNot(HaveOccurred())

				Expect(configManager.Config.Codesphere.DomainAuthPublicKey).ToNot(BeEmpty())
				Expect(configManager.Config.Codesphere.DomainAuthPrivateKey).ToNot(BeEmpty())
				Expect(configManager.Config.Codesphere.DomainAuthPublicKey).To(ContainSubstring("BEGIN"))
				Expect(configManager.Config.Codesphere.DomainAuthPrivateKey).To(ContainSubstring("BEGIN"))
			})

			It("should generate Ceph SSH keys", func() {
				err := configManager.GenerateSecrets()
				Expect(err).ToNot(HaveOccurred())

				Expect(configManager.Config.Ceph.CephAdmSSHKey.PublicKey).ToNot(BeEmpty())
				Expect(configManager.Config.Ceph.SshPrivateKey).ToNot(BeEmpty())
			})

			It("should generate PostgreSQL secrets", func() {
				err := configManager.GenerateSecrets()
				Expect(err).ToNot(HaveOccurred())

				Expect(configManager.Config.Postgres.CaCertPrivateKey).ToNot(BeEmpty())
				Expect(configManager.Config.Postgres.CACertPem).ToNot(BeEmpty())
				Expect(configManager.Config.Postgres.AdminPassword).ToNot(BeEmpty())
				Expect(configManager.Config.Postgres.ReplicaPassword).ToNot(BeEmpty())
			})

			It("should generate unique passwords", func() {
				err := configManager.GenerateSecrets()
				Expect(err).ToNot(HaveOccurred())

				adminPwd := configManager.Config.Postgres.AdminPassword
				replicaPwd := configManager.Config.Postgres.ReplicaPassword

				Expect(adminPwd).ToNot(Equal(replicaPwd))
				Expect(adminPwd).To(HaveLen(32))
				Expect(replicaPwd).To(HaveLen(32))
			})
		})

		Context("with PostgreSQL replica configuration", func() {
			BeforeEach(func() {
				configManager.Config = &files.RootConfig{
					Postgres: files.PostgresConfig{
						Primary: &files.PostgresPrimaryConfig{
							IP:       "10.50.0.2",
							Hostname: "pg-primary",
						},
						Replica: &files.PostgresReplicaConfig{
							IP:   "10.50.0.3",
							Name: "replica1",
						},
					},
				}
			})

			It("should generate replica certificates", func() {
				err := configManager.GenerateSecrets()
				Expect(err).ToNot(HaveOccurred())

				Expect(configManager.Config.Postgres.Replica.PrivateKey).ToNot(BeEmpty())
				Expect(configManager.Config.Postgres.Replica.SSLConfig.ServerCertPem).ToNot(BeEmpty())
				Expect(configManager.Config.Postgres.Replica.PrivateKey).To(ContainSubstring("BEGIN RSA PRIVATE KEY"))
				Expect(configManager.Config.Postgres.Replica.SSLConfig.ServerCertPem).To(ContainSubstring("BEGIN CERTIFICATE"))
			})

			It("should generate valid certificate format for primary and replica", func() {
				err := configManager.GenerateSecrets()
				Expect(err).ToNot(HaveOccurred())

				// Primary server certificate
				Expect(strings.HasPrefix(configManager.Config.Postgres.Primary.PrivateKey, "-----BEGIN")).To(BeTrue())
				Expect(strings.HasPrefix(configManager.Config.Postgres.Primary.SSLConfig.ServerCertPem, "-----BEGIN CERTIFICATE-----")).To(BeTrue())

				// Replica server certificate
				Expect(strings.HasPrefix(configManager.Config.Postgres.Replica.PrivateKey, "-----BEGIN")).To(BeTrue())
				Expect(strings.HasPrefix(configManager.Config.Postgres.Replica.SSLConfig.ServerCertPem, "-----BEGIN CERTIFICATE-----")).To(BeTrue())
			})
		})

		Context("without PostgreSQL primary configuration", func() {
			BeforeEach(func() {
				configManager.Config = &files.RootConfig{
					Postgres: files.PostgresConfig{
						Primary: nil,
					},
				}
			})

			It("should not generate PostgreSQL secrets", func() {
				err := configManager.GenerateSecrets()
				Expect(err).ToNot(HaveOccurred())

				Expect(configManager.Config.Postgres.CACertPem).To(BeEmpty())
				Expect(configManager.Config.Postgres.AdminPassword).To(BeEmpty())
			})

			It("should still generate other secrets", func() {
				err := configManager.GenerateSecrets()
				Expect(err).ToNot(HaveOccurred())

				Expect(configManager.Config.Codesphere.DomainAuthPublicKey).ToNot(BeEmpty())
				Expect(configManager.Config.Cluster.IngressCAKey).ToNot(BeEmpty())
				Expect(configManager.Config.Ceph.SshPrivateKey).ToNot(BeEmpty())
			})
		})

		Context("secret generation consistency", func() {
			It("should generate different keys on each invocation", func() {
				config1 := &installer.InstallConfig{
					Config: &files.RootConfig{
						Postgres: files.PostgresConfig{
							Primary: &files.PostgresPrimaryConfig{
								IP:       "10.50.0.2",
								Hostname: "pg-primary",
							},
						},
					},
				}
				config2 := &installer.InstallConfig{
					Config: &files.RootConfig{
						Postgres: files.PostgresConfig{
							Primary: &files.PostgresPrimaryConfig{
								IP:       "10.50.0.2",
								Hostname: "pg-primary",
							},
						},
					},
				}

				err1 := config1.GenerateSecrets()
				err2 := config2.GenerateSecrets()

				Expect(err1).ToNot(HaveOccurred())
				Expect(err2).ToNot(HaveOccurred())

				Expect(config1.Config.Ceph.SshPrivateKey).ToNot(Equal(config2.Config.Ceph.SshPrivateKey))
				Expect(config1.Config.Postgres.AdminPassword).ToNot(Equal(config2.Config.Postgres.AdminPassword))
			})
		})

		Context("CA certificate validation", func() {
			BeforeEach(func() {
				configManager.Config = &files.RootConfig{
					Postgres: files.PostgresConfig{
						Primary: &files.PostgresPrimaryConfig{
							IP:       "10.50.0.2",
							Hostname: "pg-primary",
						},
					},
				}
			})

			It("should generate valid ingress CA with proper PEM format", func() {
				err := configManager.GenerateSecrets()
				Expect(err).ToNot(HaveOccurred())

				// Check CA key is PEM encoded
				Expect(strings.HasPrefix(configManager.Config.Cluster.IngressCAKey, "-----BEGIN")).To(BeTrue())
				Expect(strings.HasSuffix(strings.TrimSpace(configManager.Config.Cluster.IngressCAKey), "-----")).To(BeTrue())

				// Check CA cert is PEM encoded
				Expect(strings.HasPrefix(configManager.Config.Cluster.Certificates.CA.CertPem, "-----BEGIN CERTIFICATE-----")).To(BeTrue())
				Expect(strings.HasSuffix(strings.TrimSpace(configManager.Config.Cluster.Certificates.CA.CertPem), "-----END CERTIFICATE-----")).To(BeTrue())
			})

			It("should generate valid PostgreSQL CA with proper PEM format", func() {
				err := configManager.GenerateSecrets()
				Expect(err).ToNot(HaveOccurred())

				Expect(strings.HasPrefix(configManager.Config.Postgres.CaCertPrivateKey, "-----BEGIN")).To(BeTrue())
				Expect(strings.HasPrefix(configManager.Config.Postgres.CACertPem, "-----BEGIN CERTIFICATE-----")).To(BeTrue())
			})
		})

	})
})
