// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testuser

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

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
})
