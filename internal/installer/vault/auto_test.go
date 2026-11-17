// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package vault_test

import (
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/util"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Auto vault", func() {
	var (
		tmpDir    string
		fw        util.FileIO
		vaultPath string
	)

	ginkgo.BeforeEach(func() {
		tmpDir = ginkgo.GinkgoT().TempDir()
		fw = util.NewFilesystemWriter()
		vaultPath = filepath.Join(tmpDir, "prod.vault.yaml")
	})

	ginkgo.Describe("IsEncryptedFile", func() {
		ginkgo.It("reports a vault with SOPS metadata as encrypted", func() {
			gomega.Expect(os.WriteFile(vaultPath, []byte("secrets: []\nsops:\n    age: []\n"), 0600)).To(gomega.Succeed())

			encrypted, err := vault.IsEncryptedFile(fw, vaultPath)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(encrypted).To(gomega.BeTrue())
		})

		ginkgo.It("returns an error for a file that is not valid YAML", func() {
			gomega.Expect(os.WriteFile(vaultPath, []byte("secrets:\n\tbroken: value\n"), 0600)).To(gomega.Succeed())

			_, err := vault.IsEncryptedFile(fw, vaultPath)
			gomega.Expect(err).To(gomega.HaveOccurred())
		})
	})

	ginkgo.Describe("round trip", func() {
		ginkgo.It("writes a vault that does not exist yet as plaintext", func() {
			store, err := vault.New(vault.TypeAuto, vault.Options{Path: vaultPath, FileIO: fw})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			err = store.Save(&files.InstallVault{Secrets: []files.SecretEntry{
				{Name: "token", Fields: &files.SecretFields{Password: "s3cret"}},
			}})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			raw, err := os.ReadFile(vaultPath)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(string(raw)).NotTo(gomega.ContainSubstring("sops:"))

			loaded, err := store.Load()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(loaded.GetSecret("token").Fields.Password).To(gomega.Equal("s3cret"))
		})

		ginkgo.It("fails on an encrypted vault when no age key is available", func() {
			// Empty values count as unset, so an environment key cannot make this pass by accident.
			ginkgo.GinkgoT().Setenv("SOPS_AGE_KEY", "")
			ginkgo.GinkgoT().Setenv("SOPS_AGE_KEY_FILE", "")

			gomega.Expect(os.WriteFile(vaultPath, []byte("secrets: []\nsops:\n    age: []\n"), 0600)).To(gomega.Succeed())

			store, err := vault.New(vault.TypeAuto, vault.Options{Path: vaultPath, FileIO: fw})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			_, err = store.Load()
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).To(gomega.ContainSubstring("age key"))
		})
	})
})
