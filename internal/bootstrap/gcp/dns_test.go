// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/bootstrap/gcp"
)

var _ = Describe("DataCenterDNSRecordNames", func() {
	It("returns the single-DC records for one data center", func() {
		dcs := gcp.BuildDataCenters(&gcp.CodesphereEnvironment{BaseDomain: "example.com"})

		Expect(gcp.DataCenterDNSRecordNames("example.com", dcs)).To(ConsistOf(gcp.GetDNSRecordNames("example.com")))
	})

	It("shares the platform names and scopes the workspace names per data center", func() {
		dcs := gcp.BuildDataCenters(&gcp.CodesphereEnvironment{MultiDC: true, BaseDomain: "example.com"})

		Expect(gcp.DataCenterDNSRecordNames("example.com", dcs)).To(Equal([]gcp.DNSRecordName{
			{Name: "cs.example.com.", Rtype: "A"},
			{Name: "*.cs.example.com.", Rtype: "A"},
			{Name: "1.ws.example.com.", Rtype: "A"},
			{Name: "*.1.ws.example.com.", Rtype: "A"},
			{Name: "*.1.ssh.cs.example.com.", Rtype: "A"},
			{Name: "1.cs.example.com.", Rtype: "A"},
			{Name: "*.1.cs.example.com.", Rtype: "A"},
			{Name: "2.ws.example.com.", Rtype: "A"},
			{Name: "*.2.ws.example.com.", Rtype: "A"},
			{Name: "*.2.ssh.cs.example.com.", Rtype: "A"},
			{Name: "2.cs.example.com.", Rtype: "A"},
			{Name: "*.2.cs.example.com.", Rtype: "A"},
		}))
	})
})
