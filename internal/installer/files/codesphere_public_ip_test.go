// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Codesphere public IP", func() {
	Describe("ValidatePublicIP", func() {
		It("requires a public IP", func() {
			Expect(files.CodesphereConfig{}.ValidatePublicIP()).To(MatchError(files.ErrPublicIPRequired))
		})

		It("rejects a value that is not an IP", func() {
			err := files.CodesphereConfig{PublicIP: "not-an-ip"}.ValidatePublicIP()

			Expect(err).To(MatchError(files.ErrPublicIPInvalid))
			Expect(err).To(MatchError(ContainSubstring("not-an-ip")))
		})

		// Config generation writes this placeholder, so only the install-time check may reject it.
		It("accepts the loopback placeholder", func() {
			Expect(files.CodesphereConfig{PublicIP: "127.0.0.1"}.ValidatePublicIP()).To(Succeed())
		})
	})

	Describe("ValidateReachablePublicIP", func() {
		It("accepts a public IPv4 address", func() {
			Expect(files.CodesphereConfig{PublicIP: "203.0.113.10"}.ValidateReachablePublicIP()).To(Succeed())
		})

		It("accepts a public IPv6 address", func() {
			Expect(files.CodesphereConfig{PublicIP: "2001:db8::10"}.ValidateReachablePublicIP()).To(Succeed())
		})

		It("accepts a private address for on-premises and VM-based installs", func() {
			Expect(files.CodesphereConfig{PublicIP: "192.168.5.15"}.ValidateReachablePublicIP()).To(Succeed())
		})

		It("still requires a public IP", func() {
			Expect(files.CodesphereConfig{}.ValidateReachablePublicIP()).To(MatchError(files.ErrPublicIPRequired))
		})

		It("still rejects a value that is not an IP", func() {
			Expect(files.CodesphereConfig{PublicIP: "not-an-ip"}.ValidateReachablePublicIP()).To(MatchError(files.ErrPublicIPInvalid))
		})

		It("rejects the IPv4 loopback placeholder", func() {
			err := files.CodesphereConfig{PublicIP: "127.0.0.1"}.ValidateReachablePublicIP()

			Expect(err).To(MatchError(files.ErrPublicIPUnreachable))
			Expect(err).To(MatchError(ContainSubstring("127.0.0.1")))
		})

		It("rejects the IPv6 loopback address", func() {
			Expect(files.CodesphereConfig{PublicIP: "::1"}.ValidateReachablePublicIP()).To(MatchError(files.ErrPublicIPUnreachable))
		})

		It("rejects the unspecified address", func() {
			Expect(files.CodesphereConfig{PublicIP: "0.0.0.0"}.ValidateReachablePublicIP()).To(MatchError(files.ErrPublicIPUnreachable))
		})

		It("rejects a link-local address", func() {
			Expect(files.CodesphereConfig{PublicIP: "169.254.10.1"}.ValidateReachablePublicIP()).To(MatchError(files.ErrPublicIPUnreachable))
		})

		It("rejects a multicast address", func() {
			Expect(files.CodesphereConfig{PublicIP: "224.0.0.1"}.ValidateReachablePublicIP()).To(MatchError(files.ErrPublicIPUnreachable))
		})
	})
})
