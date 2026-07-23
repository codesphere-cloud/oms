// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type argoCDInstallerStub struct {
	called bool
}

func (s *argoCDInstallerStub) Install() error {
	s.called = true
	return nil
}

var _ = Describe("ArgoCDAndAppsInstall", func() {
	It("uses the configured ArgoCD installer instance", func() {
		argoCDInstall := &argoCDInstallerStub{}
		install := installer.NewArgoCDAndAppsInstall(installer.ArgoCDAndAppsInstallConfig{
			ArgoCDInstaller: argoCDInstall,
		})

		Expect(install.InstallArgoCD()).To(Succeed())
		Expect(argoCDInstall.called).To(BeTrue())
	})
})

var _ = Describe("VaultAndRESTConfig", func() {
	It("falls back to config secrets.baseDir when the vault path is not set", func() {
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

		config := files.RootConfig{
			Secrets: files.SecretsConfig{
				BaseDir: secretsDir,
			},
		}

		loadedVault, restConfig, err := installer.VaultAndRESTConfig("", ageKeyPath, config)
		Expect(err).ToNot(HaveOccurred())
		Expect(loadedVault).ToNot(BeNil())
		Expect(restConfig).ToNot(BeNil())
	})
})
