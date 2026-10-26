// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// baseOAuthProviders are the built-in OAuth providers, all disabled in private clouds.
var baseOAuthProviders = []string{"bitbucket", "github", "gitlab", "google"}

// OAuth are the providers users sign in with.
type OAuth struct {
	Providers           map[string]OAuthProvider `yaml:"providers"`
	AdditionalProviders map[string]OIDCProvider  `yaml:"additionalProviders"`
}

// OAuthProvider is one of the built-in providers.
type OAuthProvider struct {
	Enabled bool `yaml:"enabled"`
}

// OIDCProvider is a provider the config adds.
type OIDCProvider struct {
	Type      string   `yaml:"type"`
	Enabled   bool     `yaml:"enabled"`
	Name      string   `yaml:"name"`
	IssuerURL string   `yaml:"issuerUrl"`
	Scopes    []string `yaml:"scopes"`
}

func oauthValues(oauth *files.OAuthProvidersConfig) OAuth {
	providers := map[string]OAuthProvider{}
	for _, id := range baseOAuthProviders {
		providers[id] = OAuthProvider{Enabled: false}
	}

	additional := map[string]OIDCProvider{}

	if oauth != nil && oauth.Oidc != nil {
		oidc := oauth.Oidc

		scopes := oidc.Scopes
		if scopes == nil {
			scopes = []string{"openid", "email", "profile"}
		}

		additional[oidc.Name] = OIDCProvider{
			Type:      oidc.Type,
			Enabled:   oidc.Enabled,
			Name:      oidc.Name,
			IssuerURL: oidc.IssuerURL,
			Scopes:    scopes,
		}
	}

	return OAuth{Providers: providers, AdditionalProviders: additional}
}
