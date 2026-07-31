// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package secrets_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
)

// primaryConfig returns a config shaped like the one the GCP bootstrapper generates for the
// primary data center: postgres installed on a dedicated node, so EnsureSecrets produces the
// full postgres secret set.
func primaryConfig() *files.RootConfig {
	return &files.RootConfig{
		Postgres: files.PostgresConfig{
			Mode: "install",
			Primary: &files.PostgresPrimaryConfig{
				IP:       "10.10.0.5",
				Hostname: "postgres",
			},
		},
		Codesphere: files.CodesphereConfig{
			CertIssuer: &files.CertIssuerConfig{
				Type: "acme",
				Acme: &files.ACMEConfig{Enabled: true, EABKeyID: "primary-eab-key"},
			},
		},
	}
}

// secondaryConfig returns a config shaped like the one generated for a secondary data center:
// the shared postgres server is external, so no postgres secrets are regenerated.
func secondaryConfig(caCertPem string) *files.RootConfig {
	return &files.RootConfig{
		Postgres: files.PostgresConfig{
			Mode:          "external",
			ServerAddress: "10.10.0.5",
			CACertPem:     caCertPem,
		},
		Codesphere: files.CodesphereConfig{
			CertIssuer: &files.CertIssuerConfig{
				Type: "acme",
				Acme: &files.ACMEConfig{Enabled: true, EABKeyID: "primary-eab-key"},
			},
		},
	}
}

var _ = Describe("IsDataCenterScopedSecret", func() {
	DescribeTable("classifies vault secrets",
		func(name string, expected bool) {
			Expect(secrets.IsDataCenterScopedSecret(name)).To(Equal(expected))
		},
		Entry("cluster ingress CA key", files.SecretSelfSignedCaKeyPem, true),
		Entry("cephadm ssh key", files.SecretCephSshPrivateKey, true),
		Entry("kubeconfig", files.SecretKubeConfig, true),
		Entry("acme EAB mac key", files.SecretAcmeEabMacKey, true),
		Entry("nix signing key", files.SecretPrivNixSigningKey, true),
		Entry("ceph fs id", "cephFsId", true),
		Entry("cephfs admin", "cephfsAdminCodesphere", true),
		Entry("csi provisioner", "csiRbdProvisioner", true),
		Entry("rgw admin key", "rgwAdminAccessKey", true),

		Entry("postgres admin password", files.SecretPostgresPassword, false),
		Entry("postgres CA key", files.SecretPostgresCaKeyPem, false),
		Entry("postgres primary server key", files.SecretPostgresPrimaryServerKeyPem, false),
		Entry("per-service postgres user", "postgresUserAuth", false),
		Entry("per-service postgres password", "postgresPasswordAuth", false),
		Entry("token signing key", files.SecretTokenPrivateKey, false),
		Entry("domain auth key", files.SecretDomainAuthPrivateKey, false),
		Entry("mounter hmac", files.SecretMounterHmacSecret, false),
		Entry("managed service password encryption key", files.SecretMongoDbPasswordEncryptionKey, false),
		Entry("registry password", files.SecretRegistryPassword, false),
		Entry("ssh workspace proxy host key", files.SecretSshWorkspaceProxyHostKey, false),
	)
})

var _ = Describe("DeriveDataCenterVault", func() {
	var primary *files.InstallVault

	BeforeEach(func() {
		primary = newVault()
		Expect(secrets.EnsureSecrets(primary, primaryConfig())).To(Succeed())
		// Simulate the ceph and kubernetes install steps writing their credentials back.
		primary.SetSecret(files.SecretEntry{Name: "cephFsId", Fields: &files.SecretFields{Password: "fsid-1"}})
		primary.SetSecret(files.SecretEntry{Name: "csiRbdNode", Fields: &files.SecretFields{Password: "csi-1"}})
		primary.SetSecret(files.SecretEntry{Name: "rgwAdminSecretKey", Fields: &files.SecretFields{Password: "rgw-1"}})
		primary.SetSecret(files.SecretEntry{Name: files.SecretKubeConfig, File: &files.SecretFile{Name: "kubeConfig", Content: "dc1-kubeconfig"}})
	})

	It("keeps the secrets that both data centers must share", func() {
		derived := secrets.DeriveDataCenterVault(primary)

		shared := []string{
			files.SecretPostgresPassword,
			files.SecretPostgresReplicaPassword,
			files.SecretPostgresCaKeyPem,
			files.SecretPostgresPrimaryServerKeyPem,
			files.SecretPostgresReplicaServerKeyPem,
			files.SecretTokenPrivateKey,
			files.SecretTokenPublicKey,
			files.SecretDomainAuthPrivateKey,
			files.SecretDomainAuthPublicKey,
			files.SecretMounterHmacSecret,
			files.SecretMongoDbPasswordEncryptionKey,
			files.SecretSshWorkspaceProxyHostKey,
		}
		for _, name := range shared {
			original := primary.GetSecret(name)
			Expect(original).ToNot(BeNil(), "primary vault should contain %s", name)
			Expect(derived.GetSecret(name)).To(Equal(original), "%s should be shared", name)
		}
	})

	It("keeps every per-service postgres role, because the roles live on the shared server", func() {
		derived := secrets.DeriveDataCenterVault(primary)

		for _, svc := range []string{"Auth", "Deployment", "Ide", "Marketplace", "Payment", "PublicApi", "Team", "Workspace"} {
			for _, prefix := range []string{"postgresUser", "postgresPassword"} {
				name := prefix + svc
				if primary.GetSecret(name) == nil {
					continue
				}
				Expect(derived.GetSecret(name)).To(Equal(primary.GetSecret(name)), "%s should be shared", name)
			}
		}
	})

	It("drops the data-center-scoped secrets", func() {
		derived := secrets.DeriveDataCenterVault(primary)

		for _, name := range []string{
			files.SecretSelfSignedCaKeyPem,
			files.SecretCephSshPrivateKey,
			files.SecretKubeConfig,
			"cephFsId",
			"csiRbdNode",
			"rgwAdminSecretKey",
		} {
			Expect(derived.GetSecret(name)).To(BeNil(), "%s should be dropped", name)
		}
	})

	It("does not alias the primary vault", func() {
		derived := secrets.DeriveDataCenterVault(primary)

		derived.GetSecret(files.SecretPostgresPassword).Fields.Password = "mutated"
		Expect(primary.GetSecret(files.SecretPostgresPassword).Fields.Password).ToNot(Equal("mutated"))
	})

	It("returns nil for a nil vault", func() {
		Expect(secrets.DeriveDataCenterVault(nil)).To(BeNil())
	})
})

var _ = Describe("secondary data center secret generation", func() {
	It("shares the database and auth secrets and regenerates the per-cluster ones", func() {
		primaryVault := newVault()
		primaryCfg := primaryConfig()
		Expect(secrets.EnsureSecrets(primaryVault, primaryCfg)).To(Succeed())

		secondaryVault := secrets.DeriveDataCenterVault(primaryVault)
		secondaryCfg := secondaryConfig(primaryCfg.Postgres.CACertPem)
		secrets.ClearDataCenterScopedConfig(secondaryCfg)
		Expect(secrets.EnsureSecrets(secondaryVault, secondaryCfg)).To(Succeed())

		By("keeping the shared postgres and auth credentials byte-identical")
		Expect(secondaryVault.GetSecret(files.SecretPostgresPassword).Fields.Password).
			To(Equal(primaryVault.GetSecret(files.SecretPostgresPassword).Fields.Password))
		Expect(secondaryVault.GetSecret(files.SecretTokenPrivateKey).File.Content).
			To(Equal(primaryVault.GetSecret(files.SecretTokenPrivateKey).File.Content))
		Expect(secondaryVault.GetSecret("postgresPasswordAuth").Fields.Password).
			To(Equal(primaryVault.GetSecret("postgresPasswordAuth").Fields.Password))

		By("regenerating the per-cluster ingress CA and cephadm key")
		Expect(secondaryVault.GetSecret(files.SecretSelfSignedCaKeyPem)).ToNot(BeNil())
		Expect(secondaryVault.GetSecret(files.SecretSelfSignedCaKeyPem).File.Content).
			ToNot(Equal(primaryVault.GetSecret(files.SecretSelfSignedCaKeyPem).File.Content))
		Expect(secondaryVault.GetSecret(files.SecretCephSshPrivateKey)).ToNot(BeNil())
		Expect(secondaryVault.GetSecret(files.SecretCephSshPrivateKey).File.Content).
			ToNot(Equal(primaryVault.GetSecret(files.SecretCephSshPrivateKey).File.Content))

		By("rewriting the config fields paired with the regenerated secrets")
		Expect(secondaryCfg.Cluster.Certificates.CA.CertPem).ToNot(BeEmpty())
		Expect(secondaryCfg.Cluster.Certificates.CA.CertPem).ToNot(Equal(primaryCfg.Cluster.Certificates.CA.CertPem))
		Expect(secondaryCfg.Ceph.CephAdmSSHKey.PublicKey).ToNot(BeEmpty())
		Expect(secondaryCfg.Ceph.CephAdmSSHKey.PublicKey).ToNot(Equal(primaryCfg.Ceph.CephAdmSSHKey.PublicKey))
		Expect(secondaryCfg.Codesphere.CertIssuer.Acme.EABKeyID).To(BeEmpty())

		By("not regenerating the shared postgres CA, since the server is external")
		Expect(secondaryCfg.Postgres.CACertPem).To(Equal(primaryCfg.Postgres.CACertPem))
	})
})
