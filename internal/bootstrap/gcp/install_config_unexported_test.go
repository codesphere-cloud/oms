// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Install config reuse - Unexported", func() {
	Describe("hasNoRequestsResourceProfile", func() {
		It("is false for a nil config", func() {
			Expect(hasNoRequestsResourceProfile(nil)).To(BeFalse())
		})

		It("is false when the config has no overrides", func() {
			Expect(hasNoRequestsResourceProfile(&files.RootConfig{})).To(BeFalse())
		})

		It("is false when the global overrides lack underprovisionFactors", func() {
			config := &files.RootConfig{}
			config.Codesphere.Override = files.ChartOverride{
				"global": map[string]any{"services": map[string]any{}},
			}

			Expect(hasNoRequestsResourceProfile(config)).To(BeFalse())
		})

		It("is true when the noRequests overrides are present", func() {
			config := &files.RootConfig{}
			config.Codesphere.Override = files.ChartOverride{
				"global": map[string]any{
					"underprovisionFactors": map[string]string{"cpu": "0.01", "memory": "0.01"},
				},
			}

			Expect(hasNoRequestsResourceProfile(config)).To(BeTrue())
		})
	})
})
