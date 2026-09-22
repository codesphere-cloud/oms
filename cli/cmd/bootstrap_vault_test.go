// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"filippo.io/age"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	intutil "github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("resolveVaultAccess", func() {
	var (
		dir       string
		vaultPath string
		fw        intutil.FileIO
	)

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		vaultPath = filepath.Join(dir, "prod.vault.yaml")
		fw = intutil.NewFilesystemWriter()

		// Keep the developer's own age key out of the lookup so the assertions are
		// deterministic on every machine.
		GinkgoT().Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
		GinkgoT().Setenv("SOPS_AGE_KEY", "")
		GinkgoT().Setenv("SOPS_AGE_KEY_FILE", "")
	})

	// A vault with SOPS metadata is enough: resolveVaultAccess only inspects the file,
	// it never decrypts it.
	writeEncryptedVault := func() {
		Expect(os.WriteFile(vaultPath, []byte("secrets: []\nsops:\n    age: []\n"), 0600)).To(Succeed())
	}

	writeAgeKey := func(name string) string {
		identity, err := age.GenerateX25519Identity()
		Expect(err).NotTo(HaveOccurred())

		keyPath := filepath.Join(dir, name)
		key := fmt.Sprintf("# created: test\n# public key: %s\n%s\n", identity.Recipient(), identity.String())
		Expect(os.WriteFile(keyPath, []byte(key), 0600)).To(Succeed())

		return keyPath
	}

	// resolveVaultAccess insists on the sops binary for encrypted vaults, so a stub keeps
	// these tests independent of the real toolchain.
	putFakeSopsInPath := func() {
		binDir := filepath.Join(dir, "bin")
		Expect(os.MkdirAll(binDir, 0755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(binDir, "sops"), []byte("#!/bin/sh\nexit 0\n"), 0755)).To(Succeed())

		GinkgoT().Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

	It("treats a missing vault file as plaintext so a new vault can be created", func() {
		vaultType, ageKey, err := resolveVaultAccess(fw, vaultPath, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(vaultType).To(Equal(vault.TypePlain))
		Expect(ageKey).To(BeEmpty())
	})

	It("keeps a plaintext vault plain and passes the flag through", func() {
		Expect(os.WriteFile(vaultPath, []byte("secrets: []\n"), 0600)).To(Succeed())

		vaultType, ageKey, err := resolveVaultAccess(fw, vaultPath, "key-from-flag")
		Expect(err).NotTo(HaveOccurred())
		Expect(vaultType).To(Equal(vault.TypePlain))
		Expect(ageKey).To(Equal("key-from-flag"))
	})

	It("reports a read failure instead of silently treating the vault as plaintext", func() {
		Expect(os.MkdirAll(filepath.Join(dir, "a-directory"), 0755)).To(Succeed())

		_, _, err := resolveVaultAccess(fw, filepath.Join(dir, "a-directory"), "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to detect vault type"))
	})

	It("uses the explicit age key for an encrypted vault", func() {
		putFakeSopsInPath()
		writeEncryptedVault()

		keyPath := writeAgeKey("explicit.txt")

		vaultType, ageKey, err := resolveVaultAccess(fw, vaultPath, keyPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(vaultType).To(Equal(vault.TypeSOPS))
		Expect(ageKey).To(Equal(keyPath))
	})

	It("falls back to SOPS_AGE_KEY_FILE for an encrypted vault", func() {
		putFakeSopsInPath()
		writeEncryptedVault()

		keyPath := writeAgeKey("keys.txt")
		GinkgoT().Setenv("SOPS_AGE_KEY_FILE", keyPath)

		vaultType, ageKey, err := resolveVaultAccess(fw, vaultPath, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(vaultType).To(Equal(vault.TypeSOPS))
		Expect(ageKey).To(Equal(keyPath))
	})

	It("falls back to an age_key.txt next to an encrypted vault", func() {
		putFakeSopsInPath()
		writeEncryptedVault()

		keyPath := writeAgeKey("age_key.txt")

		vaultType, ageKey, err := resolveVaultAccess(fw, vaultPath, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(vaultType).To(Equal(vault.TypeSOPS))
		Expect(ageKey).To(Equal(keyPath))
	})

	It("reports that no usable age key was found for an encrypted vault", func() {
		putFakeSopsInPath()
		writeEncryptedVault()

		_, _, err := resolveVaultAccess(fw, vaultPath, "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no usable age key"))
	})

	It("never generates a key that could not decrypt the vault", func() {
		putFakeSopsInPath()
		writeEncryptedVault()

		_, _, err := resolveVaultAccess(fw, vaultPath, "")
		Expect(err).To(HaveOccurred())
		Expect(filepath.Join(dir, "age_key.txt")).NotTo(BeAnExistingFile())
	})

	It("reports a missing sops binary before trying to resolve a key", func() {
		writeEncryptedVault()

		keyPath := writeAgeKey("explicit.txt")

		GinkgoT().Setenv("PATH", filepath.Join(dir, "empty-bin"))

		_, _, err := resolveVaultAccess(fw, vaultPath, keyPath)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("sops binary is not in PATH"))
	})
})
