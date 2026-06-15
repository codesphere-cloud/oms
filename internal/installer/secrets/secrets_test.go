// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package secrets_test

import (
	"encoding/base64"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
)

// newVault returns an empty vault for use in tests.
func newVault() *files.InstallVault {
	return &files.InstallVault{}
}

var _ = Describe("EnsureAuthKeys", func() {
	It("writes RSA token key pair and EC domain-auth key pair to vault", func() {
		vault := newVault()
		Expect(secrets.EnsureAuthKeys(vault)).To(Succeed())

		assertFileSecret(vault, "tokenPrivateKey", "PRIVATE KEY")
		assertFileSecret(vault, "tokenPublicKey", "PUBLIC KEY")
		assertFileSecret(vault, "domainAuthPrivateKey", "EC PRIVATE KEY")
		assertFileSecret(vault, "domainAuthPublicKey", "PUBLIC KEY")
	})

	It("sets the correct file names", func() {
		vault := newVault()
		Expect(secrets.EnsureAuthKeys(vault)).To(Succeed())

		Expect(vault.GetSecret("tokenPrivateKey").File.Name).To(Equal("key.pem"))
		Expect(vault.GetSecret("tokenPublicKey").File.Name).To(Equal("key.pub"))
		Expect(vault.GetSecret("domainAuthPrivateKey").File.Name).To(Equal("key.pem"))
		Expect(vault.GetSecret("domainAuthPublicKey").File.Name).To(Equal("key.pub"))
	})

	It("is idempotent — does not replace existing keys", func() {
		vault := newVault()
		Expect(secrets.EnsureAuthKeys(vault)).To(Succeed())
		originalTokenKey := vault.GetSecret("tokenPrivateKey").File.Content
		originalDomainKey := vault.GetSecret("domainAuthPrivateKey").File.Content

		Expect(secrets.EnsureAuthKeys(vault)).To(Succeed())
		Expect(vault.GetSecret("tokenPrivateKey").File.Content).To(Equal(originalTokenKey))
		Expect(vault.GetSecret("domainAuthPrivateKey").File.Content).To(Equal(originalDomainKey))
	})

	It("generates domain auth keys independently when only token key is pre-populated", func() {
		vault := newVault()
		vault.SetSecret(files.SecretEntry{Name: "tokenPrivateKey", File: &files.SecretFile{Name: "key.pem", Content: "existing"}})
		vault.SetSecret(files.SecretEntry{Name: "tokenPublicKey", File: &files.SecretFile{Name: "key.pub", Content: "existing-pub"}})

		Expect(secrets.EnsureAuthKeys(vault)).To(Succeed())

		Expect(vault.GetSecret("tokenPrivateKey").File.Content).To(Equal("existing"))
		assertFileSecret(vault, "domainAuthPrivateKey", "EC PRIVATE KEY")
		assertFileSecret(vault, "domainAuthPublicKey", "PUBLIC KEY")
	})

	It("generates distinct keys on each fresh invocation", func() {
		vault1, vault2 := newVault(), newVault()
		Expect(secrets.EnsureAuthKeys(vault1)).To(Succeed())
		Expect(secrets.EnsureAuthKeys(vault2)).To(Succeed())

		Expect(vault1.GetSecret("tokenPrivateKey").File.Content).NotTo(
			Equal(vault2.GetSecret("tokenPrivateKey").File.Content))
	})
})

var _ = Describe("EnsureMounterHmacSecret", func() {
	It("creates a 64-character hex secret", func() {
		vault := newVault()
		Expect(secrets.EnsureMounterHmacSecret(vault)).To(Succeed())

		secret := vault.GetSecret("mounterHmacSecret")
		Expect(secret).NotTo(BeNil())
		Expect(secret.Fields).NotTo(BeNil())
		Expect(secret.Fields.Password).To(HaveLen(64))
		Expect(secret.Fields.Password).To(MatchRegexp("^[0-9a-f]+$"))
	})

	It("is idempotent", func() {
		vault := newVault()
		Expect(secrets.EnsureMounterHmacSecret(vault)).To(Succeed())
		original := vault.GetSecret("mounterHmacSecret").Fields.Password

		Expect(secrets.EnsureMounterHmacSecret(vault)).To(Succeed())
		Expect(vault.GetSecret("mounterHmacSecret").Fields.Password).To(Equal(original))
	})

	It("migrates the legacy hmac-secret entry", func() {
		vault := newVault()
		vault.SetSecret(files.SecretEntry{
			Name:   "hmac-secret",
			Fields: &files.SecretFields{Password: "old-hmac-value"},
		})

		Expect(secrets.EnsureMounterHmacSecret(vault)).To(Succeed())

		secret := vault.GetSecret("mounterHmacSecret")
		Expect(secret).NotTo(BeNil())
		Expect(secret.Fields.Password).To(Equal("old-hmac-value"))
	})

	It("does not migrate when mounterHmacSecret already exists", func() {
		vault := newVault()
		vault.SetSecret(files.SecretEntry{
			Name:   "mounterHmacSecret",
			Fields: &files.SecretFields{Password: "current-value"},
		})
		vault.SetSecret(files.SecretEntry{
			Name:   "hmac-secret",
			Fields: &files.SecretFields{Password: "old-value"},
		})

		Expect(secrets.EnsureMounterHmacSecret(vault)).To(Succeed())
		Expect(vault.GetSecret("mounterHmacSecret").Fields.Password).To(Equal("current-value"))
	})
})

var _ = Describe("EnsureNixSigningKeys", func() {
	It("creates priv/pub keys in host:hexKey format", func() {
		vault := newVault()
		Expect(secrets.EnsureNixSigningKeys(vault, "myhost")).To(Succeed())

		priv := vault.GetSecret("privNixSigningKey")
		pub := vault.GetSecret("pubNixSigningKey")
		Expect(priv).NotTo(BeNil())
		Expect(pub).NotTo(BeNil())

		Expect(priv.Fields.Password).To(HavePrefix("myhost:"))
		Expect(pub.Fields.Password).To(HavePrefix("myhost:"))

		privHex := strings.TrimPrefix(priv.Fields.Password, "myhost:")
		pubHex := strings.TrimPrefix(pub.Fields.Password, "myhost:")
		Expect(privHex).To(MatchRegexp("^[0-9a-f]{64}$"))
		Expect(pubHex).To(MatchRegexp("^[0-9a-f]{64}$"))
	})

	It("is idempotent", func() {
		vault := newVault()
		Expect(secrets.EnsureNixSigningKeys(vault, "host")).To(Succeed())
		orig := vault.GetSecret("privNixSigningKey").Fields.Password

		Expect(secrets.EnsureNixSigningKeys(vault, "host")).To(Succeed())
		Expect(vault.GetSecret("privNixSigningKey").Fields.Password).To(Equal(orig))
	})
})

var _ = Describe("EnsureDefaultSecrets", func() {
	It("always overwrites digitalOceanApiToken", func() {
		vault := newVault()
		vault.SetSecret(files.SecretEntry{Name: "digitalOceanApiToken", Fields: &files.SecretFields{Password: "real-token"}})

		Expect(secrets.EnsureDefaultSecrets(vault)).To(Succeed())
		Expect(vault.GetSecret("digitalOceanApiToken").Fields.Password).To(Equal("dummy"))
	})

	It("sets optional passwords to dummy when absent", func() {
		vault := newVault()
		Expect(secrets.EnsureDefaultSecrets(vault)).To(Succeed())

		for _, name := range []string{
			"githubAppsClientId", "githubAppsClientSecret",
			"gitlabAppClientId", "gitlabAppClientSecret",
			"stripeSecretKey", "sendGridApiKey", "openBaoPassword",
		} {
			secret := vault.GetSecret(name)
			Expect(secret).NotTo(BeNil(), "missing %s", name)
			Expect(secret.Fields.Password).To(Equal("dummy"), "wrong value for %s", name)
		}
	})

	It("does not overwrite existing optional passwords", func() {
		vault := newVault()
		vault.SetSecret(files.SecretEntry{Name: "stripeSecretKey", Fields: &files.SecretFields{Password: "real-stripe-key"}})

		Expect(secrets.EnsureDefaultSecrets(vault)).To(Succeed())
		Expect(vault.GetSecret("stripeSecretKey").Fields.Password).To(Equal("real-stripe-key"))
	})

	It("generates a valid base64-encoded AES key for mongoDbPasswordEncryptionKey", func() {
		vault := newVault()
		Expect(secrets.EnsureDefaultSecrets(vault)).To(Succeed())

		secret := vault.GetSecret("mongoDbPasswordEncryptionKey")
		Expect(secret).NotTo(BeNil())
		decoded, err := base64.StdEncoding.DecodeString(secret.Fields.Password)
		Expect(err).NotTo(HaveOccurred())
		// 16 bytes → 32-char hex → base64 → 44 chars (with padding) or 32 hex chars decoded
		Expect(decoded).To(HaveLen(32)) // 16 bytes as hex string = 32 chars
	})

	It("does not overwrite existing mongoDbPasswordEncryptionKey", func() {
		vault := newVault()
		vault.SetSecret(files.SecretEntry{Name: "mongoDbPasswordEncryptionKey", Fields: &files.SecretFields{Password: "existing-key"}})

		Expect(secrets.EnsureDefaultSecrets(vault)).To(Succeed())
		Expect(vault.GetSecret("mongoDbPasswordEncryptionKey").Fields.Password).To(Equal("existing-key"))
	})

	It("sets managedServiceSecrets to a valid JSON array", func() {
		vault := newVault()
		Expect(secrets.EnsureDefaultSecrets(vault)).To(Succeed())

		secret := vault.GetSecret("managedServiceSecrets")
		Expect(secret).NotTo(BeNil())
		Expect(secret.Fields.Password).To(Equal("[]"))
	})

	It("creates a dummy googleCloudAvatarPrivateKey file entry", func() {
		vault := newVault()
		Expect(secrets.EnsureDefaultSecrets(vault)).To(Succeed())

		secret := vault.GetSecret("googleCloudAvatarPrivateKey")
		Expect(secret).NotTo(BeNil())
		Expect(secret.File).NotTo(BeNil())
	})

	It("does not overwrite existing googleCloudAvatarPrivateKey", func() {
		vault := newVault()
		vault.SetSecret(files.SecretEntry{
			Name: "googleCloudAvatarPrivateKey",
			File: &files.SecretFile{Name: "key.json", Content: "real-key-content"},
		})

		Expect(secrets.EnsureDefaultSecrets(vault)).To(Succeed())
		Expect(vault.GetSecret("googleCloudAvatarPrivateKey").File.Content).To(Equal("real-key-content"))
	})
})

var _ = Describe("EnsureIngressCA", func() {
	It("writes CA private key to vault and cert PEM to cluster config", func() {
		vault := newVault()
		cluster := &files.ClusterConfig{}

		Expect(secrets.EnsureIngressCA(vault, cluster)).To(Succeed())

		caKey := vault.GetSecret("selfSignedCaKeyPem")
		Expect(caKey).NotTo(BeNil())
		Expect(caKey.File.Content).To(ContainSubstring("BEGIN RSA PRIVATE KEY"))
		Expect(cluster.Certificates.CA.CertPem).To(ContainSubstring("BEGIN CERTIFICATE"))
	})

	It("is idempotent — does not replace existing CA", func() {
		vault := newVault()
		cluster := &files.ClusterConfig{}
		Expect(secrets.EnsureIngressCA(vault, cluster)).To(Succeed())

		origKey := vault.GetSecret("selfSignedCaKeyPem").File.Content
		origCert := cluster.Certificates.CA.CertPem

		Expect(secrets.EnsureIngressCA(vault, cluster)).To(Succeed())
		Expect(vault.GetSecret("selfSignedCaKeyPem").File.Content).To(Equal(origKey))
		Expect(cluster.Certificates.CA.CertPem).To(Equal(origCert))
	})
})

var _ = Describe("EnsureCephSSHKeys", func() {
	It("writes private key to vault and public key to ceph config", func() {
		vault := newVault()
		ceph := &files.CephConfig{}

		Expect(secrets.EnsureCephSSHKeys(vault, ceph)).To(Succeed())

		privKey := vault.GetSecret("cephSshPrivateKey")
		Expect(privKey).NotTo(BeNil())
		Expect(privKey.File.Content).To(ContainSubstring("BEGIN RSA PRIVATE KEY"))
		Expect(ceph.CephAdmSSHKey.PublicKey).To(ContainSubstring("ssh-rsa"))
	})

	It("is idempotent", func() {
		vault := newVault()
		ceph := &files.CephConfig{}
		Expect(secrets.EnsureCephSSHKeys(vault, ceph)).To(Succeed())

		origKey := vault.GetSecret("cephSshPrivateKey").File.Content
		Expect(secrets.EnsureCephSSHKeys(vault, ceph)).To(Succeed())
		Expect(vault.GetSecret("cephSshPrivateKey").File.Content).To(Equal(origKey))
	})
})

var _ = Describe("EnsurePostgresSecrets", func() {
	var postgres *files.PostgresConfig

	BeforeEach(func() {
		postgres = &files.PostgresConfig{
			Primary: &files.PostgresPrimaryConfig{
				IP:       "10.50.0.2",
				Hostname: "pg-primary",
			},
		}
	})

	It("writes CA key to vault and cert PEM to config", func() {
		vault := newVault()
		Expect(secrets.EnsurePostgresSecrets(vault, postgres)).To(Succeed())

		caKey := vault.GetSecret("postgresCaKeyPem")
		Expect(caKey).NotTo(BeNil())
		Expect(caKey.File.Content).To(ContainSubstring("BEGIN RSA PRIVATE KEY"))
		Expect(postgres.CACertPem).To(ContainSubstring("BEGIN CERTIFICATE"))
	})

	It("writes admin and replica passwords to vault", func() {
		vault := newVault()
		Expect(secrets.EnsurePostgresSecrets(vault, postgres)).To(Succeed())

		admin := vault.GetSecret("postgresPassword")
		replica := vault.GetSecret("postgresReplicaPassword")
		Expect(admin.Fields.Password).To(HaveLen(32))
		Expect(replica.Fields.Password).To(HaveLen(32))
		Expect(admin.Fields.Password).NotTo(Equal(replica.Fields.Password))
	})

	It("writes primary server key to vault and cert PEM to config", func() {
		vault := newVault()
		Expect(secrets.EnsurePostgresSecrets(vault, postgres)).To(Succeed())

		primaryKey := vault.GetSecret("postgresPrimaryServerKeyPem")
		Expect(primaryKey.File.Content).To(ContainSubstring("BEGIN RSA PRIVATE KEY"))
		Expect(postgres.Primary.SSLConfig.ServerCertPem).To(ContainSubstring("BEGIN CERTIFICATE"))
	})

	It("writes all service passwords to vault", func() {
		vault := newVault()
		Expect(secrets.EnsurePostgresSecrets(vault, postgres)).To(Succeed())

		for _, suffix := range []string{
			"Auth", "Deployment", "Ide", "Marketplace", "Payment", "Publicapi", "Team", "Workspace",
			"UsageAggregationRefresher", "UsageAggregationReader",
		} {
			s := vault.GetSecret("postgresPassword" + suffix)
			Expect(s).NotTo(BeNil(), "missing postgresPassword%s", suffix)
			Expect(s.Fields.Password).To(HaveLen(32))
		}
	})

	It("is idempotent", func() {
		vault := newVault()
		Expect(secrets.EnsurePostgresSecrets(vault, postgres)).To(Succeed())
		origPass := vault.GetSecret("postgresPassword").Fields.Password

		Expect(secrets.EnsurePostgresSecrets(vault, postgres)).To(Succeed())
		Expect(vault.GetSecret("postgresPassword").Fields.Password).To(Equal(origPass))
	})

	Context("with replica", func() {
		BeforeEach(func() {
			postgres.Replica = &files.PostgresReplicaConfig{
				IP:   "10.50.0.3",
				Name: "replica1",
			}
		})

		It("writes replica server key to vault and cert PEM to config", func() {
			vault := newVault()
			Expect(secrets.EnsurePostgresSecrets(vault, postgres)).To(Succeed())

			replicaKey := vault.GetSecret("postgresReplicaServerKeyPem")
			Expect(replicaKey).NotTo(BeNil())
			Expect(replicaKey.File.Content).To(ContainSubstring("BEGIN RSA PRIVATE KEY"))
			Expect(postgres.Replica.SSLConfig.ServerCertPem).To(ContainSubstring("BEGIN CERTIFICATE"))
		})

		It("generates primary and replica certs with different keys", func() {
			vault := newVault()
			Expect(secrets.EnsurePostgresSecrets(vault, postgres)).To(Succeed())

			primaryKey := vault.GetSecret("postgresPrimaryServerKeyPem").File.Content
			replicaKey := vault.GetSecret("postgresReplicaServerKeyPem").File.Content
			Expect(primaryKey).NotTo(Equal(replicaKey))
		})
	})
})

var _ = Describe("EnsureServiceAccountTokens", func() {
	var vault *files.InstallVault

	BeforeEach(func() {
		vault = newVault()
		Expect(secrets.EnsureAuthKeys(vault)).To(Succeed())
	})

	It("writes a token for every service account", func() {
		Expect(secrets.EnsureServiceAccountTokens(vault)).To(Succeed())

		for _, name := range []string{
			"authServiceUserToken",
			"paymentServiceUserToken",
			"publicApiServiceUserToken",
			"deploymentServiceUserToken",
			"marketplaceServiceUserToken",
			"errorPageServerUserToken",
			"userDeletionCronJobUserToken",
			"workspaceServiceUserToken",
			"workspaceProxyUserToken",
			"ideServiceUserToken",
		} {
			s := vault.GetSecret(name)
			Expect(s).NotTo(BeNil(), "missing %s", name)
			Expect(s.Fields).NotTo(BeNil(), "%s has no fields", name)
			Expect(s.Fields.Password).NotTo(BeEmpty(), "%s token is empty", name)
		}
	})

	It("tokens are valid RS512 JWTs with correct claims", func() {
		Expect(secrets.EnsureServiceAccountTokens(vault)).To(Succeed())

		tokenStr := vault.GetSecret("authServiceUserToken").Fields.Password
		// Parse without verification to inspect claims.
		tok, _, err := new(jwt.Parser).ParseUnverified(tokenStr, jwt.MapClaims{})
		Expect(err).NotTo(HaveOccurred())
		Expect(tok.Method.Alg()).To(Equal("RS512"))

		claims, ok := tok.Claims.(jwt.MapClaims)
		Expect(ok).To(BeTrue())
		Expect(claims["serviceId"]).To(Equal("auth-service"))
		Expect(claims["authenticationMethod"]).To(Equal("service"))
		Expect(claims["email"]).To(Equal("auth.service@codesphere.com"))
		Expect(claims["userId"]).To(BeNumerically("==", -1))
	})

	It("returns an error when tokenPrivateKey is absent", func() {
		emptyVault := newVault()
		Expect(secrets.EnsureServiceAccountTokens(emptyVault)).To(MatchError(ContainSubstring("tokenPrivateKey")))
	})
})

// assertFileSecret checks that a vault entry with the given name exists as a file secret
// whose content contains the given PEM header.
func assertFileSecret(vault *files.InstallVault, name, pemHeader string) {
	GinkgoHelper()
	secret := vault.GetSecret(name)
	Expect(secret).NotTo(BeNil(), "vault entry %q not found", name)
	Expect(secret.File).NotTo(BeNil(), "vault entry %q has no file content", name)
	Expect(secret.File.Content).To(ContainSubstring("BEGIN "+pemHeader), "vault entry %q has unexpected PEM header", name)
}
