// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Ceph", func() {
	Describe("ParseMonitorEndpointHost", func() {
		DescribeTable("parses monitor endpoint hosts",
			func(input, expected string) {
				host, err := util.ParseMonitorEndpointHost(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(host).To(Equal(expected))
			},
			Entry("plain IP:port", "a=10.0.0.10:6789", "10.0.0.10:6789"),
			Entry("msgr2 format", "a=[v2:10.0.0.10:3300/0,v1:10.0.0.10:6789/0]", "10.0.0.10:3300"),
			Entry("service DNS with port", "a=rook-ceph-mon-a.rook-ceph.svc:6789", "rook-ceph-mon-a.rook-ceph.svc:6789"),
			Entry("service DNS without port", "a=rook-ceph-mon-a.rook-ceph.svc", "rook-ceph-mon-a.rook-ceph.svc"),
		)
	})
})
