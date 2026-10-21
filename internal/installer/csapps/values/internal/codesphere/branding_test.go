// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Branding", func() {
	It("branding without showOnboarding -> onboarding off", func() {
		values := input{conf: `branding: {title: Example IDE}`}.mustValues()

		Expect(values["branding"]).To(Equal(map[string]any{
			"title":          "Example IDE",
			"showOnboarding": false,
		}))
	})
})
