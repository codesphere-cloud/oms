// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/cli/values"
	"helm.sh/helm/v4/pkg/getter"
)

const (
	// ociCredentialSecretName is the K8s Secret created by "oms beta install argocd"
	// that stores OCI registry credentials for pulling Codesphere charts.
	ociCredentialSecretName = "argocd-codesphere-oci-read"
	// ociCredentialNamespace is the namespace where the credential secret lives.
	ociCredentialNamespace = "argocd"
	// pcAppsReleaseName is the fixed Helm release name for the pc-applications chart.
	pcAppsReleaseName = "pc-applications"
	// pcAppsChartName is the chart name used when constructing the OCI chart URL.
	pcAppsChartName = "pc-applications"
)

// PCApps holds the configuration for installing the pc-applications Helm chart
// from a private OCI registry.
type PCApps struct {
	version        string   // chart version (required)
	namespace      string   // target namespace for the Helm release
	valuesFiles    []string // paths to values YAML files, merged in order
	forceConflicts bool
	helm           HelmClient
	client         client.Client
}

// NewPCApps creates a new PCApps installer. It validates that required fields
// are non-empty but does not apply defaults — defaults live on the CLI flag
// declarations only.
func NewPCApps(c client.Client, version, namespace string, valuesFiles []string, forceConflicts bool) (*PCApps, error) {
	if version == "" {
		return nil, errors.New("version is required")
	}
	if namespace == "" {
		return nil, errors.New("namespace is required")
	}

	helm, err := NewHelmClient(namespace)
	if err != nil {
		return nil, fmt.Errorf("creating helm client: %w", err)
	}

	return &PCApps{
		version:        version,
		namespace:      namespace,
		valuesFiles:    valuesFiles,
		forceConflicts: forceConflicts,
		helm:           helm,
		client:         c,
	}, nil
}

// resolveFromSecret reads the OCI registry credentials and chart base URL from
// the K8s Secret created by "oms beta install argocd --deploy-dc-config".
// It returns the full OCI chart URL, username, and password.
func (p *PCApps) resolveFromSecret(ctx context.Context) (chartURL, username, password string, _ error) {
	secret := &corev1.Secret{}
	key := client.ObjectKey{Name: ociCredentialSecretName, Namespace: ociCredentialNamespace}
	if err := p.client.Get(ctx, key, secret); err != nil {
		return "", "", "", fmt.Errorf(
			"K8s secret %q not found in namespace %q: %w\n"+
				"Run 'oms beta install argocd --deploy-dc-config' first to create registry credentials",
			ociCredentialSecretName, ociCredentialNamespace, err,
		)
	}

	baseURL := string(secret.Data["url"])
	username = string(secret.Data["username"])
	password = string(secret.Data["password"])

	if baseURL == "" || username == "" || password == "" {
		return "", "", "", fmt.Errorf(
			"K8s secret %q in namespace %q is missing required fields (url, username, or password)",
			ociCredentialSecretName, ociCredentialNamespace,
		)
	}

	joined, err := url.JoinPath("oci://"+baseURL, pcAppsChartName)
	if err != nil {
		return "", "", "", fmt.Errorf("constructing chart URL: %w", err)
	}
	chartURL = joined
	log.Printf("Using credentials from K8s secret %q (registry: %s)\n", ociCredentialSecretName, baseURL)
	return chartURL, username, password, nil
}

// Install authenticates against the OCI registry and installs or upgrades the
// pc-applications Helm chart.
func (p *PCApps) Install(ctx context.Context) error {
	// Validate values files before any network calls so local errors fail fast.
	valueOpts := values.Options{ValueFiles: p.valuesFiles}
	vals, err := valueOpts.MergeValues(getter.All(cli.New()))
	if err != nil {
		return fmt.Errorf("loading values files: %w", err)
	}

	chartURL, username, password, err := p.resolveFromSecret(ctx)
	if err != nil {
		return err
	}

	parsed, err := url.Parse(chartURL)
	if err != nil {
		return fmt.Errorf("parsing chart URL %q: %w", chartURL, err)
	}
	if parsed.Host == "" {
		return fmt.Errorf("chart URL %q has no host", chartURL)
	}

	log.Printf("Authenticating against OCI registry %q...\n", parsed.Host)
	if err := p.helm.LoginRegistry(ctx, parsed.Host, username, password); err != nil {
		return fmt.Errorf("registry login failed: %w", err)
	}

	cfg := ChartConfig{
		ReleaseName:     pcAppsReleaseName,
		ChartName:       chartURL,
		Namespace:       p.namespace,
		Version:         p.version,
		Values:          vals,
		CreateNamespace: true,
	}

	log.Printf("Installing/Upgrading %s (version %s) into namespace %s\n", pcAppsReleaseName, p.version, p.namespace)
	if err := p.helm.UpgradeChart(ctx, cfg, UpgradeChartOptions{
		InstallIfNotExist: true,
		ForceConflicts:    p.forceConflicts,
	}); err != nil {
		return fmt.Errorf("install/upgrade failed: %w", err)
	}

	log.Printf("Successfully installed/upgraded %s\n", pcAppsReleaseName)
	return nil
}

// NewPCAppsForTesting creates a PCApps instance with injected dependencies for
// use in tests. This avoids exporting struct fields solely for test access.
func NewPCAppsForTesting(helm HelmClient, c client.Client, version, namespace string, valuesFiles []string, forceConflicts bool) *PCApps {
	return &PCApps{
		version:        version,
		namespace:      namespace,
		valuesFiles:    valuesFiles,
		forceConflicts: forceConflicts,
		helm:           helm,
		client:         c,
	}
}
