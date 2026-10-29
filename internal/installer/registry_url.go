// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/distribution/reference"
)

var registryHostPattern = regexp.MustCompile("^" + reference.DomainRegexp.String() + "$")

// DefaultChartsRegistry writes the mirror default into a generated install
// config, preserving an explicitly configured child chart repository.
func DefaultChartsRegistry(config *files.RootConfig) error {
	if registry, _ := config.PcApps["chartsRegistry"].(string); registry != "" {
		return nil
	}

	if config.Registry == nil || config.Registry.Server == "" {
		return nil
	}

	registry, err := ChartRegistryURL(config.Registry.Server)
	if err != nil {
		return err
	}

	if config.PcApps == nil {
		config.PcApps = files.ChartValues{}
	}

	config.PcApps["chartsRegistry"] = registry

	return nil
}

// ChartRegistryURL joins the chart repository path to a registry host or repository prefix.
// For example, "registry.example.com" becomes "registry.example.com/codesphere-cloud/charts",
// and "registry.example.com:5000/team/" becomes
// "registry.example.com:5000/team/codesphere-cloud/charts". An empty server returns an empty
// URL so callers can keep the chart repository from the BOM.
func ChartRegistryURL(server string) (string, error) {
	if server == "" {
		return "", nil
	}

	if strings.Contains(server, "://") || strings.HasPrefix(server, "//") {
		return "", fmt.Errorf("registry server %q must not include a scheme", server)
	}

	registry, err := url.Parse("//" + server)
	if err != nil {
		return "", fmt.Errorf("parse chart registry URL: %w", err)
	}

	if !registryHostPattern.MatchString(registry.Host) || registry.User != nil || registry.RawQuery != "" || registry.Fragment != "" {
		return "", fmt.Errorf("invalid registry server %q: expected a registry host with an optional port and repository prefix", server)
	}

	return strings.TrimPrefix(registry.JoinPath(ChartsRepositoryPath).String(), "//"), nil
}

// RegistryHost extracts the host and optional port from a registry URL or reference.
func RegistryHost(registryURL string) string {
	if _, rest, found := strings.Cut(registryURL, "://"); found {
		registryURL = rest
	}

	host, _, _ := strings.Cut(registryURL, "/")

	return host
}
