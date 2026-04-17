// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testuser

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HashPassword", func() {
	It("produces a deterministic result", func() {
		hash1 := HashPassword("Test1234!")
		hash2 := HashPassword("Test1234!")
		Expect(hash1).To(Equal(hash2))
	})

	It("produces a valid 64-char hex string", func() {
		hash := HashPassword("Test1234!")
		Expect(hash).To(HaveLen(64))
		Expect(hash).To(MatchRegexp("^[0-9a-f]{64}$"))
	})

	It("matches the expected known test vector", func() {
		// SHA256(SHA256("Test1234!" + salt1) + salt2)
		hash := HashPassword("Test1234!")
		Expect(hash).To(Equal("a40aa4bc3ed8f631a17ba3692fd631903dfce9052ddf7e398d679ae0465ac946"))
	})

	It("produces different hashes for different inputs", func() {
		hash1 := HashPassword("password1")
		hash2 := HashPassword("password2")
		Expect(hash1).NotTo(Equal(hash2))
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
		password := HashPassword("testtoken")
		token := HashAPIToken("testtoken")
		Expect(password).NotTo(Equal(token))
	})
})
