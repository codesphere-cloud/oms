// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"go.yaml.in/yaml/v4"
)

var _ = Describe("jumpbox bootstrap TLS", func() {
	var (
		fw                *util.MockFileIO
		store             *installer.MockBootstrapTLSVault
		runner            *installer.MockCertbotRunner
		opts              installer.BootstrapTLSOptions
		secrets           *files.InstallVault
		config, cert, key []byte
		written           []byte
		arguments         []string
	)

	BeforeEach(func() {
		fw = util.NewMockFileIO(GinkgoT())
		store = installer.NewMockBootstrapTLSVault(GinkgoT())
		runner = installer.NewMockCertbotRunner(GinkgoT())
		opts = installer.BootstrapTLSOptions{ConfigPath: "config.yaml", DNSProject: "dns-project", CacheDir: "/root/oms-acme"}
		secrets = &files.InstallVault{}
		config = []byte(`codesphere:
  domain: cs.example.com
  workspaceHostingBaseDomain: ws.example.com
  clusterAdminEmail: '{{ secret "adminEmail" }}'
  certIssuer:
    type: acme
    acme:
      enabled: true
      email: test@example.com
      server: https://acme-staging-v02.api.letsencrypt.org/directory
`)
		cert, key = bootstrapCertificate([]string{"cs.example.com", "*.cs.example.com", "ws.example.com", "*.ws.example.com"})
		written = nil
		arguments = nil

		fw.EXPECT().ReadFile("config.yaml").RunAndReturn(func(string) ([]byte, error) { return config, nil })
		store.EXPECT().Load().Return(secrets, nil).Maybe()
	})

	expectSave := func() {
		store.EXPECT().Save(secrets).Return(nil)
		fw.EXPECT().Chmod(opts.ConfigPath, os.FileMode(0600)).Return(nil)
		fw.EXPECT().WriteFile(opts.ConfigPath, mock.Anything, os.FileMode(0600)).Run(func(_ string, b []byte, _ os.FileMode) { written = b }).Return(nil)
	}
	expectIssue := func() {
		runner.EXPECT().Issue(mock.Anything, opts.CacheDir, mock.Anything).RunAndReturn(func(args []string, _, _ string) ([]byte, []byte, error) { arguments = args; return cert, key, nil })
	}

	It("issues wildcard certificates using jumpbox ADC and configures both gateways without leaking keys", func() {
		expectIssue()
		expectSave()
		Expect(installer.EnsureBootstrapTLS(opts, fw, store, runner)).To(Succeed())
		Expect(arguments).To(ContainElements("--network", "host", "--dns-google", "--dns-google-project", "dns-project", "--keep-until-expiring", "https://acme-staging-v02.api.letsencrypt.org/directory", "*.cs.example.com", "*.ws.example.com"))
		Expect(string(written)).NotTo(ContainSubstring(string(key)))
		Expect(string(written)).To(ContainSubstring(`secret "adminEmail"`))
		Expect(secrets.GetSecret(files.SecretCustomCertificateKey).File.Content).To(Equal(string(key)))
		// The persisted config must be valid YAML even before resolving templates, so
		// jumpbox retries can read it while private keys remain encrypted.
		var raw map[string]any
		Expect(yaml.Unmarshal(written, &raw)).To(Succeed())
		secrets.SetSecret(files.SecretEntry{Name: "adminEmail", Fields: &files.SecretFields{Password: "admin@example.com"}})
		rendered, err := configtemplating.RenderInstallConfigTemplate(written, vault.NewVaultTemplatingSecretStore(secrets))
		Expect(err).NotTo(HaveOccurred())

		var cfg files.RootConfig
		Expect(yaml.Unmarshal(rendered, &cfg)).To(Succeed())
		Expect(cfg.Codesphere.CustomCertificate.Key).To(BeEmpty())
		Expect(cfg.Codesphere.CustomCertificate.Cert).To(Equal(string(cert)))
		Expect(cfg.Codesphere.CertIssuer.Type).To(Equal(files.CertIssuerTypeSelfSigned))
		Expect(cfg.Codesphere.CertIssuer.Acme.Enabled).To(BeFalse())
		Expect(cfg.Cluster.PublicGateway.Override).To(HaveKey("nginx"))
		Expect(cfg.PcApps).To(HaveKey("applications"))
	})
	It("reuses the same cached certificate and accepts its own templated config on retries", func() {
		expectIssue()
		expectSave()
		Expect(installer.EnsureBootstrapTLS(opts, fw, store, runner)).To(Succeed())

		firstArgs := append([]string{}, arguments...)
		config = written

		Expect(installer.EnsureBootstrapTLS(opts, fw, store, runner)).To(Succeed())
		Expect(arguments).To(Equal(firstArgs))
	})
	It("passes Google Public CA external account credentials from the vault", func() {
		config = append(config, []byte("      eabKeyId: account-id\n")...)

		secrets.SetSecret(files.SecretEntry{Name: files.SecretAcmeEabMacKey, Fields: &files.SecretFields{Password: "eab-mac-key"}})
		expectIssue()
		expectSave()
		Expect(installer.EnsureBootstrapTLS(opts, fw, store, runner)).To(Succeed())
		Expect(arguments).To(ContainElements("--eab-kid", "account-id", "--eab-hmac-key", "eab-mac-key"))
		Expect(string(written)).NotTo(ContainSubstring("eab-mac-key"))
	})
	It("does not modify config or vault if issuance fails", func() {
		runner.EXPECT().Issue(mock.Anything, opts.CacheDir, mock.Anything).Return(nil, nil, fmt.Errorf("DNS validation failed"))
		Expect(installer.EnsureBootstrapTLS(opts, fw, store, runner)).To(MatchError(ContainSubstring("DNS validation failed")))
	})
	It("rejects an incomplete wildcard certificate before saving anything", func() {
		cert, key = bootstrapCertificate([]string{"cs.example.com", "ws.example.com"})

		expectIssue()
		Expect(installer.EnsureBootstrapTLS(opts, fw, store, runner)).To(MatchError(ContainSubstring("does not cover *.cs.example.com")))
	})
	It("rejects a certificate whose private key does not match", func() {
		_, key = bootstrapCertificate([]string{"cs.example.com"})

		expectIssue()
		Expect(installer.EnsureBootstrapTLS(opts, fw, store, runner)).To(MatchError(ContainSubstring("invalid bootstrap TLS certificate/key pair")))
	})
	It("requires extra data center domains on the certificate", func() {
		opts.Domains = []string{"2.cs.example.com", "*.2.cs.example.com"}

		expectIssue()
		Expect(installer.EnsureBootstrapTLS(opts, fw, store, runner)).To(MatchError(ContainSubstring("does not cover *.2.cs.example.com")))
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
