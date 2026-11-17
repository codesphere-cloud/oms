// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package csapps deploys a Codesphere release through the cs-applications app of apps.
package csapps

import (
	"context"
	"fmt"
	"time"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/csapps/values"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	csAppsComponent     = "cs-applications"
	csUmbrellaComponent = "codesphere"
	codesphereNamespace = "codesphere"
	defaultReleaseName  = "codesphere"
)

// Install applies the cs-applications Application of the release cfg configures, and waits until
// ArgoCD has deployed all sub applications with the values built from cfg and they are healthy.
// The release reads its secrets from the vault Secret vaultSecretName.
func Install(
	ctx context.Context,
	kubeClient client.Client,
	cfg *files.RootConfig,
	csBom *bom.Config,
	vaultSecretName string,
) error {
	appValues, err := parentAppValues(cfg, csBom, vaultSecretName)
	if err != nil {
		return err
	}

	release := releaseName(cfg)
	parentAppName := parentAppNameFor(cfg)

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

	// The parent must sync first: its sync creates the sub applications and writes their new values.
	if err := argocd.WaitForAppFreshlySyncedAndHealthy(ctx, kubeClient, parentAppName); err != nil {
		return fmt.Errorf("failed to wait for ArgoCD Application %q: %w", parentAppName, err)
	}

	if err := argocd.WaitForSubAppsFreshlySyncedAndHealthy(ctx, kubeClient, parentAppName); err != nil {
		return fmt.Errorf("failed to wait for the sub applications of %q: %w", parentAppName, err)
	}

	return nil
}

// parentAppValues returns the cs-applications values: the Codesphere umbrella chart version and the values
// OMS builds from the config, with the config's csApps block merged over them. The built global
// values go to the top level, which the chart maps to each sub application's own values. Other
// values reach a sub application only through csApps; for permanent config for a new app, extend
// the config appropriately.
func parentAppValues(cfg *files.RootConfig, csBom *bom.Config, vaultSecretName string) (map[string]any, error) {
	chart, ok := csBom.GetChart(csUmbrellaComponent)
	if !ok {
		return nil, fmt.Errorf("%s chart not found in BOM", csUmbrellaComponent)
	}

	codesphereValues, err := values.Build(cfg, csBom)
	if err != nil {
		return nil, fmt.Errorf("failed to build the Codesphere values: %w", err)
	}

	global := codesphereValues["global"]
	delete(codesphereValues, "global")

	built := map[string]any{
		"namespace": codesphereNamespace,
		"global":    global,
		"applications": map[string]any{
			csUmbrellaComponent: map[string]any{
				"enabled":      true,
				"version":      chart.Tag(),
				"valuesObject": codesphereValues,
			},
		},
	}

	merged := values.MergeOverride(built, cfg.CsApps)

	// Set after the csApps merge, which must not undo them.
	return values.MergeOverride(merged, map[string]any{
		"global": map[string]any{
			// TODO(nate): Remove (default to true) once the legacy/non-argo install path is gone.
			"externalSecrets": true,
			// TODO(nate): Remove (default to true) once the legacy/non-argo install path is gone.
			"argoCd":                     true,
			"externalSecretsVaultSecret": vaultSecretName,
			// ESO only re-reads the vault when an ExternalSecret changes, so a new trigger on every
			// deploy makes a changed vault reach the release in that deploy. A timestamp also shows
			// when the last deploy ran.
			"externalSecretsSyncTrigger": time.Now().UTC().Format(time.RFC3339Nano),
		},
	}), nil
}

func parentAppNameFor(cfg *files.RootConfig) string {
	return releaseName(cfg) + "-" + csAppsComponent
}

// releaseName names the Codesphere Helm release and the release's Applications. Dev clusters set
// codesphere.releaseName, as several Codesphere releases share them.
func releaseName(cfg *files.RootConfig) string {
	if cfg.Codesphere.ReleaseName != "" {
		return cfg.Codesphere.ReleaseName
	}

	return defaultReleaseName
}
