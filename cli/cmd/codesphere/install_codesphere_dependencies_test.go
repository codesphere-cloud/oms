// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("argoCDAndAppsInstall.loadVaultData", func() {
	It("falls back to config secrets.baseDir when --vault is not set", func() {
		if !installCodesphereSopsAndAgeAvailable() {
			Skip("sops and age-keygen not available")
		}

		tmpDir := GinkgoT().TempDir()
		secretsDir := filepath.Join(tmpDir, "secrets")
		Expect(os.MkdirAll(secretsDir, 0700)).To(Succeed())

		installVault := &files.InstallVault{
			Secrets: []files.SecretEntry{
				{
					Name: files.SecretRegistryPassword,
					Fields: &files.SecretFields{
						Password: "registry-password",
					},
				},
				{
					Name: files.SecretKubeConfig,
					File: &files.SecretFile{
						Name: "kubeconfig",
						Content: `apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user:
    token: test-token
`,
					},
				},
			},
		}
		vaultYAML, err := installVault.Marshal()
		Expect(err).ToNot(HaveOccurred())
		plaintextVaultPath := filepath.Join(secretsDir, "prod.vault.plain.yaml")
		Expect(os.WriteFile(plaintextVaultPath, vaultYAML, 0600)).To(Succeed())

		ageKeyPath := filepath.Join(tmpDir, "age_key.txt")
		Expect(exec.Command("age-keygen", "-o", ageKeyPath).Run()).To(Succeed())
		recipient, err := exec.Command("age-keygen", "-y", ageKeyPath).Output()
		Expect(err).ToNot(HaveOccurred())

		vaultPath := filepath.Join(secretsDir, "prod.vault.yaml")
		Expect(vault.EncryptFileWithSOPS(plaintextVaultPath, vaultPath, strings.TrimSpace(string(recipient)))).To(Succeed())

		install := &argoCDAndAppsInstall{
			opts: &InstallCodesphereOpts{
				PrivKey: ageKeyPath,
			},
			config: files.RootConfig{
				Secrets: files.SecretsConfig{
					BaseDir: secretsDir,
				},
			},
		}

		err = install.loadVaultData()
		Expect(err).ToNot(HaveOccurred())
		Expect(install.vault).ToNot(BeNil())
		Expect(install.ociPassword).To(Equal("registry-password"))
		Expect(install.kubeConfig).ToNot(BeNil())
		Expect(install.kubeClient).ToNot(BeNil())
	})
})
