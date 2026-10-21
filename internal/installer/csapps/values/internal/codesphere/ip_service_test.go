// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("IP service", func() {
	metalLB := &files.MetalLBConfig{
		Enabled: true,
		Pools:   []files.MetalLBPoolDef{{Name: "public", IPAddresses: []string{"203.0.113.0/28"}}},
	}

	It("enabled MetalLB without an IP service -> all its pools", func() {
		values := input{metalLB: metalLB}.mustValues()

		Expect(values["ipService"]).To(Equal(map[string]any{
			"loadBalancerKind": "metallb",
			"addressPools":     []string{"public"},
		}))
	})

	It("IP service with a pool MetalLB does not define -> error", func() {
		_, err := input{
			conf:    `ipService: {loadBalancerKind: metallb, addressPools: [private]}`,
			metalLB: metalLB,
		}.values()

		Expect(err).To(MatchError(ContainSubstring("pool private is not defined")))
	})
})
