// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testuser

import (
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("generateAPIToken", func() {
	It("returns no error", func() {
		_, err := generateAPIToken()
		Expect(err).NotTo(HaveOccurred())
	})

	It("starts with the CS_ prefix", func() {
		token, err := generateAPIToken()
		Expect(err).NotTo(HaveOccurred())
		Expect(token).To(HavePrefix("CS_"))
	})

	It("has the correct length (3 prefix + 32 hex chars)", func() {
		token, err := generateAPIToken()
		Expect(err).NotTo(HaveOccurred())
		Expect(token).To(HaveLen(35))
	})

	It("produces unique tokens on successive calls", func() {
		token1, err := generateAPIToken()
		Expect(err).NotTo(HaveOccurred())
		token2, err := generateAPIToken()
		Expect(err).NotTo(HaveOccurred())
		Expect(token1).NotTo(Equal(token2))
	})

	It("contains only hex characters after the prefix", func() {
		token, err := generateAPIToken()
		Expect(err).NotTo(HaveOccurred())
		hexPart := token[len(tokenPrefix):]
		Expect(hexPart).To(MatchRegexp("^[0-9a-f]{32}$"))
	})
})

var _ = Describe("WriteResultToFile", func() {
	It("writes a valid JSON file to the given directory", func() {
		dir := GinkgoT().TempDir()
		result := &TestUserResult{
			Email:             "test@example.com",
			PlaintextPassword: "secret123",
			PlaintextAPIToken: "CS_abc123",
		}

		filePath, err := WriteResultToFile(result, dir)
		Expect(err).NotTo(HaveOccurred())
		Expect(filePath).To(Equal(filepath.Join(dir, "test-user.json")))

		data, err := os.ReadFile(filePath)
		Expect(err).NotTo(HaveOccurred())

		var loaded TestUserResult
		err = json.Unmarshal(data, &loaded)
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded.Email).To(Equal("test@example.com"))
		Expect(loaded.PlaintextPassword).To(Equal("secret123"))
		Expect(loaded.PlaintextAPIToken).To(Equal("CS_abc123"))
	})

	It("creates the directory if it does not exist", func() {
		dir := filepath.Join(GinkgoT().TempDir(), "nested", "subdir")
		result := &TestUserResult{
			Email:             "user@example.com",
			PlaintextPassword: "pass",
			PlaintextAPIToken: "CS_token",
		}

		filePath, err := WriteResultToFile(result, dir)
		Expect(err).NotTo(HaveOccurred())
		Expect(filePath).To(BeARegularFile())
	})

	It("sets restrictive file permissions (0600)", func() {
		dir := GinkgoT().TempDir()
		result := &TestUserResult{
			Email:             "user@example.com",
			PlaintextPassword: "pass",
			PlaintextAPIToken: "CS_token",
		}

		filePath, err := WriteResultToFile(result, dir)
		Expect(err).NotTo(HaveOccurred())

		info, err := os.Stat(filePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
	})
})
