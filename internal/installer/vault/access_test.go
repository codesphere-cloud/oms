// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package vault_test

import (
	"fmt"
	"os"
	"path/filepath"

	"filippo.io/age"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("ResolveExistingVaultAgeKey", func() {
	var (
		dir       string
		vaultPath string
		fw        util.FileIO
	)

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		vaultPath = filepath.Join(dir, "prod.vault.yaml")
		fw = util.NewFilesystemWriter()

		// Keep the developer's own age key out of the lookup so the assertions are
		// deterministic on every machine.
		GinkgoT().Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
		GinkgoT().Setenv("SOPS_AGE_KEY", "")
		GinkgoT().Setenv("SOPS_AGE_KEY_FILE", "")
	})

	// A vault with SOPS metadata is enough: the resolution only inspects the file, it never
	// decrypts it.
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

	// Encrypted vaults require the sops binary, so a stub keeps these tests independent of the
	// real toolchain.
	putFakeSopsInPath := func() {
		binDir := filepath.Join(dir, "bin")
		Expect(os.MkdirAll(binDir, 0755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(binDir, "sops"), []byte("#!/bin/sh\nexit 0\n"), 0755)).To(Succeed())

		GinkgoT().Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

	It("treats a missing vault file as plaintext so a new vault can be created", func() {
		ageKey, err := vault.ResolveExistingVaultAgeKey(fw, vaultPath, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(ageKey).To(BeEmpty())
	})

	It("passes the given key through for a plaintext vault", func() {
		Expect(os.WriteFile(vaultPath, []byte("secrets: []\n"), 0600)).To(Succeed())

		ageKey, err := vault.ResolveExistingVaultAgeKey(fw, vaultPath, "key-from-flag")
		Expect(err).NotTo(HaveOccurred())
		Expect(ageKey).To(Equal("key-from-flag"))
	})

	It("reports a read failure instead of silently treating the vault as plaintext", func() {
		unreadable := filepath.Join(dir, "a-directory")
		Expect(os.MkdirAll(unreadable, 0755)).To(Succeed())

		_, err := vault.ResolveExistingVaultAgeKey(fw, unreadable, "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to detect vault type"))
	})

	Context("with an encrypted vault", func() {
		BeforeEach(func() {
			putFakeSopsInPath()
			writeEncryptedVault()
		})

		It("uses the explicit age key", func() {
			keyPath := writeAgeKey("explicit.txt")

			ageKey, err := vault.ResolveExistingVaultAgeKey(fw, vaultPath, keyPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(ageKey).To(Equal(keyPath))
		})

		It("uses the age_key.txt next to the vault when no key is given", func() {
			keyPath := writeAgeKey("age_key.txt")

			ageKey, err := vault.ResolveExistingVaultAgeKey(fw, vaultPath, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(ageKey).To(Equal(keyPath))
		})

		It("reports that no usable age key was found and never generates one", func() {
			_, err := vault.ResolveExistingVaultAgeKey(fw, vaultPath, "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no usable age key"))
			Expect(filepath.Join(dir, "age_key.txt")).NotTo(BeAnExistingFile())
		})

		It("reports a missing sops binary before trying to resolve a key", func() {
			GinkgoT().Setenv("PATH", filepath.Join(dir, "empty-bin"))

			_, err := vault.ResolveExistingVaultAgeKey(fw, vaultPath, writeAgeKey("explicit.txt"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("sops binary is not in PATH"))
		})
	})
})
