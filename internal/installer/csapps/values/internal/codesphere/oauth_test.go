// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("OAuth", func() {
	oidcScopes := func(scopes []string) []string {
		conf := files.CodesphereConfig{OAuth: &files.OAuthProvidersConfig{Oidc: &files.OidcOAuthProvider{
			Type:      "oidc",
			Enabled:   true,
			Name:      "example-idp",
			IssuerURL: "https://idp.example.com",
			Scopes:    scopes,
		}}}

		values, err := codesphere.Values(conf, defaultTestImages(), defaultTestGateways(), noMetalLB, noRegistry)
		Expect(err).NotTo(HaveOccurred())

		return values.OAuth.AdditionalProviders["example-idp"].Scopes
	}

	It("OIDC provider without scopes -> openid, email and profile", func() {
		Expect(oidcScopes(nil)).To(Equal([]string{"openid", "email", "profile"}))
	})

	It("OIDC provider with an empty scope list -> kept empty", func() {
		Expect(oidcScopes([]string{})).To(Equal([]string{}))
	})
})
