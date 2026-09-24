// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"os"
	"path/filepath"

	"filippo.io/age"
	"github.com/codesphere-cloud/oms/internal/installer/vault/sops"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ResolveAgeKey", func() {
	var (
		dir string
		bs  *LocalBootstrapper
	)

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		bs = &LocalBootstrapper{
			fw:  util.NewFilesystemWriter(),
			Env: &CodesphereEnvironment{SecretsFilePath: filepath.Join(dir, "prod.vault.yaml")},
		}

		// Keep the developer's own age key out of the lookup.
		GinkgoT().Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
		GinkgoT().Setenv("SOPS_AGE_KEY", "")
		GinkgoT().Setenv("SOPS_AGE_KEY_FILE", "")
	})

	// The Codesphere installer is invoked with a private key file, so an identity that only
	// exists in SOPS_AGE_KEY has to be materialized first.
	It("materializes an identity from SOPS_AGE_KEY into its own key file", func() {
		identity, err := age.GenerateX25519Identity()
		Expect(err).NotTo(HaveOccurred())
		GinkgoT().Setenv("SOPS_AGE_KEY", identity.String())

		fallbackKey := filepath.Join(dir, "age_key.txt")
		Expect(os.WriteFile(fallbackKey, []byte("AGE-SECRET-KEY-1EXISTING\n"), 0600)).To(Succeed())

		Expect(bs.ResolveAgeKey()).To(Succeed())
		Expect(bs.ageRecipient).To(Equal(identity.Recipient().String()))
		Expect(filepath.Dir(bs.ageKeyPath)).To(Equal(dir))
		Expect(bs.ageKeyPath).NotTo(Equal(fallbackKey))

		info, err := os.Stat(bs.ageKeyPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))

		content, err := os.ReadFile(bs.ageKeyPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal(identity.String() + "\n"))

		keyFile, err := sops.ResolveExistingAgeKey(bs.ageKeyPath, dir)
		Expect(err).NotTo(HaveOccurred())
		Expect(keyFile).To(Equal(bs.ageKeyPath))

		Expect(bs.removeEnvAgeKeyFile()).To(Succeed())
		Expect(bs.ageKeyPath).NotTo(BeAnExistingFile())

		existing, err := os.ReadFile(fallbackKey)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(existing)).To(Equal("AGE-SECRET-KEY-1EXISTING\n"))
	})

	It("keeps the path of an explicitly provided key file", func() {
		identity, err := age.GenerateX25519Identity()
		Expect(err).NotTo(HaveOccurred())

		keyPath := filepath.Join(dir, "provided.txt")
		Expect(os.WriteFile(keyPath, []byte(identity.String()), 0600)).To(Succeed())
		bs.Env.AgeKey = keyPath

		Expect(bs.ResolveAgeKey()).To(Succeed())
		Expect(bs.ageKeyPath).To(Equal(keyPath))
		Expect(bs.ageRecipient).To(Equal(identity.Recipient().String()))
		Expect(filepath.Join(dir, "age_key.txt")).NotTo(BeAnExistingFile())
	})

	It("keeps the path of SOPS_AGE_KEY_FILE", func() {
		identity, err := age.GenerateX25519Identity()
		Expect(err).NotTo(HaveOccurred())

		keyPath := filepath.Join(dir, "keys.txt")
		Expect(os.WriteFile(keyPath, []byte(identity.String()), 0600)).To(Succeed())
		GinkgoT().Setenv("SOPS_AGE_KEY_FILE", keyPath)

		Expect(bs.ResolveAgeKey()).To(Succeed())
		Expect(bs.ageKeyPath).To(Equal(keyPath))
	})
})
