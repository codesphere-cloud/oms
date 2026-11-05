// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package values_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values"
)

var _ = Describe("MergeOverride", func() {
	It("override with a numeric YAML key -> string keyed, so the values marshal to JSON", func() {
		override := map[string]any{"pool": map[any]any{21: map[any]any{"size": 2}}}

		merged := values.MergeOverride(map[string]any{"base": true}, override)

		Expect(merged).To(Equal(map[string]any{
			"base": true,
			"pool": map[string]any{"21": map[string]any{"size": 2}},
		}))
	})

	It("override -> the override map is not changed through the result", func() {
		override := map[string]any{"nested": map[string]any{"a": 1}}

		merged := values.MergeOverride(map[string]any{}, override)
		merged["nested"].(map[string]any)["a"] = 2

		Expect(override).To(Equal(map[string]any{"nested": map[string]any{"a": 1}}))
	})
})
