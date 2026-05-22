// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// PCApps holds the configuration for installing the pc-apps Helm chart from
// a private OCI registry.
type PCApps struct {
	ChartURL    string   // full OCI chart reference, e.g. "oci://ghcr.io/codesphere-cloud/charts/pc-apps"
	Version     string   // chart version ("" means latest)
	Namespace   string   // target namespace (default: "argocd")
	Username    string   // OCI registry username
	Password    string   // OCI registry password/token
	ValuesFiles []string // paths to values YAML files, merged in order
	Helm        HelmClient
}

// NewPCApps creates a new PCApps installer with a real Helm client.
func NewPCApps(chartURL, version, namespace, username, password string, valuesFiles []string) (*PCApps, error) {
	if namespace == "" {
		namespace = "argocd"
	}

	helm, err := NewHelmClient(namespace)
	if err != nil {
		return nil, fmt.Errorf("creating helm client: %w", err)
	}

	return &PCApps{
		ChartURL:    chartURL,
		Version:     version,
		Namespace:   namespace,
		Username:    username,
		Password:    password,
		ValuesFiles: valuesFiles,
		Helm:        helm,
	}, nil
}

// Install authenticates against the OCI registry and installs or upgrades the
// pc-apps Helm chart.
func (p *PCApps) Install(ctx context.Context) error {
	host, releaseName, err := parseOCIChartURL(p.ChartURL)
	if err != nil {
		return err
	}

	log.Printf("Authenticating against OCI registry %q...\n", host)
	if err := p.Helm.LoginRegistry(ctx, host, p.Username, p.Password); err != nil {
		return fmt.Errorf("registry login failed: %w", err)
	}

	values, err := LoadAndMergeValues(p.ValuesFiles)
	if err != nil {
		return fmt.Errorf("loading values files: %w", err)
	}

	cfg := ChartConfig{
		ReleaseName:     releaseName,
		ChartName:       p.ChartURL,
		Namespace:       p.Namespace,
		Version:         p.Version,
		Values:          values,
		CreateNamespace: true,
	}

	if p.Version != "" {
		log.Printf("Installing/Upgrading %s (version %s) into namespace %s\n", releaseName, p.Version, p.Namespace)
	} else {
		log.Printf("Installing/Upgrading %s (latest) into namespace %s\n", releaseName, p.Namespace)
	}

	existing, err := p.Helm.FindRelease(p.Namespace, releaseName)
	if err != nil {
		return fmt.Errorf("checking existing release: %w", err)
	}

	if existing != nil {
		log.Printf("Found existing release %s (chart version %s), upgrading...\n", releaseName, existing.InstalledVersion)
		if err := p.Helm.UpgradeChart(ctx, cfg, UpgradeChartOptions{}); err != nil {
			return fmt.Errorf("upgrade failed: %w", err)
		}
		fmt.Printf("Successfully upgraded %s\n", releaseName)
	} else {
		log.Printf("No existing release found, performing fresh install...\n")
		if err := p.Helm.InstallChart(ctx, cfg); err != nil {
			return fmt.Errorf("install failed: %w", err)
		}
		fmt.Printf("Successfully installed %s\n", releaseName)
	}

	return nil
}

// parseOCIChartURL extracts the registry host and chart name from an OCI URL.
// Example: "oci://ghcr.io/codesphere-cloud/charts/pc-apps" -> ("ghcr.io", "pc-apps", nil)
func parseOCIChartURL(chartURL string) (host string, chartName string, err error) {
	if !strings.HasPrefix(chartURL, "oci://") {
		return "", "", fmt.Errorf("chart URL must start with \"oci://\", got %q", chartURL)
	}

	// Replace oci:// with https:// for standard URL parsing
	parsed, err := url.Parse(strings.Replace(chartURL, "oci://", "https://", 1))
	if err != nil {
		return "", "", fmt.Errorf("parsing chart URL %q: %w", chartURL, err)
	}

	host = parsed.Host
	if host == "" {
		return "", "", fmt.Errorf("chart URL %q has no host", chartURL)
	}

	chartName = path.Base(parsed.Path)
	if chartName == "" || chartName == "." || chartName == "/" {
		return "", "", fmt.Errorf("chart URL %q has no chart name in path", chartURL)
	}

	return host, chartName, nil
}

// LoadAndMergeValues reads multiple YAML values files and deep-merges them in
// order (later files override earlier ones).
func LoadAndMergeValues(files []string) (map[string]interface{}, error) {
	merged := map[string]interface{}{}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("reading values file %q: %w", f, err)
		}

		var vals map[string]interface{}
		if err := yaml.Unmarshal(data, &vals); err != nil {
			return nil, fmt.Errorf("parsing values file %q: %w", f, err)
		}

		merged = deepMerge(merged, vals)
	}

	return merged, nil
}

// deepMerge recursively merges src into dst. Values in src take precedence.
func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	for key, srcVal := range src {
		dstVal, exists := dst[key]
		if !exists {
			dst[key] = srcVal
			continue
		}

		// If both are maps, recurse
		srcMap, srcOk := srcVal.(map[string]interface{})
		dstMap, dstOk := dstVal.(map[string]interface{})
		if srcOk && dstOk {
			dst[key] = deepMerge(dstMap, srcMap)
		} else {
			dst[key] = srcVal
		}
	}
	return dst
}
