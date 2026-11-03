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
	It("no openFga block -> in-cluster API URL kept", func() {
		values, err := codesphere.Values(files.CodesphereConfig{}, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

		Expect(values.OpenFGA).To(BeNil())
	})

	It("OpenFGA of another data center -> its API URL", func() {
		conf := files.CodesphereConfig{OpenFga: &files.OpenFgaConfig{
			Deploy: ptr.To(false),
			APIURL: "https://fga.example.com",
		}}

		values, err := codesphere.Values(conf, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

		Expect(values.OpenFGA).To(Equal(&codesphere.OpenFGA{APIURL: "https://fga.example.com"}))
	})

	It("OpenFGA deployed with an explicit API URL -> that API URL", func() {
		conf := files.CodesphereConfig{OpenFga: &files.OpenFgaConfig{
			Deploy: ptr.To(true),
			APIURL: "https://fga.internal.example.com",
		}}

		values, err := codesphere.Values(conf, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

		Expect(values.OpenFGA).To(Equal(&codesphere.OpenFGA{APIURL: "https://fga.internal.example.com"}))
	})

	It("OpenFGA deployed and published for other data centers -> in-cluster API URL kept", func() {
		conf := files.CodesphereConfig{OpenFga: &files.OpenFgaConfig{
			Deploy: ptr.To(true),
			// The published host is for the other data centers; this one keeps the in-cluster URL.
			Expose: &files.OpenFgaExposeConfig{Enabled: true, Host: "fga.example.com"},
		}}

		values, err := codesphere.Values(conf, defaultTestInputs())
		Expect(err).NotTo(HaveOccurred())

		Expect(values.OpenFGA).To(BeNil())
	})
})
