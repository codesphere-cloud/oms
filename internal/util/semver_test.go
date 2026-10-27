// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("CodesphereRelease", func() {
	It("accepts plain versions", func() {
		release, ok := util.CodesphereRelease("1.109.0")
		Expect(ok).To(BeTrue())
		Expect(release).To(Equal("v1.109.0"))
	})

	It("accepts v-prefixed versions", func() {
		release, ok := util.CodesphereRelease("v1.109.0")
		Expect(ok).To(BeTrue())
		Expect(release).To(Equal("v1.109.0"))
	})

	It("accepts portal versions", func() {
		release, ok := util.CodesphereRelease("codesphere-v1.109.0")
		Expect(ok).To(BeTrue())
		Expect(release).To(Equal("v1.109.0"))
	})

	It("rejects branch names", func() {
		_, ok := util.CodesphereRelease("feature-branch")
		Expect(ok).To(BeFalse())
	})

	It("rejects empty versions", func() {
		_, ok := util.CodesphereRelease("")
		Expect(ok).To(BeFalse())
	})
})

var _ = Describe("InstallVersionAtLeast", func() {
	It("rejects lower versions", func() {
		Expect(util.InstallVersionAtLeast("codesphere-v1.105.0", "v1.106.0")).To(BeFalse())
	})

	It("accepts exact version matches", func() {
		Expect(util.InstallVersionAtLeast("codesphere-v1.106.0", "v1.106.0")).To(BeTrue())
	})

	It("rejects invalid versions", func() {
		Expect(util.InstallVersionAtLeast("not-a-version", "v1.106.0")).To(BeFalse())
	})
})
