// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package csapps deploys a Codesphere release through the cs-applications app of apps.
package csapps

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	csAppsComponent      = "cs-applications"
	codesphereComponent  = "codesphere"
	codesphereNamespace  = "codesphere"
	defaultReleaseName   = "codesphere"
	codesphereAppSuffix  = "-codesphere"
	csAppsAppNameSuffix  = "-" + csAppsComponent
	externalSecretsKey   = "externalSecrets"
	syncTriggerValuesKey = "externalSecretsSyncTrigger"
	argoCDValuesKey      = "argoCd"
)

// ValidateConfig rejects what the legacy installer accepts but must not reach the ArgoCD
// Application, where the values sit in plain text.
func ValidateConfig(cfg *files.RootConfig) error {
	if err := validateReleaseName(cfg); err != nil {
		return err
	}

	if cert := cfg.Codesphere.CustomCertificate; cert != nil && cert.Key != "" {
		return fmt.Errorf(
			"codesphere.customCertificate.key must not be in the config when deploying through ArgoCD, move it into the vault entry %q",
			files.SecretCustomCertificateKey,
		)
	}

	return nil
}

func validateReleaseName(cfg *files.RootConfig) error {
	if err := chartutil.ValidateReleaseName(releaseName(cfg)); err != nil {
		return fmt.Errorf("codesphere.releaseName %q: %w", cfg.Codesphere.ReleaseName, err)
	}

	return nil
}

// Install applies the cs-applications Application of the release cfg configures and waits until
// ArgoCD has deployed Codesphere with the values built from cfg.
func Install(ctx context.Context, kubeClient client.Client, cfg *files.RootConfig, b *bom.Config, logf argocd.LogFunc) error {
	if err := ValidateConfig(cfg); err != nil {
		return err
	}

	chartsRegistry, err := installer.ResolveChartsRegistry(ctx, kubeClient)
	if err != nil {
		return fmt.Errorf("failed to resolve the charts registry: %w", err)
	}

	appValues, err := Values(cfg, b, chartsRegistry, rand.Text())
	if err != nil {
		return err
	}

	release := releaseName(cfg)
	appName := release + csAppsAppNameSuffix

	apps, err := installer.NewAppOfAppsFromBom(kubeClient, b, installer.AppOfAppsConfig{
		Component:      csAppsComponent,
		Name:           appName,
		ReleaseName:    release,
		Namespace:      argocd.DefaultNamespace,
		ValuesOverride: appValues,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize the cs-applications installer: %w", err)
	}

	// Deleting the Application then tears down a whole preview release. The default release stays
	// safe from an accidental delete.
	if release != defaultReleaseName {
		apps.DeleteResourcesWithApplication()
	}

	if err := apps.Install(ctx); err != nil {
		return fmt.Errorf("failed to apply the cs-applications Application: %w", err)
	}

	// The cs-applications Application must sync first: that sync writes the new values into the
	// Codesphere Application.
	for _, name := range []string{appName, release + codesphereAppSuffix} {
		if err := argocd.WaitForAppFreshlySyncedAndHealthy(ctx, kubeClient, name, logf); err != nil {
			return fmt.Errorf("failed to wait for ArgoCD to deploy Codesphere: %w", err)
		}
	}

	return nil
}

// Values returns the cs-applications values that deploy Codesphere from chartsRegistry, with the
// config's csApps block merged over them. Changing syncTrigger makes External Secrets read the
// vault again.
func Values(cfg *files.RootConfig, b *bom.Config, chartsRegistry, syncTrigger string) (map[string]any, error) {
	chart, ok := b.GetChart(codesphereComponent)
	if !ok {
		return nil, fmt.Errorf("%s chart not found in BOM", codesphereComponent)
	}

	codesphereValues, err := values.Build(cfg, b)
	if err != nil {
		return nil, fmt.Errorf("failed to build the Codesphere values: %w", err)
	}

	global, ok := codesphereValues["global"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("the Codesphere values have no global values")
	}

	// The values hold no secrets: the release Secret must come from the vault.
	global[externalSecretsKey] = true
	global[syncTriggerValuesKey] = syncTrigger
	global[argoCDValuesKey] = true

	built := map[string]any{
		"chartsRegistry": chartsRegistry,
		"namespace":      codesphereNamespace,
		"applications": map[string]any{
			codesphereComponent: map[string]any{
				"enabled":      true,
				"version":      chart.Tag(),
				"valuesObject": codesphereValues,
			},
		},
	}

	return util.DeepMergeMaps(built, cfg.CsApps), nil
}

// releaseName names the Codesphere Helm release and the release's Applications. Dev clusters set
// codesphere.releaseName, as several Codesphere releases share them.
func releaseName(cfg *files.RootConfig) string {
	if cfg.Codesphere.ReleaseName != "" {
		return cfg.Codesphere.ReleaseName
	}

	return defaultReleaseName
}
