// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

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
			validFor := "10d"
			expectedExpiresAt := time.Now().Add(10 * 24 * time.Hour)

			c.Opts.APIKeyID = apiKeyID
			c.Opts.ValidFor = validFor

			mockPortal.EXPECT().UpdateAPIKey(apiKeyID, mock.Anything).
				RunAndReturn(func(id string, gotExpiresAt time.Time) error {
					Expect(id).To(Equal(apiKeyID))
					Expect(gotExpiresAt).To(BeTemporally("~", expectedExpiresAt, 5*time.Second))
					return nil
				})

			err := c.UpdateAPIKey(mockPortal)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error for an invalid api key id format", func() {
			apiKeyID := "not-a-valid-id"
			validFor := "5d"
			expectedExpiresAt := time.Now().Add(5 * 24 * time.Hour)

			c.Opts.APIKeyID = apiKeyID
			c.Opts.ValidFor = validFor

			mockPortal.EXPECT().UpdateAPIKey(apiKeyID, mock.Anything).
				RunAndReturn(func(id string, gotExpiresAt time.Time) error {
					Expect(id).To(Equal(apiKeyID))
					Expect(gotExpiresAt).To(BeTemporally("~", expectedExpiresAt, 5*time.Second))
					return fmt.Errorf("invalid api key id format")
				})

			err := c.UpdateAPIKey(mockPortal)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid api key id format"))
		})

		It("returns an error for an invalid valid-for format", func() {
			c.Opts.APIKeyID = "valid id"
			c.Opts.ValidFor = "invalid-date"

			err := c.UpdateAPIKey(mockPortal)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse valid-for duration"))
		})
	})
})
