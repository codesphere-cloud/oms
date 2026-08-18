// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("AppendUnique", func() {
	It("appends only values that are not already present", func() {
		Expect(util.AppendUnique([]string{"one", "two"}, "two", "three", "three")).
			To(Equal([]string{"one", "two", "three"}))
	})

	It("preserves a nil slice when there are no additions", func() {
		Expect(util.AppendUnique[string](nil)).To(BeNil())
	})

	It("supports comparable non-string values", func() {
		Expect(util.AppendUnique([]int{1}, 2, 1)).To(Equal([]int{1, 2}))
	})
})
