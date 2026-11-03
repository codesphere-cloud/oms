// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("validateArgoCDConfig", func() {
	var cfg files.RootConfig

	BeforeEach(func() {
		cfg = files.RootConfig{Codesphere: files.CodesphereConfig{
			CustomCertificate: &files.CustomCertificate{Cert: "cert-pem", Key: "key-pem"},
		}}
	})

	It("via ArgoCD with the certificate key in the config -> error", func() {
		err := validateArgoCDConfig(&InstallCodesphereOpts{CodesphereViaArgoCD: true}, cfg)

		Expect(err).To(MatchError(ContainSubstring("codesphere.customCertificate.key")))
	})

	It("without ArgoCD and the certificate key in the config -> no error, the legacy installer reads it", func() {
		Expect(validateArgoCDConfig(&InstallCodesphereOpts{}, cfg)).To(Succeed())
	})

	It("via ArgoCD with the codesphere step skipped -> no error, nothing is deployed", func() {
		opts := &InstallCodesphereOpts{CodesphereViaArgoCD: true, SkipSteps: []string{codesphereStep}}

		Expect(validateArgoCDConfig(opts, cfg)).To(Succeed())
	})
})
