// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// baseOAuthProviders are the built-in OAuth providers, all disabled in private clouds.
var baseOAuthProviders = []string{"bitbucket", "github", "gitlab", "google"}

var defaultOIDCScopes = []string{"openid", "email", "profile"}

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

	values := OAuth{Providers: providers, AdditionalProviders: map[string]OIDCProvider{}}
	if oauth == nil || oauth.Oidc == nil {
		return values
	}

	oidc := oauth.Oidc

	name := oidc.Name
	if name == "" {
		name = files.DefaultOidcProviderName
	}

	scopes := oidc.Scopes
	if scopes == nil {
		scopes = slices.Clone(defaultOIDCScopes)
	}

	values.AdditionalProviders[name] = OIDCProvider{
		Type:      oidc.Type,
		Enabled:   oidc.Enabled,
		Name:      name,
		IssuerURL: oidc.IssuerURL,
		Scopes:    scopes,
	}

	return values
}
