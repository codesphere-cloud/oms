// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package env_test

import (
	"strings"

	"github.com/codesphere-cloud/oms/internal/env"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Environment", func() {
	Describe("GetOmsPortalApiKey", func() {
		var e *env.Environment

		BeforeEach(func() {
			e = env.NewEnv()
		})

		It("returns the API key in the new format", func() {
			newKey := "new-long-api-key-format-very-long-string"
			GinkgoT().Setenv("OMS_PORTAL_API_KEY", newKey)

			apiKey, err := e.GetOmsPortalApiKey()
			Expect(err).NotTo(HaveOccurred())
			Expect(apiKey).To(Equal(newKey))
		})

		It("returns an error if the API key is not set", func() {
			GinkgoT().Setenv("OMS_PORTAL_API_KEY", "")

			_, err := e.GetOmsPortalApiKey()
			Expect(err).To(MatchError(ContainSubstring("not set")))
		})

		It("returns an error if the API key uses the old format", func() {
			GinkgoT().Setenv("OMS_PORTAL_API_KEY", strings.Repeat("a", 25))

			_, err := e.GetOmsPortalApiKey()
			Expect(err).To(MatchError(env.ErrLegacyApiKey))
		})
	})

	Describe("IsLegacyApiKey", func() {
		DescribeTable("detects old API keys",
			func(apiKey string, expected bool) {
				Expect(env.IsLegacyApiKey(apiKey)).To(Equal(expected))
			},
			Entry("old format", strings.Repeat("a", 25), true),
			Entry("new format", "new-long-api-key-format-very-long-string", false),
			Entry("empty key", "", false),
		)
	})
})
