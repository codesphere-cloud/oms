// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
)

var _ = Describe("Branding", func() {
	It("branding with only a title -> only the title", func() {
		values := input{conf: `branding: {title: Example IDE}`}.mustValues()

		Expect(values.Branding).To(Equal(&codesphere.Branding{Title: "Example IDE"}))
	})
})
