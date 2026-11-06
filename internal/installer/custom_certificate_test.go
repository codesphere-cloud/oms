// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"time"

	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v4"
)

var _ = Describe("custom certificate configuration", func() {
	var (
		config           map[string]any
		secrets          *files.InstallVault
		certificate, key []byte
		domains          []string
	)

	BeforeEach(func() {
		config = map[string]any{"codesphere": map[string]any{"clusterAdminEmail": `{{ secret "adminEmail" }}`}}
		secrets = &files.InstallVault{}
		domains = []string{"cs.example.com", "*.cs.example.com", "ws.example.com", "*.ws.example.com"}
		certificate, key = bootstrapCertificate(domains)
	})
	It("configures both gateways, preserves unrelated references and stores keys in the vault", func() {
		updated, err := installer.ConfigureCustomTLS(config, certificate, key, secrets, domains)
		Expect(err).NotTo(HaveOccurred())
		data, err := yaml.Marshal(updated)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).NotTo(ContainSubstring(string(key)))
		Expect(string(data)).To(ContainSubstring(`secret "adminEmail"`))
		Expect(secrets.GetSecret(files.SecretCustomCertificateKey).File.Content).To(Equal(string(key)))
		secrets.SetSecret(files.SecretEntry{Name: "adminEmail", Fields: &files.SecretFields{Password: "admin@example.com"}})
		rendered, err := configtemplating.RenderInstallConfigTemplate(data, vault.NewVaultTemplatingSecretStore(secrets))
		Expect(err).NotTo(HaveOccurred())

		var cfg files.RootConfig
		Expect(yaml.Unmarshal(rendered, &cfg)).To(Succeed())
		Expect(cfg.Codesphere.CustomCertificate.Cert).To(Equal(string(certificate)))
		Expect(cfg.Codesphere.CustomCertificate.Key).To(BeEmpty())
		Expect(cfg.Codesphere.CertIssuer.Type).To(Equal(files.CertIssuerTypeSelfSigned))
		Expect(cfg.Codesphere.CertIssuer.Acme.Enabled).To(BeFalse())
		Expect(cfg.Cluster.PublicGateway.Override).To(HaveKey("nginx"))
		Expect(cfg.PcApps).To(HaveKey("applications"))
	})
	It("rejects an incomplete wildcard certificate without changing the vault", func() {
		certificate, key = bootstrapCertificate([]string{"cs.example.com", "ws.example.com"})
		_, err := installer.ConfigureCustomTLS(config, certificate, key, secrets, domains)
		Expect(err).To(MatchError(ContainSubstring("does not cover *.cs.example.com")))
		Expect(secrets.Secrets).To(BeEmpty())
	})
	It("rejects a certificate whose key does not match", func() {
		_, key = bootstrapCertificate([]string{"cs.example.com"})
		_, err := installer.ConfigureCustomTLS(config, certificate, key, secrets, domains)
		Expect(err).To(MatchError(ContainSubstring("invalid custom TLS certificate/key pair")))
	})
	It("requires the additional data center domains", func() {
		_, err := installer.ConfigureCustomTLS(config, certificate, key, secrets, append(domains, "*.2.cs.example.com"))
		Expect(err).To(MatchError(ContainSubstring("does not cover *.2.cs.example.com")))
	})
})

func bootstrapCertificate(domains []string) ([]byte, []byte) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())

	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: domains, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	Expect(err).NotTo(HaveOccurred())
	key, err := x509.MarshalECPrivateKey(privateKey)
	Expect(err).NotTo(HaveOccurred())

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: key})
}
