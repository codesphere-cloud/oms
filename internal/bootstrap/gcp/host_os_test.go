// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HostOS", func() {
	It("lists the supported host operating systems", func() {
		Expect(gcp.SupportedHostOSes()).To(Equal([]string{"rhel-9", "ubuntu"}))
	})

	It("boots Ubuntu 22.04 for the Ubuntu host OS", func() {
		image, err := gcp.HostOSUbuntu.SourceImage()
		Expect(err).NotTo(HaveOccurred())
		Expect(image).To(Equal("projects/ubuntu-os-cloud/global/images/family/ubuntu-2204-lts"))
	})

	It("boots Ubuntu 22.04 when no host OS is set, as infra files written before the selection did", func() {
		image, err := gcp.HostOS("").SourceImage()
		Expect(err).NotTo(HaveOccurred())
		Expect(image).To(Equal("projects/ubuntu-os-cloud/global/images/family/ubuntu-2204-lts"))
	})

	It("boots RHEL 9.6 with Extended Update Support for the RHEL 9 host OS", func() {
		image, err := gcp.HostOSRHEL9.SourceImage()
		Expect(err).NotTo(HaveOccurred())
		Expect(image).To(Equal("projects/rhel-cloud/global/images/family/rhel-9-6-eus"))
	})

	It("rejects an unsupported host OS and names the supported ones", func() {
		_, err := gcp.HostOS("debian-12").SourceImage()
		Expect(err).To(MatchError(ContainSubstring(`unsupported host OS "debian-12" (supported: rhel-9, ubuntu)`)))
	})
})
