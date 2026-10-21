// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Telemetry export", func() {
	It("traces without an endpoint -> error", func() {
		_, err := input{conf: `telemetryExport: {traces: true}`}.values()

		Expect(err).To(MatchError(ContainSubstring("traceEndpoint must be set")))
	})
})
