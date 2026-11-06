// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package codesphere provides commands for installing Codesphere.
package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v4"
	"k8s.io/client-go/util/cert"
)

var _ = Describe("bootstrap TLS install config compatibility", func() {
	It("repairs the old key template and passes the Codesphere secret-reference validation", func() {
		certificate, key, err := cert.GenerateSelfSignedCertKey("cs.example.com", nil, []string{"*.cs.example.com", "ws.example.com", "*.ws.example.com"})
		Expect(err).NotTo(HaveOccurred())

		config := []byte("codesphere:\n  domain: cs.example.com\n  workspaceHostingBaseDomain: ws.example.com\n  customCertificate:\n    cert: old-certificate\n    key: '{{ secret \"customCertificateKey\" \"file.content\" }}'\n  certIssuer:\n    type: self-signed\n    acme:\n      enabled: false\n      email: test@example.com\n      server: https://acme-staging-v02.api.letsencrypt.org/directory\n")
		secrets := &files.InstallVault{}

		var configMap map[string]any
		Expect(yaml.Unmarshal(config, &configMap)).To(Succeed())
		updated, err := installer.ConfigureCustomTLS(configMap, certificate, key, secrets, []string{"cs.example.com", "*.cs.example.com", "ws.example.com", "*.ws.example.com"})
		Expect(err).NotTo(HaveOccurred())
		written, err := yaml.Marshal(updated)
		Expect(err).NotTo(HaveOccurred())
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

		Expect(installer.ResolveLegacyCertificateKey(raw, vault.NewVaultTemplatingSecretStore(secrets))).To(Succeed())
		certificateValues := raw["codesphere"].(map[string]any)["customCertificate"].(map[string]any)
		Expect(certificateValues["key"]).To(Equal(string(key)))
	})
})
