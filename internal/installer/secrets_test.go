// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ExtractVault", func() {
	It("extracts all secrets from config into vault format", func() {
		config := &InstallConfigContent{
			Postgres: PostgresConfig{
				CACertPem:        "-----BEGIN CERTIFICATE-----\nPG-CA\n-----END CERTIFICATE-----",
				caCertPrivateKey: "-----BEGIN RSA PRIVATE KEY-----\nPG-CA-KEY\n-----END RSA PRIVATE KEY-----",
				adminPassword:    "admin-pass-123",
				replicaPassword:  "replica-pass-456",
				Primary: &PostgresPrimaryConfig{
					SSLConfig: SSLConfig{
						ServerCertPem: "-----BEGIN CERTIFICATE-----\nPG-PRIMARY\n-----END CERTIFICATE-----",
					},
					IP:         "10.50.0.2",
					Hostname:   "pg-primary",
					privateKey: "-----BEGIN RSA PRIVATE KEY-----\nPG-PRIMARY-KEY\n-----END RSA PRIVATE KEY-----",
				},
				Replica: &PostgresReplicaConfig{
					IP:   "10.50.0.3",
					Name: "replica1",
					SSLConfig: SSLConfig{
						ServerCertPem: "-----BEGIN CERTIFICATE-----\nPG-REPLICA\n-----END CERTIFICATE-----",
					},
					privateKey: "-----BEGIN RSA PRIVATE KEY-----\nPG-REPLICA-KEY\n-----END RSA PRIVATE KEY-----",
				},
				userPasswords: map[string]string{
					"auth":       "auth-pass",
					"deployment": "deploy-pass",
				},
			},
			Ceph: CephConfig{
				sshPrivateKey: "-----BEGIN RSA PRIVATE KEY-----\nCEPH-SSH\n-----END RSA PRIVATE KEY-----",
			},
			Cluster: ClusterConfig{
				ingressCAKey: "-----BEGIN RSA PRIVATE KEY-----\nINGRESS-CA-KEY\n-----END RSA PRIVATE KEY-----",
			},
			Codesphere: CodesphereConfig{
				domainAuthPrivateKey: "-----BEGIN EC PRIVATE KEY-----\nDOMAIN-AUTH-PRIV\n-----END EC PRIVATE KEY-----",
				domainAuthPublicKey:  "-----BEGIN PUBLIC KEY-----\nDOMAIN-AUTH-PUB\n-----END PUBLIC KEY-----",
			},
			Kubernetes: KubernetesConfig{
				needsKubeConfig: true,
			},
		}

		vault := config.ExtractVault()

		Expect(vault.Secrets).NotTo(BeEmpty())

		domainAuthPrivFound := false
		domainAuthPubFound := false
		for _, secret := range vault.Secrets {
			if secret.Name == "domainAuthPrivateKey" {
				domainAuthPrivFound = true
				Expect(secret.File).NotTo(BeNil())
				Expect(secret.File.Content).To(ContainSubstring("DOMAIN-AUTH-PRIV"))
			}
			if secret.Name == "domainAuthPublicKey" {
				domainAuthPubFound = true
				Expect(secret.File).NotTo(BeNil())
				Expect(secret.File.Content).To(ContainSubstring("DOMAIN-AUTH-PUB"))
			}
		}
		Expect(domainAuthPrivFound).To(BeTrue())
		Expect(domainAuthPubFound).To(BeTrue())

		ingressCAFound := false
		for _, secret := range vault.Secrets {
			if secret.Name == "selfSignedCaKeyPem" {
				ingressCAFound = true
				Expect(secret.File.Content).To(ContainSubstring("INGRESS-CA-KEY"))
			}
		}
		Expect(ingressCAFound).To(BeTrue())

		cephSSHFound := false
		for _, secret := range vault.Secrets {
			if secret.Name == "cephSshPrivateKey" {
				cephSSHFound = true
				Expect(secret.File.Content).To(ContainSubstring("CEPH-SSH"))
			}
		}
		Expect(cephSSHFound).To(BeTrue())

		pgPasswordFound := false
		pgUserPassFound := false
		for _, secret := range vault.Secrets {
			if secret.Name == "postgresPassword" {
				pgPasswordFound = true
				Expect(secret.Fields.Password).To(Equal("admin-pass-123"))
			}
			if len(secret.Name) > len("postgresPassword") && secret.Name[:16] == "postgresPassword" && secret.Name != "postgresPassword" {
				pgUserPassFound = true
			}
		}
		Expect(pgPasswordFound).To(BeTrue())
		Expect(pgUserPassFound).To(BeTrue())

		kubeConfigFound := false
		for _, secret := range vault.Secrets {
			if secret.Name == "kubeConfig" {
				kubeConfigFound = true
			}
		}
		Expect(kubeConfigFound).To(BeTrue())
	})

	It("does not include kubeconfig for managed k8s", func() {
		config := &InstallConfigContent{
			Kubernetes: KubernetesConfig{
				needsKubeConfig: false,
			},
			Codesphere: CodesphereConfig{
				domainAuthPrivateKey: "test-key",
				domainAuthPublicKey:  "test-pub",
			},
		}

		vault := config.ExtractVault()

		kubeConfigFound := false
		for _, secret := range vault.Secrets {
			if secret.Name == "kubeConfig" {
				kubeConfigFound = true
			}
		}
		Expect(kubeConfigFound).To(BeFalse())
	})

	It("handles all postgres service passwords", func() {
		services := []string{"auth", "deployment", "ide", "marketplace", "payment", "public_api", "team", "workspace"}
		userPasswords := make(map[string]string)
		for _, service := range services {
			userPasswords[service] = service + "-pass"
		}

		config := &InstallConfigContent{
			Postgres: PostgresConfig{
				Primary:       &PostgresPrimaryConfig{},
				userPasswords: userPasswords,
			},
			Codesphere: CodesphereConfig{
				domainAuthPrivateKey: "test",
				domainAuthPublicKey:  "test",
			},
		}

		vault := config.ExtractVault()

		for _, service := range services {
			foundUser := false
			foundPass := false
			for _, secret := range vault.Secrets {
				if secret.Name == "postgresUser"+Capitalize(service) {
					foundUser = true
				}
				if secret.Name == "postgresPassword"+Capitalize(service) {
					foundPass = true
					Expect(secret.Fields.Password).To(Equal(service + "-pass"))
				}
			}
			Expect(foundUser).To(BeTrue(), "User secret for service %s not found", service)
			Expect(foundPass).To(BeTrue(), "Password secret for service %s not found", service)
		}
	})
})
