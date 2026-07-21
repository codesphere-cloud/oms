// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
)

const testKubeConfig = `apiVersion: v1
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
`

// writeVaultFile SOPS-encrypts vault with a freshly generated age key and writes
// it to dir/prod.vault.yaml, returning the vault path and the age key path
// needed to decrypt it. Skips the test if sops/age-keygen are unavailable.
func writeVaultFile(dir string, installVault *files.InstallVault) (vaultPath, ageKeyPath string) {
	if !sopsAndAgeAvailable() {
		Skip("sops and age-keygen not available")
	}

	vaultYAML, err := installVault.Marshal()
	Expect(err).ToNot(HaveOccurred())

	plaintextPath := filepath.Join(dir, "prod.vault.plain.yaml")
	Expect(os.WriteFile(plaintextPath, vaultYAML, 0600)).To(Succeed())

	ageKeyPath = filepath.Join(dir, "age_key.txt")
	Expect(exec.Command("age-keygen", "-o", ageKeyPath).Run()).To(Succeed())
	recipient, err := exec.Command("age-keygen", "-y", ageKeyPath).Output()
	Expect(err).ToNot(HaveOccurred())

	vaultPath = filepath.Join(dir, "prod.vault.yaml")
	Expect(vault.EncryptFileWithSOPS(plaintextPath, vaultPath, strings.TrimSpace(string(recipient)))).To(Succeed())

	return vaultPath, ageKeyPath
}

func vaultWithKubeConfig() *files.InstallVault {
	return &files.InstallVault{
		Secrets: []files.SecretEntry{
			{
				Name: files.SecretKubeConfig,
				File: &files.SecretFile{
					Name:    "kubeconfig",
					Content: testKubeConfig,
				},
			},
		},
	}
}

var _ = Describe("ResolveVaultPath", func() {
	It("returns the explicit vault path when set", func() {
		path, err := installer.ResolveVaultPath("/some/vault.yaml", files.RootConfig{
			Secrets: files.SecretsConfig{BaseDir: "/ignored"},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(path).To(Equal("/some/vault.yaml"))
	})

	It("falls back to prod.vault.yaml in the config secrets baseDir", func() {
		path, err := installer.ResolveVaultPath("", files.RootConfig{
			Secrets: files.SecretsConfig{BaseDir: "/base"},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(path).To(Equal(filepath.Join("/base", "prod.vault.yaml")))
	})

	It("treats a whitespace-only vault path as unset", func() {
		path, err := installer.ResolveVaultPath("   ", files.RootConfig{
			Secrets: files.SecretsConfig{BaseDir: "/base"},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(path).To(Equal(filepath.Join("/base", "prod.vault.yaml")))
	})

	It("fails when neither vault path nor secrets baseDir is set", func() {
		_, err := installer.ResolveVaultPath("", files.RootConfig{})
		Expect(err).To(MatchError(ContainSubstring("vault path is not set")))
	})
})

var _ = Describe("VaultAndRESTConfig", func() {
	It("loads the vault and builds a REST config from the kubeconfig secret", func() {
		vaultPath, ageKeyPath := writeVaultFile(GinkgoT().TempDir(), vaultWithKubeConfig())

		vault, restConfig, err := installer.VaultAndRESTConfig(vaultPath, ageKeyPath, files.RootConfig{})
		Expect(err).ToNot(HaveOccurred())
		Expect(vault).ToNot(BeNil())
		Expect(vault.GetSecret(files.SecretKubeConfig)).ToNot(BeNil())
		Expect(restConfig).ToNot(BeNil())
		Expect(restConfig.Host).To(Equal("https://127.0.0.1:6443"))
		Expect(restConfig.BearerToken).To(Equal("test-token"))
	})

	It("resolves the vault path via the config secrets baseDir", func() {
		secretsDir := GinkgoT().TempDir()
		_, ageKeyPath := writeVaultFile(secretsDir, vaultWithKubeConfig())

		vault, restConfig, err := installer.VaultAndRESTConfig("", ageKeyPath, files.RootConfig{
			Secrets: files.SecretsConfig{BaseDir: secretsDir},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(vault).ToNot(BeNil())
		Expect(restConfig).ToNot(BeNil())
	})

	It("fails when the vault path cannot be resolved", func() {
		_, _, err := installer.VaultAndRESTConfig("", "", files.RootConfig{})
		Expect(err).To(MatchError(ContainSubstring("vault path is not set")))
	})

	It("fails when the vault file does not exist", func() {
		vaultPath := filepath.Join(GinkgoT().TempDir(), "missing.vault.yaml")

		_, _, err := installer.VaultAndRESTConfig(vaultPath, "", files.RootConfig{})
		Expect(err).To(MatchError(ContainSubstring("failed to load vault")))
	})

	It("fails when the vault does not contain a kubeconfig secret", func() {
		vaultPath, ageKeyPath := writeVaultFile(GinkgoT().TempDir(), &files.InstallVault{
			Secrets: []files.SecretEntry{
				{
					Name:   files.SecretRegistryPassword,
					Fields: &files.SecretFields{Password: "registry-password"},
				},
			},
		})

		_, _, err := installer.VaultAndRESTConfig(vaultPath, ageKeyPath, files.RootConfig{})
		Expect(err).To(MatchError(ContainSubstring("kubeconfig not found in vault")))
	})

	It("fails when the kubeconfig secret content is empty", func() {
		vaultPath, ageKeyPath := writeVaultFile(GinkgoT().TempDir(), &files.InstallVault{
			Secrets: []files.SecretEntry{
				{
					Name: files.SecretKubeConfig,
					File: &files.SecretFile{Name: "kubeconfig", Content: "   "},
				},
			},
		})

		_, _, err := installer.VaultAndRESTConfig(vaultPath, ageKeyPath, files.RootConfig{})
		Expect(err).To(MatchError(ContainSubstring("kubeconfig not found in vault")))
	})

	It("fails when the kubeconfig content is not a valid kubeconfig", func() {
		vaultPath, ageKeyPath := writeVaultFile(GinkgoT().TempDir(), &files.InstallVault{
			Secrets: []files.SecretEntry{
				{
					Name: files.SecretKubeConfig,
					File: &files.SecretFile{Name: "kubeconfig", Content: "not: a kubeconfig"},
				},
			},
		})

		_, _, err := installer.VaultAndRESTConfig(vaultPath, ageKeyPath, files.RootConfig{})
		Expect(err).To(MatchError(ContainSubstring("failed to load kubernetes config from vault")))
	})
})

var _ = Describe("EnsureClusterAdminSecret", func() {
	It("is a no-op when the config does not set a cluster admin email", func() {
		// No vault path or secrets baseDir: reaching the vault loading would fail,
		// so a nil result proves the email check short-circuits first.
		err := installer.EnsureClusterAdminSecret(context.Background(), "", "", files.RootConfig{})
		Expect(err).ToNot(HaveOccurred())
	})

	It("propagates vault loading errors when an email is set", func() {
		cfg := files.RootConfig{
			Codesphere: files.CodesphereConfig{ClusterAdminEmail: "admin@codesphere.com"},
		}

		err := installer.EnsureClusterAdminSecret(context.Background(), "", "", cfg)
		Expect(err).To(MatchError(ContainSubstring("vault path is not set")))
	})

	It("fails when the vault file does not exist", func() {
		vaultPath := filepath.Join(GinkgoT().TempDir(), "missing.vault.yaml")
		cfg := files.RootConfig{
			Codesphere: files.CodesphereConfig{ClusterAdminEmail: "admin@codesphere.com"},
		}

		err := installer.EnsureClusterAdminSecret(context.Background(), vaultPath, "", cfg)
		Expect(err).To(MatchError(ContainSubstring("failed to load vault")))
	})
})
