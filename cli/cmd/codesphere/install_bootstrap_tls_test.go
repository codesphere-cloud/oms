// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package codesphere provides commands for installing Codesphere.
package codesphere

import (
	"os"
	"strings"

	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"go.yaml.in/yaml/v4"
	"k8s.io/client-go/util/cert"
)

var _ = Describe("bootstrap TLS install config compatibility", func() {
	It("repairs the old key template and passes the Codesphere secret-reference validation", func() {
		fw := util.NewMockFileIO(GinkgoT())
		store := installer.NewMockBootstrapTLSVault(GinkgoT())
		runner := installer.NewMockCertbotRunner(GinkgoT())
		certificate, key, err := cert.GenerateSelfSignedCertKey("cs.example.com", nil, []string{"*.cs.example.com", "ws.example.com", "*.ws.example.com"})
		Expect(err).NotTo(HaveOccurred())

		config := []byte("codesphere:\n  domain: cs.example.com\n  workspaceHostingBaseDomain: ws.example.com\n  customCertificate:\n    cert: old-certificate\n    key: old-key\n  certIssuer:\n    type: self-signed\n    acme:\n      enabled: false\n      email: test@example.com\n      server: https://acme-staging-v02.api.letsencrypt.org/directory\n")
		secrets := &files.InstallVault{}

		opts := installer.BootstrapTLSOptions{ConfigPath: "config.yaml", DNSProject: "dns-project", CacheDir: "/root/oms-acme"}
		fw.EXPECT().ReadFile(opts.ConfigPath).Return(config, nil)
		fw.EXPECT().MkdirAll(opts.CacheDir, os.FileMode(0700)).Return(nil)
		fw.EXPECT().Chmod(opts.CacheDir, os.FileMode(0700)).Return(nil)
		fw.EXPECT().ReadFile(mock.MatchedBy(func(path string) bool { return strings.HasSuffix(path, "/fullchain.pem") })).Return(certificate, nil)
		fw.EXPECT().ReadFile(mock.MatchedBy(func(path string) bool { return strings.HasSuffix(path, "/privkey.pem") })).Return(key, nil)
		store.EXPECT().Load().Return(secrets, nil)
		store.EXPECT().Save(secrets).Return(nil)
		runner.EXPECT().Run(mock.Anything).Return(nil)

		var written []byte

		fw.EXPECT().Chmod(opts.ConfigPath, os.FileMode(0600)).Return(nil)
		fw.EXPECT().WriteFile(opts.ConfigPath, mock.Anything, os.FileMode(0600)).Run(func(_ string, data []byte, _ os.FileMode) { written = data }).Return(nil)

		Expect(installer.EnsureBootstrapTLS(opts, fw, store, runner)).To(Succeed())
		// Use the installer's marker rendering and validation on the actual bootstrap
		// output, rather than only proving that generic YAML template rendering works.
		marked, err := configtemplating.RenderWithSecretResolver(written, func(string, ...string) (string, error) { return secretRefMarker, nil })
		Expect(err).NotTo(HaveOccurred())

		var raw map[string]any
		Expect(yaml.Unmarshal(marked, &raw)).To(Succeed())
		Expect(codesphereSecretRefs(raw)).To(BeEmpty())
		Expect(string(written)).NotTo(ContainSubstring(string(key)))
		Expect(secrets.GetSecret(files.SecretCustomCertificateKey).File.Content).To(Equal(string(key)))
		rendered, err := configtemplating.RenderInstallConfigTemplate(written, vault.NewVaultTemplatingSecretStore(secrets))
		Expect(err).NotTo(HaveOccurred())

		var cfg files.RootConfig
		Expect(yaml.Unmarshal(rendered, &cfg)).To(Succeed())
		Expect(cfg.Codesphere.CustomCertificate.Cert).To(Equal(string(certificate)))
		Expect(cfg.Codesphere.CustomCertificate.Key).To(BeEmpty())
	})
})
