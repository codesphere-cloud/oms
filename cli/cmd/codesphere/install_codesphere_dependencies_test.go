// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("argoCDAndAppsInstall.loadVaultData", func() {
	It("falls back to config secrets.baseDir when --vault is not set", func() {
		tmpDir := GinkgoT().TempDir()
		secretsDir := filepath.Join(tmpDir, "secrets")
		Expect(os.MkdirAll(secretsDir, 0700)).To(Succeed())

		vault := &files.InstallVault{
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
		vaultYAML, err := vault.Marshal()
		Expect(err).ToNot(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(secretsDir, "prod.vault.yaml"), vaultYAML, 0600)).To(Succeed())

		install := &argoCDAndAppsInstall{
			opts: &InstallCodesphereOpts{},
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
