// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/cluster"
)

var _ = Describe("Values", func() {
	It("extra workspace files -> name, content and encoding passed on", func() {
		values := input{conf: `
			extraWorkspaceFiles:
			  - {name: .npmrc, content: cmVnaXN0cnk=, encoded: true}
		`}.mustValues()

		Expect(values["extraWorkspaceFiles"]).To(Equal([]any{
			map[string]any{"name": ".npmrc", "content": "cmVnaXN0cnk=", "encoded": true},
		}))
	})

	It("disabled gateway APIs -> no gateway domains experiment", func() {
		values := input{gateways: &cluster.Gateways{}}.mustValues()

		Expect(values["frontendGateway"]).To(
			HaveKeyWithValue("gatewayApi", map[string]any{"enabled": false}),
		)
		Expect(values["customDomains"]).To(
			HaveKeyWithValue("gatewayApi", map[string]any{"useGateway": false}),
		)
		Expect(values["experiments"]).NotTo(HaveKey("gateway-domains"))
	})
})
