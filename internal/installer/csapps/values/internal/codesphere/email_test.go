// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("SMTP", func() {
	It("email provider other than smtp -> error", func() {
		conf := files.CodesphereConfig{Email: &files.EmailConfig{
			Provider: "sendgrid",
			SMTP:     files.SMTPConfig{Host: "smtp.example.com", Port: 587},
		}}

		_, err := codesphere.Values(conf, defaultTestInputs())

		Expect(err).To(MatchError(`email.provider must be "smtp", got "sendgrid"`))
	})

	It("email provider unset -> error", func() {
		conf := files.CodesphereConfig{Email: &files.EmailConfig{
			SMTP: files.SMTPConfig{Host: "smtp.example.com", Port: 587},
		}}

		_, err := codesphere.Values(conf, defaultTestInputs())

		Expect(err).To(MatchError(`email.provider must be "smtp", got ""`))
	})
})
