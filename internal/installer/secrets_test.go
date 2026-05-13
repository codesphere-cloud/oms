// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ExtractVault", func() {
	It("extracts all secrets from config into vault format", func() {
		config := &files.RootConfig{
			Postgres: files.PostgresConfig{
				CACertPem:         "-----BEGIN CERTIFICATE-----\nPG-CA\n-----END CERTIFICATE-----",
				CaCertPrivateKey:  "-----BEGIN RSA PRIVATE KEY-----\nPG-CA-KEY\n-----END RSA PRIVATE KEY-----",
				AdminPassword:     "admin-pass-123",
				ReplicaPassword:   "replica-pass-456",
				ReplicaPrivateKey: "-----BEGIN RSA PRIVATE KEY-----\nPG-REPLICA-KEY\n-----END RSA PRIVATE KEY-----",
				Primary: &files.PostgresPrimaryConfig{
					SSLConfig: files.SSLConfig{
						ServerCertPem: "-----BEGIN CERTIFICATE-----\nPG-PRIMARY\n-----END CERTIFICATE-----",
					},
					IP:         "10.50.0.2",
					Hostname:   "pg-primary",
					PrivateKey: "-----BEGIN RSA PRIVATE KEY-----\nPG-PRIMARY-KEY\n-----END RSA PRIVATE KEY-----",
				},
				Replica: &files.PostgresReplicaConfig{
					IP:   "10.50.0.3",
					Name: "replica1",
					SSLConfig: files.SSLConfig{
						ServerCertPem: "-----BEGIN CERTIFICATE-----\nPG-REPLICA\n-----END CERTIFICATE-----",
					},
				},
				UserPasswords: map[string]string{
					"auth":       "auth-pass",
					"deployment": "deploy-pass",
				},
			},
			Ceph: files.CephConfig{
				SshPrivateKey: "-----BEGIN RSA PRIVATE KEY-----\nCEPH-SSH\n-----END RSA PRIVATE KEY-----",
			},
			Cluster: files.ClusterConfig{
				IngressCAKey: "-----BEGIN RSA PRIVATE KEY-----\nINGRESS-CA-KEY\n-----END RSA PRIVATE KEY-----",
			},
			Codesphere: files.CodesphereConfig{
				DomainAuthPrivateKey: "-----BEGIN EC PRIVATE KEY-----\nDOMAIN-AUTH-PRIV\n-----END EC PRIVATE KEY-----",
				DomainAuthPublicKey:  "-----BEGIN PUBLIC KEY-----\nDOMAIN-AUTH-PUB\n-----END PUBLIC KEY-----",
			},
			Kubernetes: files.KubernetesConfig{
				NeedsKubeConfig: true,
			},
			Registry: &files.RegistryConfig{},
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
		config := files.NewRootConfig()
		config.Kubernetes = files.KubernetesConfig{
			NeedsKubeConfig: false,
		}
		config.Codesphere = files.CodesphereConfig{
			DomainAuthPrivateKey: "test-key",
			DomainAuthPublicKey:  "test-pub",
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

		config := files.NewRootConfig()
		config.Postgres = files.PostgresConfig{
			Primary:       &files.PostgresPrimaryConfig{},
			UserPasswords: userPasswords,
		}
		config.Codesphere = files.CodesphereConfig{
			DomainAuthPrivateKey: "test",
			DomainAuthPublicKey:  "test",
		}

		vault := config.ExtractVault()

		for _, service := range services {
			foundUser := false
			foundPass := false
			for _, secret := range vault.Secrets {
				if secret.Name == "postgresUser"+capitalize(service) {
					foundUser = true
				}
				if secret.Name == "postgresPassword"+capitalize(service) {
					foundPass = true
					Expect(secret.Fields.Password).To(Equal(service + "-pass"))
				}
			}
			Expect(foundUser).To(BeTrue(), "User secret for service %s not found", service)
			Expect(foundPass).To(BeTrue(), "Password secret for service %s not found", service)
		}
	})

	It("extracts GitLab secrets into vault", func() {
		config := files.NewRootConfig()
		config.Codesphere = files.CodesphereConfig{
			GitProviders: &files.GitProvidersConfig{
				GitLab: &files.GitProviderConfig{
					OAuth: files.OAuthConfig{
						ClientID:     "gl-client-id",
						ClientSecret: "gl-client-secret",
					},
				},
			},
		}

		vault := config.ExtractVault()

		assertSecretPassword(vault, "gitlabAppClientId", "gl-client-id")
		assertSecretPassword(vault, "gitlabAppClientSecret", "gl-client-secret")
	})

	It("extracts Bitbucket secrets into vault", func() {
		config := files.NewRootConfig()
		config.Codesphere = files.CodesphereConfig{
			GitProviders: &files.GitProvidersConfig{
				Bitbucket: &files.GitProviderConfig{
					OAuth: files.OAuthConfig{
						ClientID:     "bb-client-id",
						ClientSecret: "bb-client-secret",
					},
				},
			},
		}

		vault := config.ExtractVault()

		assertSecretPassword(vault, "bitbucketAppsClientId", "bb-client-id")
		assertSecretPassword(vault, "bitbucketAppsClientSecret", "bb-client-secret")
	})

	It("extracts Azure DevOps secrets into vault", func() {
		config := files.NewRootConfig()
		config.Codesphere = files.CodesphereConfig{
			GitProviders: &files.GitProvidersConfig{
				AzureDevOps: &files.GitProviderConfig{
					OAuth: files.OAuthConfig{
						ClientID:     "az-client-id",
						ClientSecret: "az-client-secret",
					},
				},
			},
		}

		vault := config.ExtractVault()

		assertSecretPassword(vault, "azureDevOpsAppClientId", "az-client-id")
		assertSecretPassword(vault, "azureDevOpsAppClientSecret", "az-client-secret")
	})

	It("does not include git provider secrets when providers are not set", func() {
		config := files.NewRootConfig()
		config.Codesphere = files.CodesphereConfig{}

		vault := config.ExtractVault()

		for _, secret := range vault.Secrets {
			Expect(secret.Name).NotTo(BeElementOf(
				"gitlabAppClientId", "gitlabAppClientSecret",
				"bitbucketAppsClientId", "bitbucketAppsClientSecret",
				"azureDevOpsAppClientId", "azureDevOpsAppClientSecret",
			))
		}
	})

	It("extracts OIDC OAuth secrets into vault", func() {
		config := files.NewRootConfig()
		config.Codesphere = files.CodesphereConfig{
			OAuth: &files.OAuthProvidersConfig{
				Oidc: &files.OidcOAuthProvider{
					ClientID:     "oidc-client-id",
					ClientSecret: "oidc-client-secret",
				},
			},
		}

		vault := config.ExtractVault()

		assertSecretPassword(vault, "oidcClientId", "oidc-client-id")
		assertSecretPassword(vault, "oidcClientSecret", "oidc-client-secret")
	})
})

func assertSecretPassword(vault *files.InstallVault, name string, expectedPassword string) {
	for _, secret := range vault.Secrets {
		if secret.Name == name {
			ExpectWithOffset(1, secret.Fields).NotTo(BeNil(), "Secret %s has no fields", name)
			ExpectWithOffset(1, secret.Fields.Password).To(Equal(expectedPassword), "Secret %s password mismatch", name)
			return
		}
	}
	Fail("Secret " + name + " not found in vault")
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "_", "")
	return strings.ToUpper(s[:1]) + s[1:]
}
