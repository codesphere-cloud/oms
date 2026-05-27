// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConfigTemplateCmd", func() {
	It("renders config templates with secrets from a vault file", func() {
		if !sopsAndAgeAvailable() {
			Skip("sops and age-keygen not available")
		}

		tempDir := GinkgoT().TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")
		vaultPath := filepath.Join(tempDir, "prod.vault.yaml")
		plaintextVaultPath := filepath.Join(tempDir, "prod.vault.plain.yaml")
		ageKeyPath := filepath.Join(tempDir, "age_key.txt")

		Expect(os.WriteFile(configPath, []byte(`codesphere:
  override:
    global:
      license:
        key: '{{ secret "codesphereLicenseKey" }}'
postgres:
  override:
    auth:
      username: '{{ secret "postgresAdmin" "fields.username" }}'
      password: '{{ secret "postgresAdmin" "fields.password" }}'
`), 0644)).To(Succeed())
		Expect(exec.Command("age-keygen", "-o", ageKeyPath).Run()).To(Succeed())

		vault := &files.InstallVault{
			Secrets: []files.SecretEntry{
				{
					Name: "codesphereLicenseKey",
					File: &files.SecretFile{Content: "license-secret"},
				},
				{
					Name: "postgresAdmin",
					Fields: &files.SecretFields{
						Username: "postgres",
						Password: "admin-secret",
					},
				},
			},
		}
		vaultYaml, err := vault.Marshal()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(plaintextVaultPath, vaultYaml, 0600)).To(Succeed())
		recipient, err := exec.Command("age-keygen", "-y", ageKeyPath).Output()
		Expect(err).NotTo(HaveOccurred())
		Expect(installer.EncryptFileWithSOPS(plaintextVaultPath, vaultPath, strings.TrimSpace(string(recipient)))).To(Succeed())

		rootCmd := cmd.GetRootCmd()
		var output bytes.Buffer
		rootCmd.SetOut(&output)
		rootCmd.SetErr(&output)
		rootCmd.SetArgs([]string{
			"config",
			"template",
			"--config",
			configPath,
			"--vault",
			vaultPath,
			"--age-key",
			ageKeyPath,
		})

		err = rootCmd.Execute()

		Expect(err).NotTo(HaveOccurred())
		Expect(output.String()).To(ContainSubstring("key: 'license-secret'"))
		Expect(output.String()).To(ContainSubstring("username: 'postgres'"))
		Expect(output.String()).To(ContainSubstring("password: 'admin-secret'"))
	})

	It("adds the template command with required flags", func() {
		rootCmd := cmd.GetRootCmd()

		configCmd, _, err := rootCmd.Find([]string{"config", "template"})
		Expect(err).NotTo(HaveOccurred())
		Expect(configCmd).NotTo(BeNil())
		Expect(configCmd.Use).To(Equal("template"))
		Expect(configCmd.Short).To(Equal("Render a config.yaml template using secrets from a vault file"))

		Expect(configCmd.Flags().Lookup("config")).NotTo(BeNil())
		Expect(configCmd.Flags().Lookup("vault")).NotTo(BeNil())
		Expect(configCmd.Flags().Lookup("age-key")).NotTo(BeNil())
	})
})

func sopsAndAgeAvailable() bool {
	if _, err := exec.LookPath("sops"); err != nil {
		return false
	}
	if _, err := exec.LookPath("age-keygen"); err != nil {
		return false
	}
	return true
}
