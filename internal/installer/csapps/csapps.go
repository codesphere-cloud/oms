// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package csapps deploys a Codesphere release through the cs-applications app of apps.
package csapps

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	csAppsComponent     = "cs-applications"
	codesphereComponent = "codesphere"
	codesphereNamespace = "codesphere"
	defaultReleaseName  = "codesphere"
	csAppsAppNameSuffix = "-" + csAppsComponent
)

// ValidateConfig rejects what the legacy installer accepts but must not reach the ArgoCD
// Application, where the values sit in plain text. It is exported so the CLI can reject the config
// before the legacy installer turns on maintenance mode and migrates the database.
func ValidateConfig(cfg *files.RootConfig) error {
	if err := chartutil.ValidateReleaseName(releaseName(cfg)); err != nil {
		return fmt.Errorf("codesphere.releaseName %q: %w", cfg.Codesphere.ReleaseName, err)
	}

	if cert := cfg.Codesphere.CustomCertificate; cert != nil && cert.Key != "" {
		return fmt.Errorf(
			"codesphere.customCertificate.key must not be in the config when deploying through ArgoCD, move it into the vault entry %q",
			files.SecretCustomCertificateKey,
		)
	}

	return nil
}

// Install applies the cs-applications Application of the release cfg configures, and waits until
// ArgoCD has deployed all sub applications with the values built from cfg and they are healthy.
// The release reads its secrets from the vault Secret vaultSecretName.
func Install(ctx context.Context, kubeClient client.Client, cfg *files.RootConfig, csBom *bom.Config, vaultSecretName string) error {
	if err := ValidateConfig(cfg); err != nil {
		return err
	}

	appValues, err := Values(cfg, csBom, vaultSecretName)
	if err != nil {
		return err
	}

	release := releaseName(cfg)
	parentAppName := release + csAppsAppNameSuffix

	parentApp, err := installer.NewAppOfAppsFromBom(kubeClient, csBom, installer.AppOfAppsConfig{
		Component:      csAppsComponent,
		Name:           parentAppName,
		ReleaseName:    release,
		Namespace:      argocd.DefaultNamespace,
		ValuesOverride: appValues,
		// Deleting the Application tears down the whole release, e.g. a preview. The chart's sub
		// applications carry the same finalizer.
		DeleteResourcesWithApp: true,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize the %s installer: %w", csAppsComponent, err)
	}

	if err := parentApp.Install(ctx); err != nil {
		return fmt.Errorf("failed to apply ArgoCD Application %q: %w", parentAppName, err)
	}

	// The parent must sync first: that sync writes the new values into the sub applications.
	if err := argocd.WaitForAppFreshlySyncedAndHealthy(ctx, kubeClient, parentAppName); err != nil {
		return fmt.Errorf("failed to wait for ArgoCD Application %q: %w", parentAppName, err)
	}

	return waitForSubApps(ctx, kubeClient, parentAppName)
}

// waitForSubApps waits in parallel for every Application the parent deployed, so a failure
// reports all sub applications that did not come up.
func waitForSubApps(ctx context.Context, kubeClient client.Client, parentAppName string) error {
	names, err := subAppNames(ctx, kubeClient, parentAppName)
	if err != nil {
		return err
	}

	waitErrs := make([]error, len(names))

	var wg sync.WaitGroup
	for i, name := range names {
		wg.Go(func() {
			if err := argocd.WaitForAppFreshlySyncedAndHealthy(ctx, kubeClient, name); err != nil {
				waitErrs[i] = fmt.Errorf("failed to wait for ArgoCD Application %q: %w", name, err)
			}
		})
	}

	wg.Wait()

	return errors.Join(waitErrs...)
}

func subAppNames(ctx context.Context, kubeClient client.Client, parentAppName string) ([]string, error) {
	parent := &argov1alpha1.Application{}
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: parentAppName, Namespace: argocd.DefaultNamespace}, parent); err != nil {
		return nil, fmt.Errorf("failed to read ArgoCD Application %q: %w", parentAppName, err)
	}

	var names []string

	for _, resource := range parent.Status.Resources {
		if resource.Group == argov1alpha1.SchemeGroupVersion.Group && resource.Kind == argov1alpha1.ApplicationSchemaGroupVersionKind.Kind {
			names = append(names, resource.Name)
		}
	}

	return names, nil
}

// Values returns the cs-applications values: the Codesphere chart version and values OMS builds,
// with the config's csApps block merged over them. Sub applications other than codesphere get
// their values only from csApps; extend the config for a new one that needs built values.
func Values(cfg *files.RootConfig, csBom *bom.Config, vaultSecretName string) (map[string]any, error) {
	chart, ok := csBom.GetChart(codesphereComponent)
	if !ok {
		return nil, fmt.Errorf("%s chart not found in BOM", codesphereComponent)
	}

	codesphereValues, err := values.Build(cfg, csBom)
	if err != nil {
		return nil, fmt.Errorf("failed to build the Codesphere values: %w", err)
	}

	built := map[string]any{
		"namespace": codesphereNamespace,
		"applications": map[string]any{
			codesphereComponent: map[string]any{
				"enabled":      true,
				"version":      chart.Tag(),
				"valuesObject": codesphereValues,
			},
		},
	}

	merged := values.MergeOverride(built, cfg.CsApps)

	// Set after the csApps merge, which must not undo them.
	return values.MergeOverride(merged, map[string]any{
		"applications": map[string]any{
			codesphereComponent: map[string]any{
				"valuesObject": map[string]any{
					"global": map[string]any{
						// TODO(nate): Remove (default to true) once the legacy/non-argo install path is gone.
						"externalSecrets": true,
						// TODO(nate): Remove (default to true) once the legacy/non-argo install path is gone.
						"argoCd":                     true,
						"externalSecretsVaultSecret": vaultSecretName,
						// ESO only re-reads the vault when an ExternalSecret changes, so a new trigger on
						// every deploy makes a changed vault reach the release in that deploy.
						"externalSecretsSyncTrigger": rand.Text(),
					},
				},
			},
		},
	}), nil
}

// releaseName names the Codesphere Helm release and the release's Applications. Dev clusters set
// codesphere.releaseName, as several Codesphere releases share them.
func releaseName(cfg *files.RootConfig) string {
	if cfg.Codesphere.ReleaseName != "" {
		return cfg.Codesphere.ReleaseName
	}

	return defaultReleaseName
}
