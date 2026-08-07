// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package vault_test

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Vault stores", func() {
	It("round-trips secrets and secret files through a plain vault", func() {
		path := filepath.Join(GinkgoT().TempDir(), "prod.vault.yaml")
		store, err := vault.New(vault.TypePlain, vault.Options{Path: path})
		Expect(err).NotTo(HaveOccurred())

		want := &files.InstallVault{Secrets: []files.SecretEntry{
			{Name: "password", Fields: &files.SecretFields{Username: "user", Password: "secret"}},
			{Name: "certificate", File: &files.SecretFile{Name: "ca.pem", Content: "PEM"}},
		}}
		Expect(store.Save(want)).To(Succeed())
		got, err := store.Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(got.GetSecret("password").Fields.Password).To(Equal("secret"))
		Expect(got.GetSecret("certificate").File.Content).To(Equal("PEM"))

		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
	})

	It("requires an age key for a SOPS vault", func() {
		GinkgoT().Setenv("SOPS_AGE_KEY", "")
		GinkgoT().Setenv("SOPS_AGE_KEY_FILE", "")
		_, err := vault.New(vault.TypeSOPS, vault.Options{Path: filepath.Join(GinkgoT().TempDir(), "prod.vault.yaml")})
		Expect(err).To(HaveOccurred())
	})

	It("round-trips secrets through a SOPS vault", func() {
		if _, err := exec.LookPath("sops"); err != nil {
			Skip("sops is not installed")
		}

		if _, err := exec.LookPath("age-keygen"); err != nil {
			Skip("age-keygen is not installed")
		}

		dir := GinkgoT().TempDir()
		keyPath := filepath.Join(dir, "age-key.txt")
		output, err := exec.Command("age-keygen", "-o", keyPath).CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(output))

		path := filepath.Join(dir, "prod.vault.yaml")
		store, err := vault.New(vault.TypeSOPS, vault.Options{Path: path, AgeKey: keyPath})
		Expect(err).NotTo(HaveOccurred())

		want := &files.InstallVault{Secrets: []files.SecretEntry{{Name: "token", Fields: &files.SecretFields{Password: "secret"}}}}
		Expect(store.Save(want)).To(Succeed())

		onDisk, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(onDisk).NotTo(BeEmpty())

		got, err := store.Load()
		Expect(err).NotTo(HaveOccurred())
		Expect(got.GetSecret("token").Fields.Password).To(Equal("secret"))
	})

	It("defaults an empty type to SOPS", func() {
		vaultType, err := vault.ParseType("")
		Expect(err).NotTo(HaveOccurred())
		Expect(vaultType).To(Equal(vault.TypeSOPS))
	})

	It("validates file paths in the file-backed implementations", func() {
		_, err := vault.NewPlainFileVault(vault.FileOptions{})
		Expect(err).To(HaveOccurred())
		GinkgoT().Setenv("SOPS_AGE_KEY", "test-key-is-present")

		_, err = vault.NewSOPSVault(vault.SOPSOptions{})
		Expect(err).To(HaveOccurred())
	})

	It("handles a missing plain file inside the plain vault", func() {
		store, err := vault.NewPlainFileVault(vault.FileOptions{Path: filepath.Join(GinkgoT().TempDir(), "missing.yaml")})
		Expect(err).NotTo(HaveOccurred())
		data, err := store.LoadOrCreate()
		Expect(err).NotTo(HaveOccurred())
		Expect(data.Secrets).To(BeEmpty())
	})
})
