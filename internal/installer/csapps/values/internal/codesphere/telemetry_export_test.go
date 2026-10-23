// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Telemetry export", func() {
	It("traces without an endpoint -> error", func() {
		_, err := input{conf: files.CodesphereConfig{TelemetryExport: &files.TelemetryExport{Traces: true}}}.values()

		Expect(err).To(MatchError(ContainSubstring("traceEndpoint must be set")))
	})
})
