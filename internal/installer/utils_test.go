// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("IsValidIP", func() {
	DescribeTable("IP validation",
		func(ip string, valid bool) {
			result := IsValidIP(ip)
			Expect(result).To(Equal(valid))
		},
		Entry("valid IPv4", "192.168.1.1", true),
		Entry("valid IPv6", "2001:db8::1", true),
		Entry("invalid IP", "not-an-ip", false),
		Entry("empty string", "", false),
		Entry("partial IP", "192.168", false),
		Entry("localhost", "127.0.0.1", true),
	)
})

var _ = Describe("AddConfigComments", func() {
	It("adds header comments to config YAML", func() {
		yamlData := []byte("test: value\n")

		result := AddConfigComments(yamlData)
		resultStr := string(result)

		Expect(resultStr).To(ContainSubstring("Codesphere Installer Configuration"))
		Expect(resultStr).To(ContainSubstring("test: value"))
	})
})

var _ = Describe("AddVaultComments", func() {
	It("adds security warnings to vault YAML", func() {
		yamlData := []byte("secrets:\n  - name: test\n")

		result := AddVaultComments(yamlData)
		resultStr := string(result)

		Expect(resultStr).To(ContainSubstring("Codesphere Installer Secrets"))
		Expect(resultStr).To(ContainSubstring("IMPORTANT"))
		Expect(resultStr).To(ContainSubstring("SOPS"))
		Expect(resultStr).To(ContainSubstring("secrets:"))
	})
})
