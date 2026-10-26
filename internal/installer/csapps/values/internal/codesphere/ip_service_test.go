// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("IP service", func() {
	metalLB := &files.MetalLBConfig{
		Enabled: true,
		Pools:   []files.MetalLBPoolDef{{Name: "public", IPAddresses: []string{"203.0.113.0/28"}}},
	}

	It("enabled MetalLB without an IP service -> all its pools", func() {
		values, err := codesphere.Values(files.CodesphereConfig{}, defaultTestImages(), defaultTestGateways(), metalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		Expect(values.IPService).To(Equal(&codesphere.IPService{
			LoadBalancerKind: "metallb",
			AddressPools:     []string{"public"},
		}))
	})

	It("IP service with a pool MetalLB does not define -> error", func() {
		conf := files.CodesphereConfig{
			IPService: &files.IPServiceConfig{LoadBalancerKind: "metallb", AddressPools: []string{"private"}},
		}

		_, err := codesphere.Values(conf, defaultTestImages(), defaultTestGateways(), metalLB, noRegistry)

		Expect(err).To(MatchError(ContainSubstring("pool private is not defined")))
	})
})
