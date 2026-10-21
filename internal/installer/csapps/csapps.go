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
)

// Install applies the cs-applications Application of the release cfg configures and waits until
// ArgoCD has deployed Codesphere with the values built from cfg.
func Install(ctx context.Context, kubeClient client.Client, cfg *files.RootConfig, b *bom.Config, logf argocd.LogFunc) error {
	chartsRegistry, err := installer.ResolveChartsRegistry(ctx, kubeClient)
	if err != nil {
		return fmt.Errorf("failed to resolve the charts registry: %w", err)
	}

	appValues, err := Values(cfg, b, chartsRegistry, rand.Text())
	if err != nil {
		return err
	}

	release := releaseName(cfg.Codesphere)
	appName := release + csAppsAppNameSuffix

	apps, err := installer.NewAppOfAppsFromBom(kubeClient, b, csAppsComponent, appName, argocd.DefaultNamespace, appValues)
	if err != nil {
		return fmt.Errorf("failed to initialize the cs-applications installer: %w", err)
	}

	apps.DeleteResourcesWithApplication()

	if err := apps.Install(ctx); err != nil {
		return fmt.Errorf("failed to apply the cs-applications Application: %w", err)
	}

	// The cs-applications Application must sync first: that sync writes the new values into the
	// Codesphere Application.
	for _, name := range []string{appName, release + codesphereAppSuffix} {
		if err := argocd.WaitForApplicationSynced(ctx, kubeClient, name, logf); err != nil {
			return fmt.Errorf("failed to wait for ArgoCD to deploy Codesphere: %w", err)
		}
	}

	return nil
}

// Values returns the cs-applications values that deploy Codesphere from chartsRegistry. Changing
// syncTrigger makes External Secrets read the vault again.
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

	return map[string]any{
		"chartsRegistry": chartsRegistry,
		"releaseName":    releaseName(cfg.Codesphere),
		"namespace":      codesphereNamespace,
		"applications": map[string]any{
			codesphereComponent: map[string]any{
				"version":      chart.Tag(),
				"valuesObject": codesphereValues,
			},
		},
	}, nil
}

// releaseName is the Helm release name of Codesphere, as the Codesphere installer names it.
func releaseName(conf files.CodesphereConfig) string {
	if conf.ReleaseName != "" {
		return conf.ReleaseName
	}

	return defaultReleaseName
}
