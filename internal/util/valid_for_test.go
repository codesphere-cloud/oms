// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("GetDurationFromString", func() {
	It("parses days to duration", func() {
		d, err := util.GetDurationFromString("10d")
		Expect(err).NotTo(HaveOccurred())
		Expect(d).To(Equal(10 * 24 * time.Hour))
	})

	It("returns error when suffix is missing", func() {
		_, err := util.GetDurationFromString("10")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("expected format '<days>d'"))
	})

	It("returns error when days is not a positive integer", func() {
		_, err := util.GetDurationFromString("0d")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("days > 0"))

		_, err = util.GetDurationFromString("abc")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("expected format '<days>d'"))

		_, err = util.GetDurationFromString("a1d")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("expected format '<days>d'"))
	})
})
