// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = DescribeTable("hasNoRequestsResourceProfile",
	func(config *files.RootConfig, expected bool) {
		Expect(hasNoRequestsResourceProfile(config)).To(Equal(expected))
	},
	Entry("nil config", nil, false),
	Entry("no overrides", &files.RootConfig{}, false),
	Entry("global overrides without underprovisionFactors", &files.RootConfig{Codesphere: files.CodesphereConfig{
		Override: files.ChartOverride{"global": map[string]any{"services": map[string]any{}}},
	}}, false),
	Entry("noRequests overrides", &files.RootConfig{Codesphere: files.CodesphereConfig{
		Override: files.ChartOverride{"global": map[string]any{"underprovisionFactors": map[string]string{"cpu": "0.01"}}},
	}}, true),
)
