// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

func (g *InstallConfig) GenerateSecrets() error {
	fmt.Println("Generating domain authentication keys...")
	var err error
	g.Config.Codesphere.DomainAuthPublicKey, g.Config.Codesphere.DomainAuthPrivateKey, err = GenerateECDSAKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate domain auth keys: %w", err)
	}

	fmt.Println("Generating ingress CA certificate...")
	g.Config.Cluster.IngressCAKey, g.Config.Cluster.Certificates.CA.CertPem, err = GenerateCA("Cluster Ingress CA", "DE", "Karlsruhe", "Codesphere")
	if err != nil {
		return fmt.Errorf("failed to generate ingress CA: %w", err)
	}

	fmt.Println("Generating Ceph SSH keys...")
	g.Config.Ceph.CephAdmSSHKey.PublicKey, g.Config.Ceph.SshPrivateKey, err = GenerateSSHKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate Ceph SSH keys: %w", err)
	}

	if g.Config.Postgres.Primary != nil {
		if err := g.generatePostgresSecrets(g.Config); err != nil {
			return err
		}
	}

	return nil
}

func (g *InstallConfig) generatePostgresSecrets(config *files.RootConfig) error {
	fmt.Println("Generating PostgreSQL certificates and passwords...")
	var err error
	config.Postgres.CaCertPrivateKey, config.Postgres.CACertPem, err = GenerateCA("PostgreSQL CA", "DE", "Karlsruhe", "Codesphere")
	if err != nil {
		return fmt.Errorf("failed to generate PostgreSQL CA: %w", err)
	}

	config.Postgres.Primary.PrivateKey, config.Postgres.Primary.SSLConfig.ServerCertPem, err = GenerateServerCertificate(
		config.Postgres.CaCertPrivateKey,
		config.Postgres.CACertPem,
		config.Postgres.Primary.Hostname,
		[]string{config.Postgres.Primary.IP},
	)
	if err != nil {
		return fmt.Errorf("failed to generate primary PostgreSQL certificate: %w", err)
	}

	config.Postgres.AdminPassword = GeneratePassword(32)
	config.Postgres.ReplicaPassword = GeneratePassword(32)

	if config.Postgres.Replica != nil {
		config.Postgres.Replica.PrivateKey, config.Postgres.Replica.SSLConfig.ServerCertPem, err = GenerateServerCertificate(
			config.Postgres.CaCertPrivateKey,
			config.Postgres.CACertPem,
			config.Postgres.Replica.Name,
			[]string{config.Postgres.Replica.IP},
		)
		if err != nil {
			return fmt.Errorf("failed to generate replica PostgreSQL certificate: %w", err)
		}
	}

	services := []string{"auth", "deployment", "ide", "marketplace", "payment", "public_api", "team", "workspace"}
	config.Postgres.UserPasswords = make(map[string]string)
	for _, service := range services {
		config.Postgres.UserPasswords[service] = GeneratePassword(32)
	}

	return nil
}
