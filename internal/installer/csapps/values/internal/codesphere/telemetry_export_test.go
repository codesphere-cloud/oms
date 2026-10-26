// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Telemetry export", func() {
	It("traces without an endpoint -> error", func() {
		conf := files.CodesphereConfig{TelemetryExport: &files.TelemetryExport{Traces: true}}

		_, err := codesphere.Values(conf, defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)

		Expect(err).To(MatchError(ContainSubstring("traceEndpoint must be set")))
	})

	It("no telemetry config -> disabled exports and default central collector", func() {
		values, err := codesphere.Values(files.CodesphereConfig{}, defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		Expect(values.Otel).To(Equal(codesphere.Otel{
			CentralOtelExport: codesphere.CentralOtelExport{
				Enabled:              false,
				TelemetryCredentials: codesphere.TelemetryCredentials{SecretName: "otel-collector-central-creds"},
				OTLPHTTPEndpoint:     "https://otel.tools.codesphere.com",
			},
		}))
	})

	It("remote export without an endpoint -> default central endpoint", func() {
		conf := files.CodesphereConfig{TelemetryExport: &files.TelemetryExport{RemoteExport: true}}

		values, err := codesphere.Values(conf, defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		Expect(values.Otel.CentralOtelExport.Enabled).To(BeTrue())
		Expect(values.Otel.CentralOtelExport.OTLPHTTPEndpoint).To(Equal("https://otel.tools.codesphere.com"))
	})
})
