// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package vault_test

import (
	"os"
	"os/exec"
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

	ginkgo.Describe("DetectType", func() {
		ginkgo.It("reports plain for a missing file so a new vault can be created", func() {
			vaultType, err := vault.DetectType(fw, vaultPath)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(vaultType).To(gomega.Equal(vault.TypePlain))
		})

		ginkgo.DescribeTable("reports the type of the vault on disk",
			func(content string, want vault.Type) {
				gomega.Expect(os.WriteFile(vaultPath, []byte(content), 0600)).To(gomega.Succeed())

				vaultType, err := vault.DetectType(fw, vaultPath)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(vaultType).To(gomega.Equal(want))
			},
			ginkgo.Entry("plaintext", "secrets: []\n", vault.TypePlain),
			ginkgo.Entry("SOPS metadata", "secrets: []\nsops:\n    age: []\n", vault.TypeSOPS),
		)

		ginkgo.It("returns an error for a file that is not valid YAML", func() {
			gomega.Expect(os.WriteFile(vaultPath, []byte("secrets:\n\tbroken: value\n"), 0600)).To(gomega.Succeed())

			_, err := vault.DetectType(fw, vaultPath)
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

		ginkgo.It("keeps an encrypted vault encrypted", func() {
			if !sopsAndAgeAvailable() {
				ginkgo.Skip("sops and age-keygen not available")
			}

			ageKeyPath := filepath.Join(tmpDir, "age_key.txt")
			out, err := exec.Command("age-keygen", "-o", ageKeyPath).CombinedOutput()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), string(out))

			sopsStore, err := vault.New(vault.TypeSOPS, vault.Options{
				Path: vaultPath, AgeKey: ageKeyPath, FileIO: fw,
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(sopsStore.Save(&files.InstallVault{Secrets: []files.SecretEntry{
				{Name: "token", Fields: &files.SecretFields{Password: "s3cret"}},
			}})).To(gomega.Succeed())

			store, err := vault.New(vault.TypeAuto, vault.Options{
				Path: vaultPath, AgeKey: ageKeyPath, FileIO: fw,
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			loaded, err := store.Load()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			loaded.SetSecret(files.SecretEntry{Name: "extra", Fields: &files.SecretFields{Password: "value"}})
			gomega.Expect(store.Save(loaded)).To(gomega.Succeed())

			raw, err := os.ReadFile(vaultPath)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(string(raw)).To(gomega.ContainSubstring("sops"))
			gomega.Expect(string(raw)).NotTo(gomega.ContainSubstring("s3cret"))

			reloaded, err := store.Load()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(reloaded.GetSecret("extra").Fields.Password).To(gomega.Equal("value"))
		})

		ginkgo.It("fails on an encrypted vault when no age key is available", func() {
			// Deterministic: an empty value counts as unset, so an age key from the
			// environment cannot make this test pass by accident.
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
