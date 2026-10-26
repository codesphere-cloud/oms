// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("SCMManager", func() {
	scmManager := func(scm *files.SCMManagerConfig) codesphere.SCMManager {
		conf := files.CodesphereConfig{GitProviders: &files.GitProvidersConfig{SCMManager: scm}}

		values, err := codesphere.Values(conf, defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		return values.GitProviders.SCMManager
	}

	It("disabled SCM-Manager with a URL -> disabled without the connection", func() {
		Expect(scmManager(&files.SCMManagerConfig{Enabled: false, URL: "https://scm.example.com"})).
			To(Equal(codesphere.SCMManager{Enabled: false}))
	})

	It("SCM-Manager without an API -> no API base URL", func() {
		Expect(scmManager(&files.SCMManagerConfig{Enabled: true, URL: "https://scm.example.com"})).
			To(Equal(codesphere.SCMManager{
				Enabled:              true,
				SCMManagerConnection: &codesphere.SCMManagerConnection{URL: "https://scm.example.com"},
			}))
	})
})
