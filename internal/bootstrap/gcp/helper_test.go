// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("calculateProjectExpiryLabel", func() {
	const customDateFormat string = "2006-01-02_15-04-05_utc"
	const customDateFormatRegex string = `^\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}_utc$`

	type validTestCase struct {
		inputTTL         string
		expectedDuration time.Duration
	}

	DescribeTable("calculating the expiry label from string durations",
		func(tc validTestCase) {
			label, err := calculateProjectExpiryLabel(tc.inputTTL)
			Expect(err).NotTo(HaveOccurred())
			Expect(label).To(MatchRegexp(customDateFormatRegex))

			parsedTime, err := time.Parse(customDateFormat, label)
			Expect(err).NotTo(HaveOccurred())

			expectedTime := time.Now().UTC().Add(tc.expectedDuration)

			Expect(parsedTime).To(BeTemporally("~", expectedTime, 10*time.Second))
		},

		// Define the table rows using the temp struct
		Entry("1 Minute", validTestCase{
			inputTTL:         "1m",
			expectedDuration: 1 * time.Minute,
		}),
		Entry("1 hour", validTestCase{
			inputTTL:         "1h",
			expectedDuration: 1 * time.Hour,
		}),
		Entry("1 Day", validTestCase{
			inputTTL:         "24h",
			expectedDuration: 24 * time.Hour,
		}),
	)

	It("returns an error for invalid duration strings", func() {
		_, err := calculateProjectExpiryLabel("1d") // 'd' is not a valid time unit in Go's duration parsing
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid project TTL format"))
	})
})
