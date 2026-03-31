// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("IAM & Admin - Unexported", func() {
	Describe("calculateProjectExpiryLabel", func() {
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

			// Define test scenarios
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
			label, err := calculateProjectExpiryLabel("1d") // 'd' is not a valid time unit in Go's duration parsing
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid project TTL format"))
			Expect(label).To(BeEmpty())
		})
	})

	Describe("createProjectLabel", func() {
		type validTestCase struct {
			inputValue    string
			expectedLabel string
		}

		DescribeTable("", func(tc validTestCase) {
			actualLabel, err := createLabel(tc.inputValue)
			Expect(err).ToNot(HaveOccurred())
			Expect(actualLabel).To(Equal(tc.expectedLabel))
		},

			Entry("master", validTestCase{
				inputValue:    "master",
				expectedLabel: "master",
			}),
			Entry("codesphere-v1.23.4", validTestCase{
				inputValue:    "codesphere-v1.23.4",
				expectedLabel: "codesphere-v1_23_4",
			}),
			Entry("feat/my-branch-name", validTestCase{
				inputValue:    "feat/my-branch-name",
				expectedLabel: "feat_my-branch-name",
			}),
			Entry("long label is cut after 64 chars", validTestCase{
				inputValue:    "this/is.averylongvaluewhichexceedsthemaximumlengthofagcpprojectlabel",
				expectedLabel: "this_is_averylongvaluewhichexceedsthemaximumlengthofagcpproject",
			}),
			Entry("uppercase to lowercase", validTestCase{
				inputValue:    "Master",
				expectedLabel: "master",
			}),
			Entry("timestamp format is accepted", validTestCase{
				inputValue:    "2026-03-31_11-36-55_utc",
				expectedLabel: "2026-03-31_11-36-55_utc",
			}),
		)

		It("returns an error for empty input", func() {
			label, err := createLabel("")
			Expect(err).To(HaveOccurred())
			Expect(label).To(BeEmpty())
		})
	})
})
