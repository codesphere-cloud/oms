// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// GitProviders are the git providers Codesphere integrates with.
type GitProviders struct {
	AzureDevOps GitProvider `yaml:"azureDevOps"`
	GitHub      GitProvider `yaml:"github"`
	GitLab      GitProvider `yaml:"gitlab"`
	Bitbucket   GitProvider `yaml:"bitbucket"`
	SCMManager  SCMManager  `yaml:"scmManager"`
}

// GitProvider is a git provider; disabled, it has no connection.
type GitProvider struct {
	Enabled                bool `yaml:"enabled"`
	*GitProviderConnection `yaml:",inline"`
}

// GitProviderConnection is where Codesphere reaches a git provider and how it signs users in.
type GitProviderConnection struct {
	URL   string            `yaml:"url"`
	API   files.APIConfig   `yaml:"api"`
	OAuth files.OAuthConfig `yaml:"oauth"`
}

// SCMManager is the SCM-Manager integration; disabled, it has no connection.
type SCMManager struct {
	Enabled               bool `yaml:"enabled"`
	*SCMManagerConnection `yaml:",inline"`
}

// SCMManagerConnection is where Codesphere reaches SCM-Manager.
type SCMManagerConnection struct {
	URL string        `yaml:"url"`
	API SCMManagerAPI `yaml:"api"`
}

// SCMManagerAPI is SCM-Manager's API, when the config sets it apart from its URL.
type SCMManagerAPI struct {
	BaseURL string `yaml:"baseUrl,omitempty"`
}

func gitProvidersValues(providers *files.GitProvidersConfig) GitProviders {
	if providers == nil {
		providers = &files.GitProvidersConfig{}
	}

	return GitProviders{
		AzureDevOps: singleGitProviderValues(providers.AzureDevOps),
		GitHub:      singleGitProviderValues(providers.GitHub),
		GitLab:      singleGitProviderValues(providers.GitLab),
		Bitbucket:   singleGitProviderValues(providers.Bitbucket),
		SCMManager:  scmManagerValues(providers.SCMManager),
	}
}

func singleGitProviderValues(provider *files.GitProviderConfig) GitProvider {
	if provider == nil || !provider.Enabled {
		return GitProvider{Enabled: false}
	}

	return GitProvider{Enabled: true, GitProviderConnection: &GitProviderConnection{
		URL:   provider.URL,
		API:   provider.API,
		OAuth: provider.OAuth,
	}}
}

func scmManagerValues(scm *files.SCMManagerConfig) SCMManager {
	if scm == nil || !scm.Enabled {
		return SCMManager{Enabled: false}
	}

	connection := &SCMManagerConnection{URL: scm.URL}
	if scm.API != nil {
		connection.API.BaseURL = scm.API.BaseURL
	}

	return SCMManager{Enabled: true, SCMManagerConnection: connection}
}
