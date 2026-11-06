// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Legacy custom certificate key", func() {
	It("loads the key from the vault when the inline key is empty", func() {
		certificate := map[string]any{"cert": "certificate-pem", "key": ""}
		config := map[string]any{"codesphere": map[string]any{"customCertificate": certificate}}
		secrets := &files.InstallVault{}
		secrets.SetSecret(files.SecretEntry{Name: files.SecretCustomCertificateKey, File: &files.SecretFile{Content: "key-pem"}})
		Expect(installer.ResolveLegacyCertificateKey(config, vault.NewVaultTemplatingSecretStore(secrets))).To(Succeed())
		Expect(certificate).To(Equal(map[string]any{"cert": "certificate-pem", "key": "key-pem"}))
	})

	It("preserves inline keys without accessing the vault", func() {
		certificate := map[string]any{"cert": "certificate-pem", "key": "inline-key"}
		config := map[string]any{"codesphere": map[string]any{"customCertificate": certificate}}
		Expect(installer.ResolveLegacyCertificateKey(config, nil)).To(Succeed())
		Expect(certificate["key"]).To(Equal("inline-key"))
	})

	It("does not access the vault when no custom certificate is configured", func() {
		Expect(installer.ResolveLegacyCertificateKey(map[string]any{}, nil)).To(Succeed())
	})

	It("reports a missing vault key before running the installer", func() {
		config := map[string]any{"codesphere": map[string]any{"customCertificate": map[string]any{"cert": "certificate-pem", "key": ""}}}
		Expect(installer.ResolveLegacyCertificateKey(config, vault.NewVaultTemplatingSecretStore(&files.InstallVault{}))).To(MatchError(ContainSubstring("resolving custom certificate private key")))
	})
})
