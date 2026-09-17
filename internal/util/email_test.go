// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("ValidateEmail", func() {
	It("accepts a valid email address", func() {
		err := util.ValidateEmail("jane.doe@example.com")
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns error for a plain string", func() {
		err := util.ValidateEmail("test")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("expected a valid email address"))
	})

	It("returns error for an empty string", func() {
		err := util.ValidateEmail("")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("expected a valid email address"))
	})
})
