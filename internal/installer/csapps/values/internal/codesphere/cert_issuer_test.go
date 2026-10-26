// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Cert issuer", func() {
	acmeConf := func(acme files.ACMEConfig) files.CodesphereConfig {
		acme.Server = "https://acme.example.com/directory"
		acme.Email = "ops@example.com"

		return files.CodesphereConfig{
			CertIssuer: &files.CertIssuerConfig{Type: files.CertIssuerTypeACME, Acme: &acme},
		}
	}

	It("ACME with only the cluster EAB key -> cluster key and its MAC key", func() {
		values, err := codesphere.Values(acmeConf(files.ACMEConfig{EABKeyID: "cluster-key"}),
			defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		Expect(values.CertIssuer).To(Equal(codesphere.CertIssuer{ACME: codesphere.ACME{Enabled: true, ACMEAccount: &codesphere.ACMEAccount{
			Server:        "https://acme.example.com/directory",
			Email:         "ops@example.com",
			EABKeyID:      "cluster-key",
			EABMacKeyName: "acmeEabMacKey",
		}}}))
	})

	It("ACME without EAB keys -> account without EAB", func() {
		values, err := codesphere.Values(acmeConf(files.ACMEConfig{}),
			defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		Expect(values.CertIssuer).To(Equal(codesphere.CertIssuer{ACME: codesphere.ACME{Enabled: true, ACMEAccount: &codesphere.ACMEAccount{
			Server: "https://acme.example.com/directory",
			Email:  "ops@example.com",
		}}}))
	})

	DescribeTable("no ACME issuer -> ACME disabled without an account",
		func(issuer *files.CertIssuerConfig) {
			values, err := codesphere.Values(files.CodesphereConfig{CertIssuer: issuer},
				defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
			Expect(err).NotTo(HaveOccurred())

			Expect(values.CertIssuer).To(Equal(codesphere.CertIssuer{ACME: codesphere.ACME{Enabled: false}}))
		},
		Entry("no cert issuer", nil),
		Entry("self-signed issuer", &files.CertIssuerConfig{Type: files.CertIssuerTypeSelfSigned}),
		Entry("ACME type without ACME config", &files.CertIssuerConfig{Type: files.CertIssuerTypeACME}),
	)
})
