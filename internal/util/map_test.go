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

var _ = Describe("MergeChartValues", func() {
	It("override with a numeric YAML key -> string keyed, so the values marshal to JSON", func() {
		override := map[string]any{"pool": map[any]any{21: map[any]any{"size": 2}}}

		merged := util.MergeChartValues(map[string]any{"base": true}, override)

		Expect(merged).To(Equal(map[string]any{
			"base": true,
			"pool": map[string]any{"21": map[string]any{"size": 2}},
		}))
	})

	It("override -> the override map is not changed through the result", func() {
		override := map[string]any{"nested": map[string]any{"a": 1}}

		merged := util.MergeChartValues(map[string]any{}, override)
		merged["nested"].(map[string]any)["a"] = 2

		Expect(override).To(Equal(map[string]any{"nested": map[string]any{"a": 1}}))
	})
})
