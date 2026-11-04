// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("SignUp", func() {
	It("signUp unset -> no allowed domains", func() {
		values, err := codesphere.Values(files.CodesphereConfig{}, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

		Expect(values.SignUp).To(Equal(codesphere.SignUp{}))
	})

	It("signUp without email -> no allowed domains", func() {
		conf := files.CodesphereConfig{SignUp: &files.SignUpConfig{}}

		values, err := codesphere.Values(conf, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

		Expect(values.SignUp).To(Equal(codesphere.SignUp{}))
	})
})
