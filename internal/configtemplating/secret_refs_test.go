// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package configtemplating_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/codesphere-cloud/oms/internal/configtemplating"
)

func renderSecretRefs(configYAML string) map[string]any {
	rendered, err := configtemplating.RenderSecretRefs([]byte(configYAML))
	Expect(err).NotTo(HaveOccurred())

	config := map[string]any{}
	Expect(yaml.Unmarshal(rendered, &config)).To(Succeed())

	return config
}

var _ = Describe("ValidateCodesphereSecretRefs", func() {
	It("rejects a secret in the codesphere block without a release name", func() {
		err := configtemplating.ValidateCodesphereSecretRefs(renderSecretRefs(`
codesphere:
  override:
    global:
      oauth:
        clientSecret: '{{ secret "oidcClientSecret" }}'
`))

		Expect(err).To(MatchError(
			"secret templates are not supported in the codesphere block, found at codesphere.override.global.oauth.clientSecret",
		))
	})

	It("accepts a secret in the codesphere block of a dev release", func() {
		err := configtemplating.ValidateCodesphereSecretRefs(renderSecretRefs(`
codesphere:
  releaseName: codesphere-master-dev
  override:
    clientSecret: '{{ secret "oidcClientSecret" }}'
`))

		Expect(err).NotTo(HaveOccurred())
	})

	It("accepts secrets outside the codesphere block", func() {
		err := configtemplating.ValidateCodesphereSecretRefs(renderSecretRefs(`
cluster:
  certificates:
    override:
      api-token: '{{ secret "acmeSolverSecret" }}'
codesphere:
  domain: example.com
`))

		Expect(err).NotTo(HaveOccurred())
	})

	It("accepts a config without a codesphere block", func() {
		Expect(configtemplating.ValidateCodesphereSecretRefs(renderSecretRefs(`postgres: {}`))).To(Succeed())
	})

	It("rejects a secret embedded in a longer value", func() {
		err := configtemplating.ValidateCodesphereSecretRefs(renderSecretRefs(`
codesphere:
  override:
    authHeader: 'Bearer {{ secret "apiToken" }}'
`))

		Expect(err).To(MatchError(ContainSubstring("codesphere.override.authHeader")))
	})

	It("rejects a secret inside a list with its index", func() {
		err := configtemplating.ValidateCodesphereSecretRefs(renderSecretRefs(`
codesphere:
  override:
    providers:
      - name: dex
      - clientSecret: '{{ secret "dexClientSecret" }}'
`))

		Expect(err).To(MatchError(ContainSubstring("codesphere.override.providers[1].clientSecret")))
	})

	It("lists every offending path in sorted order", func() {
		err := configtemplating.ValidateCodesphereSecretRefs(renderSecretRefs(`
codesphere:
  override:
    zeta: '{{ secret "zeta" }}'
    alpha: '{{ secret "alpha" "fields.password" }}'
`))

		Expect(err).To(MatchError(
			"secret templates are not supported in the codesphere block, found at codesphere.override.alpha, codesphere.override.zeta",
		))
	})
})
