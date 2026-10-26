// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("OpenFGA", func() {
	It("OpenFGA deployed and published for other data centers -> in-cluster API URL kept", func() {
		conf := files.CodesphereConfig{OpenFga: &files.OpenFgaConfig{
			Deploy: ptr.To(true),
			// The published host is for the other data centers; this one keeps the in-cluster URL.
			Expose: &files.OpenFgaExposeConfig{Enabled: true, Host: "fga.example.com"},
		}}

		values, err := codesphere.Values(conf, defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		Expect(values.OpenFGA).To(BeNil())
	})
})
