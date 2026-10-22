// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// baseOAuthProviders are the built-in OAuth providers, all disabled in private clouds.
var baseOAuthProviders = []string{"bitbucket", "github", "gitlab", "google"}

func oauthValues(oauth *files.OAuthProvidersConfig) map[string]any {
	providers := map[string]any{}
	for _, id := range baseOAuthProviders {
		providers[id] = map[string]any{"enabled": false}
	}

	additional := map[string]any{}

	if oauth != nil && oauth.Oidc != nil {
		oidc := oauth.Oidc

		scopes := oidc.Scopes
		if scopes == nil {
			scopes = []string{"openid", "email", "profile"}
		}

		additional[oidc.Name] = map[string]any{
			"type":      oidc.Type,
			"enabled":   oidc.Enabled,
			"name":      oidc.Name,
			"issuerUrl": oidc.IssuerURL,
			"scopes":    scopes,
		}
	}

	return map[string]any{"providers": providers, "additionalProviders": additional}
}
