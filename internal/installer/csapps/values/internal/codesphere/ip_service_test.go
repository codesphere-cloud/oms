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
	withMetalLB := func(metalLB *files.MetalLBConfig) codesphere.Inputs {
		inputs := defaultTestInputs()
		inputs.MetalLB = metalLB

		return inputs
	}
	metalLB := &files.MetalLBConfig{
		Enabled: true,
		Pools: []files.MetalLBPoolDef{
			{Name: "public", IPAddresses: []string{"203.0.113.0/28"}},
			{Name: "internal", IPAddresses: []string{"198.51.100.0/28"}},
		},
	}
	disabledMetalLB := &files.MetalLBConfig{Enabled: false, Pools: metalLB.Pools}

	It("enabled MetalLB without an IP service -> all its pools", func() {
		values, err := codesphere.Values(files.CodesphereConfig{}, withMetalLB(metalLB))
		Expect(err).NotTo(HaveOccurred())

		Expect(values.IPService).To(Equal(&codesphere.IPService{
			LoadBalancerKind: "metallb",
			AddressPools:     []string{"public", "internal"},
		}))
	})

	It("enabled MetalLB with an IP service using one of its pools -> that pool", func() {
		conf := files.CodesphereConfig{
			IPService: &files.IPServiceConfig{LoadBalancerKind: "metallb", AddressPools: []string{"public"}},
		}

		values, err := codesphere.Values(conf, withMetalLB(metalLB))
		Expect(err).NotTo(HaveOccurred())

		Expect(values.IPService).To(Equal(&codesphere.IPService{
			LoadBalancerKind: "metallb",
			AddressPools:     []string{"public"},
		}))
	})

	It("enabled MetalLB with an IP service using a pool MetalLB does not define -> error", func() {
		conf := files.CodesphereConfig{
			IPService: &files.IPServiceConfig{LoadBalancerKind: "metallb", AddressPools: []string{"private"}},
		}

		_, err := codesphere.Values(conf, withMetalLB(metalLB))

		Expect(err).To(MatchError(ContainSubstring("pool private is not defined")))
	})

	It("enabled MetalLB with an IP service of another kind -> error", func() {
		conf := files.CodesphereConfig{IPService: &files.IPServiceConfig{LoadBalancerKind: "cloud"}}

		_, err := codesphere.Values(conf, withMetalLB(metalLB))

		Expect(err).To(MatchError(ContainSubstring("codesphere.ipService must also use metallb")))
	})

	It("no MetalLB and no IP service -> no IP service", func() {
		values, err := codesphere.Values(files.CodesphereConfig{}, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

		Expect(values.IPService).To(BeNil())
	})

	It("disabled MetalLB and no IP service -> no IP service", func() {
		values, err := codesphere.Values(files.CodesphereConfig{}, withMetalLB(disabledMetalLB))
		Expect(err).NotTo(HaveOccurred())

		Expect(values.IPService).To(BeNil())
	})

	It("disabled MetalLB with an IP service using an undefined pool -> configured IP service", func() {
		conf := files.CodesphereConfig{
			IPService: &files.IPServiceConfig{LoadBalancerKind: "metallb", AddressPools: []string{"private"}},
		}

		values, err := codesphere.Values(conf, withMetalLB(disabledMetalLB))
		Expect(err).NotTo(HaveOccurred())

		Expect(values.IPService).To(Equal(&codesphere.IPService{
			LoadBalancerKind: "metallb",
			AddressPools:     []string{"private"},
		}))
	})

	It("no MetalLB with an IP service of another kind -> configured IP service", func() {
		conf := files.CodesphereConfig{IPService: &files.IPServiceConfig{LoadBalancerKind: "cloud"}}

		values, err := codesphere.Values(conf, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

		Expect(values.IPService).To(Equal(&codesphere.IPService{LoadBalancerKind: "cloud"}))
	})
})
