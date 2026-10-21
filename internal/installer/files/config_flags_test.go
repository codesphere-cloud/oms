// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package files_test

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CodesphereConfig.IsEnabled", func() {
	DescribeTable("checks each flag source", func(config files.CodesphereConfig, enabled bool) {
		Expect(config.IsEnabled("target")).To(Equal(enabled))
	},
		Entry("internal", files.CodesphereConfig{Internal: []string{"other", "target"}}, true),
		Entry("preview", files.CodesphereConfig{Preview: map[string]bool{"target": true}}, true),
		Entry("feature", files.CodesphereConfig{Features: map[string]bool{"target": true}}, true),
		Entry("disabled", files.CodesphereConfig{Preview: map[string]bool{"target": false}, Features: map[string]bool{"target": false}}, false),
		Entry("unrelated", files.CodesphereConfig{Internal: []string{"other"}, Preview: map[string]bool{"other": true}, Features: map[string]bool{"other": true}}, false),
		Entry("empty", files.CodesphereConfig{}, false),
	)
})
