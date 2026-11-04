// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Registry - Unexported", func() {
	DescribeTable("ResolveRegistryType",
		func(input string, explicit, airgapped bool, expected RegistryType) {
			Expect(ResolveRegistryType(input, explicit, airgapped)).To(Equal(expected))
		},
		Entry("keeps the default type without airgap", "github", false, false, RegistryTypeGitHub),
		Entry("selects the local container registry for an airgapped installation", "github", false, true, RegistryTypeLocalContainer),
		Entry("keeps an explicit type for an airgapped installation so validation rejects it", "artifact-registry", true, true, RegistryTypeArtifactRegistry),
		Entry("keeps an explicit local container registry for an airgapped installation", "local-container", true, true, RegistryTypeLocalContainer),
	)

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
