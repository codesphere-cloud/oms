// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("StringSliceToBoolMap", func() {
	It("maps every element to true", func() {
		Expect(util.StringSliceToBoolMap([]string{"a", "b"})).To(Equal(map[string]bool{
			"a": true,
			"b": true,
		}))
	})

	It("returns nil for a nil slice", func() {
		Expect(util.StringSliceToBoolMap(nil)).To(BeNil())
	})

	It("returns an empty map for an empty slice", func() {
		result := util.StringSliceToBoolMap([]string{})
		Expect(result).NotTo(BeNil())
		Expect(result).To(BeEmpty())
	})

	It("deduplicates repeated elements", func() {
		Expect(util.StringSliceToBoolMap([]string{"a", "a"})).To(Equal(map[string]bool{"a": true}))
	})
})
