// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/cluster"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
)

var _ = Describe("Values", func() {
	It("extra workspace files -> name, content and encoding passed on", func() {
		values := input{conf: `
			extraWorkspaceFiles:
			  - {name: .npmrc, content: cmVnaXN0cnk=, encoded: true}
		`}.mustValues()

		Expect(values.ExtraWorkspaceFiles).To(Equal([]codesphere.ExtraWorkspaceFile{
			{Name: ".npmrc", Content: "cmVnaXN0cnk=", Encoded: true},
		}))
	})

	It("disabled gateway APIs -> no gateway domains experiment", func() {
		values := input{gateways: &cluster.Gateways{}}.mustValues()

		Expect(values.FrontendGateway.GatewayAPI.Enabled).To(BeFalse())
		Expect(values.CustomDomains.GatewayAPI.UseGateway).To(BeFalse())
		Expect(values.Experiments).NotTo(HaveKey("gateway-domains"))
	})
})
