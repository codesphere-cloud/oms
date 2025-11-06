// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

func (g *InstallConfig) generateSecrets(config *files.RootConfig) error {
	fmt.Println("Generating domain authentication keys...")
	domainAuthPub, domainAuthPriv, err := GenerateECDSAKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate domain auth keys: %w", err)
	}
	config.Codesphere.DomainAuthPublicKey = domainAuthPub
	config.Codesphere.DomainAuthPrivateKey = domainAuthPriv

	fmt.Println("Generating ingress CA certificate...")
	ingressCAKey, ingressCACert, err := GenerateCA("Cluster Ingress CA", "DE", "Karlsruhe", "Codesphere")
	if err != nil {
		return fmt.Errorf("failed to generate ingress CA: %w", err)
	}
	config.Cluster.Certificates.CA.CertPem = ingressCACert
	config.Cluster.IngressCAKey = ingressCAKey

	fmt.Println("Generating Ceph SSH keys...")
	cephSSHPub, cephSSHPriv, err := GenerateSSHKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate Ceph SSH keys: %w", err)
	}
	config.Ceph.CephAdmSSHKey.PublicKey = cephSSHPub
	config.Ceph.SshPrivateKey = cephSSHPriv

	if config.Postgres.Primary != nil {
		if err := g.generatePostgresSecrets(config); err != nil {
			return err
		}
	}

	return nil
}

func (g *InstallConfig) generatePostgresSecrets(config *files.RootConfig) error {
	fmt.Println("Generating PostgreSQL certificates and passwords...")

	pgCAKey, pgCACert, err := GenerateCA("PostgreSQL CA", "DE", "Karlsruhe", "Codesphere")
	if err != nil {
		return fmt.Errorf("failed to generate PostgreSQL CA: %w", err)
	}
	config.Postgres.CACertPem = pgCACert
	config.Postgres.CaCertPrivateKey = pgCAKey

	pgPrimaryKey, pgPrimaryCert, err := GenerateServerCertificate(
		pgCAKey,
		pgCACert,
		config.Postgres.Primary.Hostname,
		[]string{config.Postgres.Primary.IP},
	)
	if err != nil {
		return fmt.Errorf("failed to generate primary PostgreSQL certificate: %w", err)
	}
	config.Postgres.Primary.SSLConfig.ServerCertPem = pgPrimaryCert
	config.Postgres.Primary.PrivateKey = pgPrimaryKey

	config.Postgres.AdminPassword = GeneratePassword(32)
	config.Postgres.ReplicaPassword = GeneratePassword(32)

	if config.Postgres.Replica != nil {
		pgReplicaKey, pgReplicaCert, err := GenerateServerCertificate(
			pgCAKey,
			pgCACert,
			config.Postgres.Replica.Name,
			[]string{config.Postgres.Replica.IP},
		)
		if err != nil {
			return fmt.Errorf("failed to generate replica PostgreSQL certificate: %w", err)
		}
		config.Postgres.Replica.SSLConfig.ServerCertPem = pgReplicaCert
		config.Postgres.Replica.PrivateKey = pgReplicaKey
	}

	services := []string{"auth", "deployment", "ide", "marketplace", "payment", "public_api", "team", "workspace"}
	config.Postgres.UserPasswords = make(map[string]string)
	for _, service := range services {
		config.Postgres.UserPasswords[service] = GeneratePassword(32)
	}

	return nil
}
