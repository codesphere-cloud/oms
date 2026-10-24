// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DenderTemplate", func() {
	It("replaces a single variable", func() {
		tpl := []byte(`name: "dc-${DC_NUMBER}"`)
		rendered, err := util.RenderTemplate(tpl, map[string]string{
			"DC_NUMBER": "5",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(string(rendered)).To(Equal(`name: "dc-5"`))
	})

	It("replaces multiple variables", func() {
		tpl := []byte(`server: ${HOST}, port: ${PORT}`)
		rendered, err := util.RenderTemplate(tpl, map[string]string{
			"HOST": "localhost",
			"PORT": "8080",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(string(rendered)).To(Equal(`server: localhost, port: 8080`))
	})

	It("replaces multiple occurrences of the same variable", func() {
		tpl := []byte(`a: ${VAR}, b: ${VAR}`)
		rendered, err := util.RenderTemplate(tpl, map[string]string{
			"VAR": "value",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(string(rendered)).To(Equal(`a: value, b: value`))
	})

	It("leaves unmatched placeholders intact", func() {
		tpl := []byte(`name: ${KNOWN}, secret: ${UNKNOWN}`)
		rendered, err := util.RenderTemplate(tpl, map[string]string{
			"KNOWN": "replaced",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(string(rendered)).To(ContainSubstring("replaced"))
		Expect(string(rendered)).To(ContainSubstring("${UNKNOWN}"))
	})

	It("returns input unchanged when vars map is empty", func() {
		tpl := []byte(`nothing: to replace`)
		rendered, err := util.RenderTemplate(tpl, map[string]string{})
		Expect(err).ToNot(HaveOccurred())
		Expect(string(rendered)).To(Equal(`nothing: to replace`))
	})
})
