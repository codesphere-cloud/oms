// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package datacenters_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/datacenters"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var (
	main   = files.DatacenterConfig{ID: 1, Name: "main", City: "Karlsruhe", CountryCode: "DE"}
	second = files.DatacenterConfig{ID: 2, Name: "second", City: "Frankfurt", CountryCode: "DE"}
)

var _ = Describe("Values", func() {
	It("several data centers -> sorted by id, configured default", func() {
		values, err := datacenters.Values(&files.RootConfig{
			Datacenter:          main,
			DataCenters:         []files.DatacenterConfig{second, main},
			DefaultDataCenterID: 2,
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(values).To(Equal(datacenters.DataCenters{
			DataCenterID:         1,
			DataCenterName:       "main",
			DefaultDataCenterID:  2,
			AvailableDataCenters: []int{1, 2},
			DataCenters: []datacenters.DataCenter{
				{ID: 1, Name: "main", City: "Karlsruhe", CountryCode: "DE"},
				{ID: 2, Name: "second", City: "Frankfurt", CountryCode: "DE"},
			},
		}))
	})

	It("second data center's config without a default -> second is target and default", func() {
		values, err := datacenters.Values(&files.RootConfig{
			Datacenter:  second,
			DataCenters: []files.DatacenterConfig{main, second},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(values.DataCenterID).To(Equal(2))
		Expect(values.DataCenterName).To(Equal("second"))
		Expect(values.DefaultDataCenterID).To(Equal(2))
		Expect(values.AvailableDataCenters).To(Equal([]int{1, 2}))
	})

	It("no .dataCenters -> only the target one, which is the default", func() {
		values, err := datacenters.Values(&files.RootConfig{Datacenter: second})
		Expect(err).NotTo(HaveOccurred())

		Expect(values.AvailableDataCenters).To(Equal([]int{2}))
		Expect(values.DefaultDataCenterID).To(Equal(2))
	})

	It("data center with id 0 -> error", func() {
		_, err := datacenters.Values(&files.RootConfig{
			Datacenter:  main,
			DataCenters: []files.DatacenterConfig{main, {ID: 0, Name: "zero"}},
		})

		Expect(err).To(MatchError(ContainSubstring(`data center "zero" has id 0`)))
	})

	It("duplicate id -> error", func() {
		_, err := datacenters.Values(&files.RootConfig{
			Datacenter:  main,
			DataCenters: []files.DatacenterConfig{main, second, main},
		})

		Expect(err).To(MatchError(ContainSubstring("duplicate data center id 1")))
	})

	It("data centers without the target one -> error", func() {
		_, err := datacenters.Values(&files.RootConfig{
			Datacenter:  main,
			DataCenters: []files.DatacenterConfig{second},
		})

		Expect(err).To(MatchError(ContainSubstring("is missing .dataCenter (id 1)")))
	})

	It("target entry that differs from .dataCenter -> error", func() {
		renamed := main
		renamed.Name = "renamed"

		_, err := datacenters.Values(&files.RootConfig{
			Datacenter:  main,
			DataCenters: []files.DatacenterConfig{renamed, second},
		})

		Expect(err).To(MatchError(ContainSubstring("entry with id 1 differs from .dataCenter")))
	})

	It("unknown default -> error", func() {
		_, err := datacenters.Values(&files.RootConfig{
			Datacenter:          main,
			DataCenters:         []files.DatacenterConfig{main, second},
			DefaultDataCenterID: 3,
		})

		Expect(err).To(MatchError(ContainSubstring(".defaultDataCenterId 3 is not one")))
	})
})
