// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"errors"
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

func oauthValues(oauth *files.OAuthProvidersConfig) (OAuth, error) {
	providers := map[string]OAuthProvider{}
	for _, id := range baseOAuthProviders {
		providers[id] = OAuthProvider{Enabled: false}
	}

	values := OAuth{Providers: providers, AdditionalProviders: map[string]OIDCProvider{}}
	if oauth == nil || oauth.Oidc == nil {
		return values, nil
	}

	oidc := oauth.Oidc
	if oidc.Name == "" {
		return OAuth{}, errors.New("oauth.oidc.name is required")
	}

	scopes := oidc.Scopes
	if scopes == nil {
		scopes = slices.Clone(defaultOIDCScopes)
	}

	values.AdditionalProviders[oidc.Name] = OIDCProvider{
		Type:      oidc.Type,
		Enabled:   oidc.Enabled,
		Name:      oidc.Name,
		IssuerURL: oidc.IssuerURL,
		Scopes:    scopes,
	}

	return values, nil
}
