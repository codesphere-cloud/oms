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

	k8s "github.com/codesphere-cloud/oms/internal/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"gopkg.in/yaml.v3"
)

const (
	// ociCredentialSecretName is the K8s Secret created by "oms beta install argocd"
	// that stores OCI registry credentials for pulling Codesphere charts.
	ociCredentialSecretName = "argocd-codesphere-oci-read"
	// ociCredentialNamespace is the namespace where the credential secret lives.
	ociCredentialNamespace = "argocd"
)

// PCApps holds the configuration for installing the pc-apps Helm chart from
// a private OCI registry.
type PCApps struct {
	ChartURL    string   // full OCI chart reference, e.g. "oci://ghcr.io/codesphere-cloud/charts/pc-apps"
	Version     string   // chart version ("" means latest)
	Namespace   string   // target namespace (default: "argocd")
	Username    string   // OCI registry username (optional, falls back to K8s secret)
	Password    string   // OCI registry password/token (optional, falls back to K8s secret)
	ValuesFiles []string // paths to values YAML files, merged in order
	Helm        HelmClient
	Clientset   kubernetes.Interface
}

// NewPCApps creates a new PCApps installer with a real Helm client and
// Kubernetes clientset for credential fallback.
func NewPCApps(chartURL, version, namespace, username, password string, valuesFiles []string) (*PCApps, error) {
	if namespace == "" {
		namespace = "argocd"
	}

	helm, err := NewHelmClient(namespace)
	if err != nil {
		return nil, fmt.Errorf("creating helm client: %w", err)
	}

	clientset, _, err := k8s.NewClients()
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes clients: %w", err)
	}

	return &PCApps{
		ChartURL:    chartURL,
		Version:     version,
		Namespace:   namespace,
		Username:    username,
		Password:    password,
		ValuesFiles: valuesFiles,
		Helm:        helm,
		Clientset:   clientset,
	}, nil
}

// resolveCredentials returns the username and password to use for OCI registry
// authentication. If explicit credentials are provided on the struct, those are
// used. Otherwise it falls back to reading the K8s Secret created by
// "oms beta install argocd".
func (p *PCApps) resolveCredentials(ctx context.Context) (username, password string, err error) {
	if p.Username != "" && p.Password != "" {
		return p.Username, p.Password, nil
	}

	secret, err := p.Clientset.CoreV1().Secrets(ociCredentialNamespace).Get(ctx, ociCredentialSecretName, metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf(
			"no credentials provided and K8s secret %q not found in namespace %q: %w\n"+
				"Provide --username or run 'oms beta install argocd' first",
			ociCredentialSecretName, ociCredentialNamespace, err,
		)
	}

	username = string(secret.Data["username"])
	password = string(secret.Data["password"])
	if username == "" || password == "" {
		return "", "", fmt.Errorf(
			"K8s secret %q in namespace %q is missing username or password fields",
			ociCredentialSecretName, ociCredentialNamespace,
		)
	}

	log.Printf("Using credentials from K8s secret %q\n", ociCredentialSecretName)
	return username, password, nil
}

// Install authenticates against the OCI registry and installs or upgrades the
// pc-apps Helm chart.
func (p *PCApps) Install(ctx context.Context) error {
	host, releaseName, err := parseOCIChartURL(p.ChartURL)
	if err != nil {
		return err
	}

	username, password, err := p.resolveCredentials(ctx)
	if err != nil {
		return err
	}

	log.Printf("Authenticating against OCI registry %q...\n", host)
	if err := p.Helm.LoginRegistry(ctx, host, username, password); err != nil {
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
