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

var _ = Describe("Resolve", func() {
	It("several data centers -> sorted by id, configured default", func() {
		topology, err := datacenters.Resolve(&files.RootConfig{
			Datacenter:          main,
			DataCenters:         []files.DatacenterConfig{second, main},
			DefaultDataCenterID: 2,
		})
		Expect(err).NotTo(HaveOccurred())

		values := datacenters.Values(topology)

		Expect(values["availableDataCenters"]).To(Equal([]int{1, 2}))
		Expect(values["dataCenterId"]).To(Equal(1))
		Expect(values["defaultDataCenterId"]).To(Equal(2))
	})

	It("data centers without the installed one -> error", func() {
		_, err := datacenters.Resolve(&files.RootConfig{
			Datacenter:  main,
			DataCenters: []files.DatacenterConfig{second},
		})

		Expect(err).To(MatchError(ContainSubstring("missing in .dataCenters")))
	})
})
