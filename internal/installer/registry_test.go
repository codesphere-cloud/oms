// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Registry normalization", func() {
	It("rejects uppercase repository paths", func() {
		_, err := installer.NormalizeRegistry("registry.example.com/Team")
		Expect(err).To(MatchError(ContainSubstring("invalid image registry")))
	})

	It("rejects tags embedded in the registry path", func() {
		_, err := installer.NormalizeRegistry("registry.example.com/repo:tag")
		Expect(err).To(MatchError(ContainSubstring("invalid image registry")))
	})

	It("preserves ports and normalizes nested repository paths", func() {
		Expect(installer.NormalizeRegistry("registry.example.com:5000/team//mirror/")).To(Equal("registry.example.com:5000/team/mirror"))
	})

	It("accepts a localhost registry with a port", func() {
		Expect(installer.NormalizeRegistry("localhost:5000")).To(Equal("localhost:5000"))
	})
})
