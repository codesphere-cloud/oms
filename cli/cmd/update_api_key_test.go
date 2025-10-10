// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/portal"
)

var _ = Describe("UpdateAPIKey", func() {

	var (
		mockPortal *portal.MockPortal
		c          cmd.UpdateAPIKeyCmd
	)

	BeforeEach(func() {
		mockPortal = portal.NewMockPortal(GinkgoT())
		c = cmd.UpdateAPIKeyCmd{}
	})

	Describe("Run", func() {
		It("successfully updates the API key when given valid input", func() {
			apiKeyID := "aaaaaaaaaaaaaaaaaaaaaa"
			expiresAtStr := "2027-12-31T23:59:59Z"
			expectedExpiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
			Expect(err).NotTo(HaveOccurred())

			c.Opts.APIKeyID = apiKeyID
			c.Opts.ExpiresAtStr = expiresAtStr

			mockPortal.EXPECT().UpdateAPIKey(apiKeyID, expectedExpiresAt).Return(nil)

			err = c.UpdateAPIKey(mockPortal)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error for an invalid api key id format", func() {
			apiKeyID := "not-a-valid-id"
			expiresAtStr := "2027-12-31T23:59:59Z"
			expectedExpiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
			Expect(err).NotTo(HaveOccurred())

			c.Opts.APIKeyID = apiKeyID
			c.Opts.ExpiresAtStr = expiresAtStr

			mockPortal.EXPECT().UpdateAPIKey(apiKeyID, expectedExpiresAt).Return(fmt.Errorf("invalid api key id format"))

			err = c.UpdateAPIKey(mockPortal)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid api key id format"))
		})

		It("returns an error for an invalid date format", func() {
			c.Opts.APIKeyID = "valid id"
			c.Opts.ExpiresAtStr = "2025/123/123"

			err := c.UpdateAPIKey(mockPortal)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid date format"))
		})
	})
})
