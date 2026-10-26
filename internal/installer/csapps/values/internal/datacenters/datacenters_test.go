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
	primary   = files.DatacenterConfig{ID: 1, Name: "primary", City: "Karlsruhe", CountryCode: "DE"}
	secondary = files.DatacenterConfig{ID: 2, Name: "secondary", City: "Frankfurt", CountryCode: "DE"}
)

var _ = Describe("Values", func() {
	It("several data centers -> sorted by id, configured default", func() {
		values, err := datacenters.Values(&files.RootConfig{
			Datacenter:          primary,
			DataCenters:         []files.DatacenterConfig{secondary, primary},
			DefaultDataCenterID: 2,
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(values).To(Equal(datacenters.DataCenters{
			DataCenterID:         1,
			DataCenterName:       "primary",
			DefaultDataCenterID:  2,
			AvailableDataCenters: []int{1, 2},
			DataCenters:          []files.DatacenterConfig{primary, secondary},
		}))
	})

	It("secondary data center's config without a default -> secondary is target and default", func() {
		values, err := datacenters.Values(&files.RootConfig{
			Datacenter:  secondary,
			DataCenters: []files.DatacenterConfig{primary, secondary},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(values.DataCenterID).To(Equal(2))
		Expect(values.DataCenterName).To(Equal("secondary"))
		Expect(values.DefaultDataCenterID).To(Equal(2))
		Expect(values.AvailableDataCenters).To(Equal([]int{1, 2}))
	})

	It("no .dataCenters -> only the target one, which is the default", func() {
		values, err := datacenters.Values(&files.RootConfig{Datacenter: secondary})
		Expect(err).NotTo(HaveOccurred())

		Expect(values.AvailableDataCenters).To(Equal([]int{2}))
		Expect(values.DefaultDataCenterID).To(Equal(2))
	})

	It("invalid data centers -> error", func() {
		_, err := datacenters.Values(&files.RootConfig{
			Datacenter:  primary,
			DataCenters: []files.DatacenterConfig{secondary},
		})

		Expect(err).To(MatchError(ContainSubstring("is missing .dataCenter (id 1)")))
	})
})
