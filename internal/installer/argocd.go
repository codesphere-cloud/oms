// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"log"
	"os"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
)

type ArgoCDManager interface {
	Install() error
}

type ArgoCD struct {
	Version string
}

func NewArgoCD() ArgoCDManager {
	return &ArgoCD{
		Version: "9.1.4",
	}
}

func (a *ArgoCD) Install() error {
	settings := cli.New()
	ctx := context.Background()

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), os.Getenv("HELM_DRIVER")); err != nil {
		log.Fatalf("Init failed: %v", err)
	}

	// 2. Setup the Install Client
	client := action.NewInstall(actionConfig)
	client.ReleaseName = "argocd"
	client.Namespace = "argocd"
	client.CreateNamespace = true
	client.DryRunStrategy = "none"
	client.WaitStrategy = "watcher"
	client.Version = "9.1.4"
	// The repo URL must be provided so LocateChart knows where to look
	client.ChartPathOptions.RepoURL = "https://argoproj.github.io/argo-helm"

	// 3. Locate and Load Chart
	// This replaces the manual downloader/manager logic for standard charts
	chartPath, err := client.ChartPathOptions.LocateChart("argo-cd", settings)
	if err != nil {
		log.Fatalf("LocateChart failed: %v", err)
	}

	chartRequested, err := loader.Load(chartPath)
	if err != nil {
		log.Fatalf("Load failed: %v", err)
	}

	// 4. Define Values (Example: enabling the UI)
	vals := map[string]interface{}{
		"dex": map[string]interface{}{
			"enabled": false,
		},
		"configs": map[string]interface{}{
			"secret": map[string]interface{}{
				"createSecret": true,
			},
		},
	}

	// 5. Run Installation
	_, err = client.RunWithContext(ctx, chartRequested, vals)
	if err != nil {
		log.Fatalf("Install failed: %v", err)
	}

	fmt.Printf("Successfully installed Argo CD %s (Chart version: %s)\n", client.ReleaseName, client.Version)
	return nil
}
