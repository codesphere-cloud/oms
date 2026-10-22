// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

func gitProvidersValues(providers *files.GitProvidersConfig) map[string]any {
	if providers == nil {
		providers = &files.GitProvidersConfig{}
	}

	values := map[string]any{
		"azureDevOps": gitProviderValues(providers.AzureDevOps),
		"github":      gitProviderValues(providers.GitHub),
		"gitlab":      gitProviderValues(providers.GitLab),
		"bitbucket":   gitProviderValues(providers.Bitbucket),
	}

	scm := providers.SCMManager
	if scm == nil || !scm.Enabled {
		values["scmManager"] = map[string]any{"enabled": false}
		return values
	}

	scmValues := map[string]any{"enabled": true, "url": scm.URL, "api": map[string]any{}}
	if scm.API != nil {
		scmValues["api"] = map[string]any{"baseUrl": scm.API.BaseURL}
	}

	values["scmManager"] = scmValues

	return values
}

func gitProviderValues(provider *files.GitProviderConfig) map[string]any {
	if provider == nil || !provider.Enabled {
		return map[string]any{"enabled": false}
	}

	oauth := map[string]any{
		"issuer":                provider.OAuth.Issuer,
		"authorizationEndpoint": provider.OAuth.AuthorizationEndpoint,
		"tokenEndpoint":         provider.OAuth.TokenEndpoint,
	}

	optional := map[string]string{
		"redirectUri":      provider.OAuth.RedirectURI,
		"installationUri":  provider.OAuth.InstallationURI,
		"clientAuthMethod": provider.OAuth.ClientAuthMethod,
	}
	for key, value := range optional {
		if value != "" {
			oauth[key] = value
		}
	}

	if provider.OAuth.Scope != "" {
		oauth["scope"] = provider.OAuth.Scope
	}

	return map[string]any{
		"enabled": true,
		"url":     provider.URL,
		"api":     map[string]any{"baseUrl": provider.API.BaseURL},
		"oauth":   oauth,
	}
}
