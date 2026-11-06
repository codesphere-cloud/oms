// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/cert"
)

var _ = Describe("supplied bootstrap certificates", func() {
	var (
		b                *GCPBootstrapper
		config           files.RootConfig
		secrets          *files.InstallVault
		certificate, key []byte
		domains          []string
	)

	BeforeEach(func() {
		b = &GCPBootstrapper{Env: &CodesphereEnvironment{}}
		domains = []string{"cs.example.com", "*.cs.example.com", "ws.example.com", "*.ws.example.com"}

		var err error

		certificate, key, err = cert.GenerateSelfSignedCertKey(domains[0], nil, domains[1:])
		Expect(err).NotTo(HaveOccurred())

		config.Codesphere.CustomCertificate = &files.CustomCertificate{Cert: string(certificate), Key: string(key)}
		secrets = &files.InstallVault{}
	})
	It("preserves an inline certificate without requiring ACME settings or a jumpbox", func() {
		actual, actualKey, err := b.installCertificate(config, secrets, domains)
		Expect(err).NotTo(HaveOccurred())
		Expect(actual).To(Equal(certificate))
		Expect(actualKey).To(Equal(key))
	})
	It("reuses a vault-backed certificate on retries without issuing a replacement", func() {
		config.Codesphere.CustomCertificate.Key = ""

		secrets.SetSecret(files.SecretEntry{Name: files.SecretCustomCertificateKey, File: &files.SecretFile{Content: string(key)}})
		actual, actualKey, err := b.installCertificate(config, secrets, domains)
		Expect(err).NotTo(HaveOccurred())
		Expect(actual).To(Equal(certificate))
		Expect(actualKey).To(Equal(key))
	})
	It("rejects a supplied certificate with a missing key rather than issuing a replacement", func() {
		config.Codesphere.CustomCertificate.Key = ""
		actual, actualKey, err := b.installCertificate(config, secrets, domains)
		Expect(err).NotTo(HaveOccurred())
		_, err = installer.ConfigureCustomTLS(nil, actual, actualKey, secrets, domains)
		Expect(err).To(MatchError(ContainSubstring("invalid custom TLS certificate/key pair")))
		Expect(secrets.Secrets).To(BeEmpty())
	})
})
