// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Flags", func() {
	It("flags -> experiments and features", func() {
		values := input{conf: `
			experiments: [legacy]
			internal: [internal-flag]
			preview: {preview-on: true, preview-off: false}
			features: {feature-on: true, feature-off: false}
		`}.mustValues()

		experiments := values["experiments"].(map[string]any)
		Expect(experiments).To(HaveKeyWithValue("legacy", []string{"prod"}))
		Expect(experiments).To(HaveKeyWithValue("internal-flag", []string{"prod"}))
		Expect(experiments).To(HaveKeyWithValue("preview-on", []string{"prod"}))
		Expect(experiments).NotTo(HaveKey("preview-off"))

		features := values["features"].(map[string]any)
		Expect(features).To(HaveKeyWithValue("feature-on", []string{"prod"}))
		Expect(features).To(HaveKeyWithValue("feature-off", []string{}))
	})
})
