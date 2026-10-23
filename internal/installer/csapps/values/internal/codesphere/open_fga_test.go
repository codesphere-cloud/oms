// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("OpenFGA", func() {
	It("OpenFGA deployed in this data center -> chart default API URL kept", func() {
		values := input{conf: `
			openFga:
			  deploy: true
			  expose: {enabled: true, host: fga.example.com}
		`}.mustValues()

		Expect(values.OpenFGA).To(BeNil())
	})
})
