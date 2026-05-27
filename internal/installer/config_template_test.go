// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Config templating", func() {
	var vault *files.InstallVault

	BeforeEach(func() {
		vault = &files.InstallVault{
			Secrets: []files.SecretEntry{
				{
					Name: "apiToken",
					Fields: &files.SecretFields{
						Username: "codesphere",
						Password: "super-secret-token",
					},
				},
				{
					Name: "caPem",
					File: &files.SecretFile{
						Name:    "ca.pem",
						Content: "-----BEGIN CERTIFICATE-----\nsecret\n-----END CERTIFICATE-----",
					},
				},
			},
		}
	})

	It("renders secret values from the vault dynamically", func() {
		rendered, err := configtemplating.RenderInstallConfigTemplate(
			[]byte(`password: "{{ secret "apiToken" }}"`),
			installer.NewVaultTemplatingSecretStore(vault),
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(string(rendered)).To(Equal(`password: "super-secret-token"`))
	})

	It("supports selecting specific secret fields", func() {
		rendered, err := configtemplating.RenderInstallConfigTemplate(
			[]byte(`username: "{{ secret "apiToken" "fields.username" }}"`),
			installer.NewVaultTemplatingSecretStore(vault),
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(string(rendered)).To(Equal(`username: "codesphere"`))
	})

	It("returns an error for missing secrets", func() {
		_, err := configtemplating.RenderInstallConfigTemplate(
			[]byte(`password: "{{ secret "missing" }}"`),
			installer.NewVaultTemplatingSecretStore(vault),
		)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`secret "missing" not found`))
	})

	It("renders a config file using an explicit prod.vault.yaml path", func() {
		if !sopsAndAgeAvailable() {
			Skip("sops and age-keygen not available")
		}

		tempDir := GinkgoT().TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")
		vaultPath := filepath.Join(tempDir, "prod.vault.yaml")
		plaintextVaultPath := filepath.Join(tempDir, "prod.vault.plain.yaml")
		ageKeyPath := filepath.Join(tempDir, "age_key.txt")

		Expect(os.WriteFile(configPath, []byte(`secrets:
  baseDir: "`+tempDir+`"
codesphere:
  override:
    apiToken: "{{ secret "apiToken" }}"
`), 0644)).To(Succeed())
		vaultYaml, err := vault.Marshal()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(plaintextVaultPath, vaultYaml, 0600)).To(Succeed())
		Expect(exec.Command("age-keygen", "-o", ageKeyPath).Run()).To(Succeed())
		recipient, err := exec.Command("age-keygen", "-y", ageKeyPath).Output()
		Expect(err).NotTo(HaveOccurred())
		Expect(installer.EncryptFileWithSOPS(plaintextVaultPath, vaultPath, strings.TrimSpace(string(recipient)))).To(Succeed())

		renderedPath, cleanup, err := configtemplating.RenderConfigFileToTempIfNeeded(
			configPath,
			installer.NewLazyVaultTemplatingSecretStore(vaultPath, ageKeyPath),
		)
		defer cleanup()
		Expect(err).NotTo(HaveOccurred())

		rendered, err := os.ReadFile(renderedPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(rendered)).To(ContainSubstring(`apiToken: "super-secret-token"`))
	})

	It("rejects unencrypted prod.vault.yaml files", func() {
		tempDir := GinkgoT().TempDir()
		vaultPath := filepath.Join(tempDir, "prod.vault.yaml")

		vaultYaml, err := vault.Marshal()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(vaultPath, vaultYaml, 0600)).To(Succeed())

		_, err = installer.LoadVaultData(vaultPath, "")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("is not SOPS-encrypted"))
	})

})
