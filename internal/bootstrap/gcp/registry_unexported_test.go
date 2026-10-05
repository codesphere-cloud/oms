// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Registry - Unexported", func() {
	// Runs the quoted value through a real shell, which is the only way to tell that it arrives
	// unchanged rather than that the quoting merely looks right.
	DescribeTable("shellQuote passes a value to the shell unchanged",
		func(value string) {
			out, err := exec.Command("sh", "-c", "printf '%s' "+shellQuote(value)).Output()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal(value))
		},
		Entry("plain", "custom-registry"),
		Entry("single quote", "it's"),
		Entry("only single quotes", "''"),
		Entry("shell syntax", `$(id) $HOME "x" \n ; | & * ~`),
		Entry("empty", ""),
	)
})
