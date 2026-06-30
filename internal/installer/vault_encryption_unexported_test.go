// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("unwrapSOPSData", func() {
	It("returns data unchanged when there is no data: wrapper", func() {
		input := []byte("secrets:\n    - name: foo\n      fields:\n        password: bar\n")
		output := unwrapSOPSData(input)
		Expect(string(output)).To(Equal(string(input)))
	})

	It("strips a top-level data: | wrapper and returns inner content", func() {
		input := []byte("data: |\n    secrets:\n        - name: foo\n          fields:\n            password: bar\n")
		output := unwrapSOPSData(input)
		Expect(string(output)).To(Equal("secrets:\n    - name: foo\n      fields:\n        password: bar\n"))
	})

	It("returns data unchanged for an empty document", func() {
		input := []byte("")
		output := unwrapSOPSData(input)
		Expect(string(output)).To(Equal(string(input)))
	})

	It("returns data unchanged when root has multiple keys", func() {
		input := []byte("data: some-value\nsops:\n  key: val\n")
		output := unwrapSOPSData(input)
		Expect(string(output)).To(Equal(string(input)))
	})

	It("returns data unchanged for invalid YAML", func() {
		input := []byte("not: valid: yaml: [[")
		output := unwrapSOPSData(input)
		Expect(string(output)).To(Equal(string(input)))
	})

	It("returns data unchanged when data is not a scalar", func() {
		input := []byte("data:\n  nested: value\n")
		output := unwrapSOPSData(input)
		Expect(string(output)).To(Equal(string(input)))
	})
})
