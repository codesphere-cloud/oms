// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"path/filepath"

	intutil "github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("resolveVaultAccess", func() {
	var (
		dir string
		fw  intutil.FileIO
	)

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		fw = intutil.NewFilesystemWriter()
	})

	It("resolves the key of the local vault outside of recovery", func() {
		vaultPath := filepath.Join(dir, "prod.vault.yaml")
		Expect(os.WriteFile(vaultPath, []byte("secrets: []\n"), 0600)).To(Succeed())

		ageKey, err := resolveVaultAccess(fw, vaultPath, "key-from-flag", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(ageKey).To(Equal("key-from-flag"))
	})

	It("skips the local vault when recovering and keeps the age key flag", func() {
		unreadable := filepath.Join(dir, "unreadable")
		Expect(os.MkdirAll(unreadable, 0755)).To(Succeed())

		ageKey, err := resolveVaultAccess(fw, unreadable, "key-from-flag", true)
		Expect(err).NotTo(HaveOccurred())
		Expect(ageKey).To(Equal("key-from-flag"))
	})
})
