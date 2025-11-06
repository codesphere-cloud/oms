// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"strings"
)

func (g *InstallConfig) generateSecrets(config *InstallConfigContent) error {
	fmt.Println("Generating domain authentication keys...")
	domainAuthPub, domainAuthPriv, err := GenerateECDSAKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate domain auth keys: %w", err)
	}
	config.Codesphere.domainAuthPublicKey = domainAuthPub
	config.Codesphere.domainAuthPrivateKey = domainAuthPriv

	fmt.Println("Generating ingress CA certificate...")
	ingressCAKey, ingressCACert, err := GenerateCA("Cluster Ingress CA", "DE", "Karlsruhe", "Codesphere")
	if err != nil {
		return fmt.Errorf("failed to generate ingress CA: %w", err)
	}
	config.Cluster.Certificates.CA.CertPem = ingressCACert
	config.Cluster.ingressCAKey = ingressCAKey

	fmt.Println("Generating Ceph SSH keys...")
	cephSSHPub, cephSSHPriv, err := GenerateSSHKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate Ceph SSH keys: %w", err)
	}
	config.Ceph.CephAdmSSHKey.PublicKey = cephSSHPub
	config.Ceph.sshPrivateKey = cephSSHPriv

	if config.Postgres.Primary != nil {
		if err := g.generatePostgresSecrets(config); err != nil {
			return err
		}
	}

	return nil
}

func (g *InstallConfig) generatePostgresSecrets(config *InstallConfigContent) error {
	fmt.Println("Generating PostgreSQL certificates and passwords...")

	pgCAKey, pgCACert, err := GenerateCA("PostgreSQL CA", "DE", "Karlsruhe", "Codesphere")
	if err != nil {
		return fmt.Errorf("failed to generate PostgreSQL CA: %w", err)
	}
	config.Postgres.CACertPem = pgCACert
	config.Postgres.caCertPrivateKey = pgCAKey

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
	config.Postgres.Primary.privateKey = pgPrimaryKey

	config.Postgres.adminPassword = GeneratePassword(32)
	config.Postgres.replicaPassword = GeneratePassword(32)

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
		config.Postgres.Replica.privateKey = pgReplicaKey
	}

	services := []string{"auth", "deployment", "ide", "marketplace", "payment", "public_api", "team", "workspace"}
	config.Postgres.userPasswords = make(map[string]string)
	for _, service := range services {
		config.Postgres.userPasswords[service] = GeneratePassword(32)
	}

	return nil
}

func (c *InstallConfigContent) ExtractVault() *InstallVault {
	vault := &InstallVault{
		Secrets: []SecretEntry{},
	}

	c.addCodesphereSecrets(vault)
	c.addIngressCASecret(vault)
	c.addCephSecrets(vault)
	c.addPostgresSecrets(vault)
	c.addManagedServiceSecrets(vault)
	c.addRegistrySecrets(vault)
	c.addKubeConfigSecret(vault)

	return vault
}

func (c *InstallConfigContent) addCodesphereSecrets(vault *InstallVault) {
	if c.Codesphere.domainAuthPrivateKey != "" {
		vault.Secrets = append(vault.Secrets,
			SecretEntry{
				Name: "domainAuthPrivateKey",
				File: &SecretFile{
					Name:    "key.pem",
					Content: c.Codesphere.domainAuthPrivateKey,
				},
			},
			SecretEntry{
				Name: "domainAuthPublicKey",
				File: &SecretFile{
					Name:    "key.pem",
					Content: c.Codesphere.domainAuthPublicKey,
				},
			},
		)
	}
}

func (c *InstallConfigContent) addIngressCASecret(vault *InstallVault) {
	if c.Cluster.ingressCAKey != "" {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "selfSignedCaKeyPem",
			File: &SecretFile{
				Name:    "key.pem",
				Content: c.Cluster.ingressCAKey,
			},
		})
	}
}

func (c *InstallConfigContent) addCephSecrets(vault *InstallVault) {
	if c.Ceph.sshPrivateKey != "" {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "cephSshPrivateKey",
			File: &SecretFile{
				Name:    "id_rsa",
				Content: c.Ceph.sshPrivateKey,
			},
		})
	}
}

func (c *InstallConfigContent) addPostgresSecrets(vault *InstallVault) {
	if c.Postgres.Primary == nil {
		return
	}

	if c.Postgres.adminPassword != "" {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "postgresPassword",
			Fields: &SecretFields{
				Password: c.Postgres.adminPassword,
			},
		})
	}

	if c.Postgres.Primary.privateKey != "" {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "postgresPrimaryServerKeyPem",
			File: &SecretFile{
				Name:    "primary.key",
				Content: c.Postgres.Primary.privateKey,
			},
		})
	}

	if c.Postgres.Replica != nil {
		if c.Postgres.replicaPassword != "" {
			vault.Secrets = append(vault.Secrets, SecretEntry{
				Name: "postgresReplicaPassword",
				Fields: &SecretFields{
					Password: c.Postgres.replicaPassword,
				},
			})
		}

		if c.Postgres.Replica.privateKey != "" {
			vault.Secrets = append(vault.Secrets, SecretEntry{
				Name: "postgresReplicaServerKeyPem",
				File: &SecretFile{
					Name:    "replica.key",
					Content: c.Postgres.Replica.privateKey,
				},
			})
		}
	}

	services := []string{"auth", "deployment", "ide", "marketplace", "payment", "public_api", "team", "workspace"}
	for _, service := range services {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: fmt.Sprintf("postgresUser%s", Capitalize(service)),
			Fields: &SecretFields{
				Password: service + "_blue",
			},
		})
		if password, ok := c.Postgres.userPasswords[service]; ok {
			vault.Secrets = append(vault.Secrets, SecretEntry{
				Name: fmt.Sprintf("postgresPassword%s", Capitalize(service)),
				Fields: &SecretFields{
					Password: password,
				},
			})
		}
	}
}

func (c *InstallConfigContent) addManagedServiceSecrets(vault *InstallVault) {
	vault.Secrets = append(vault.Secrets, SecretEntry{
		Name: "managedServiceSecrets",
		Fields: &SecretFields{
			Password: "[]",
		},
	})
}

func (c *InstallConfigContent) addRegistrySecrets(vault *InstallVault) {
	if c.Registry != nil {
		vault.Secrets = append(vault.Secrets,
			SecretEntry{
				Name: "registryUsername",
				Fields: &SecretFields{
					Password: "YOUR_REGISTRY_USERNAME",
				},
			},
			SecretEntry{
				Name: "registryPassword",
				Fields: &SecretFields{
					Password: "YOUR_REGISTRY_PASSWORD",
				},
			},
		)
	}
}

func (c *InstallConfigContent) addKubeConfigSecret(vault *InstallVault) {
	if c.Kubernetes.needsKubeConfig {
		vault.Secrets = append(vault.Secrets, SecretEntry{
			Name: "kubeConfig",
			File: &SecretFile{
				Name:    "kubeConfig",
				Content: "# YOUR KUBECONFIG CONTENT HERE\n# Replace this with your actual kubeconfig for the external cluster\n",
			},
		})
	}
}

func Capitalize(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "_", "")
	return strings.ToUpper(s[:1]) + s[1:]
}
