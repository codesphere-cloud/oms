// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testuser

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HashPassword", func() {
	BeforeEach(func() {
		GinkgoHelper()
		Expect(os.Setenv("OMS_CS_SALT_1", "testsalt1")).To(Succeed())
		Expect(os.Setenv("OMS_CS_SALT_2", "testsalt2")).To(Succeed())
	})

	AfterEach(func() {
		GinkgoHelper()
		Expect(os.Unsetenv("OMS_CS_SALT_1")).To(Succeed())
		Expect(os.Unsetenv("OMS_CS_SALT_2")).To(Succeed())
	})

	It("produces a deterministic result", func() {
		hash1, err := HashPassword("Test1234!")
		Expect(err).NotTo(HaveOccurred())
		hash2, err := HashPassword("Test1234!")
		Expect(err).NotTo(HaveOccurred())
		Expect(hash1).To(Equal(hash2))
	})

	It("produces a valid 64-char hex string", func() {
		hash, err := HashPassword("Test1234!")
		Expect(err).NotTo(HaveOccurred())
		Expect(hash).To(HaveLen(64))
		Expect(hash).To(MatchRegexp("^[0-9a-f]{64}$"))
	})

	It("produces different hashes for different inputs", func() {
		hash1, err := HashPassword("password1")
		Expect(err).NotTo(HaveOccurred())
		hash2, err := HashPassword("password2")
		Expect(err).NotTo(HaveOccurred())
		Expect(hash1).NotTo(Equal(hash2))
	})

	It("returns an error when SALT_1 is not set", func() {
		Expect(os.Unsetenv("OMS_CS_SALT_1")).To(Succeed())
		_, err := HashPassword("Test1234!")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("OMS_CS_SALT_1"))
	})

	It("returns an error when SALT_2 is not set", func() {
		Expect(os.Unsetenv("OMS_CS_SALT_2")).To(Succeed())
		_, err := HashPassword("Test1234!")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("OMS_CS_SALT_2"))
	})
})

var _ = Describe("HashAPIToken", func() {
	It("produces a deterministic result", func() {
		hash1 := HashAPIToken("testtoken")
		hash2 := HashAPIToken("testtoken")
		Expect(hash1).To(Equal(hash2))
	})

	It("produces a valid 64-char hex string", func() {
		hash := HashAPIToken("testtoken")
		Expect(hash).To(HaveLen(64))
		Expect(hash).To(MatchRegexp("^[0-9a-f]{64}$"))
	})

	It("matches the expected known test vector", func() {
		// SHA256("testtoken")
		hash := HashAPIToken("testtoken")
		Expect(hash).To(Equal("ada63e98fe50eccb55036d88eda4b2c3709f53c2b65bc0335797067e9a2a5d8b"))
	})

	It("produces different hashes for different inputs", func() {
		hash1 := HashAPIToken("token1")
		hash2 := HashAPIToken("token2")
		Expect(hash1).NotTo(Equal(hash2))
	})

	It("differs from HashPassword for the same input", func() {
		Expect(os.Setenv("OMS_CS_SALT_1", "testsalt1")).To(Succeed())
		Expect(os.Setenv("OMS_CS_SALT_2", "testsalt2")).To(Succeed())
		defer func() { Expect(os.Unsetenv("OMS_CS_SALT_1")).To(Succeed()) }()
		defer func() { Expect(os.Unsetenv("OMS_CS_SALT_2")).To(Succeed()) }()

		password, err := HashPassword("testtoken")
		Expect(err).NotTo(HaveOccurred())
		token := HashAPIToken("testtoken")
		Expect(password).NotTo(Equal(token))
	})
})
