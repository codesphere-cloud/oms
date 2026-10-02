// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"fmt"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer"
)

type prerequisiteChartConfig struct {
	Name, Chart, RepoURL, TargetRevision, Namespace string
	Values                                          map[string]interface{}
}

// installPrerequisiteChart installs operators needed before Argo CD is available.
func (b *LocalBootstrapper) installPrerequisiteChart(cfg prerequisiteChartConfig) error {
	helm := b.helmClient
	if helm == nil {
		return fmt.Errorf("helm client is not set")
	}

	chartName := cfg.Chart
	repoURL := cfg.RepoURL
	if !strings.HasPrefix(repoURL, "https://") && !strings.HasPrefix(repoURL, "http://") {
		repoURL = strings.TrimPrefix(repoURL, "oci://")
		if err := helm.LoginRegistry(b.ctx, strings.Split(repoURL, "/")[0], b.Env.RegistryUser, b.Env.RegistryPassword); err != nil {
			return fmt.Errorf("failed to log in to chart registry: %w", err)
		}
		chartName = "oci://" + strings.TrimSuffix(repoURL, "/") + "/" + cfg.Chart
		repoURL = ""
	}
	chart := installer.ChartConfig{
		ReleaseName: cfg.Name, ChartName: chartName, RepoURL: repoURL,
		Namespace: cfg.Namespace, Version: cfg.TargetRevision, Values: cfg.Values,
		CreateNamespace: true,
	}
	release, err := helm.FindRelease(cfg.Namespace, cfg.Name)
	if err != nil {
		return fmt.Errorf("failed to find Helm release %q: %w", cfg.Name, err)
	}
	if release == nil {
		if err := helm.InstallChart(b.ctx, chart, installer.InstallChartOptions{}); err != nil {
			return fmt.Errorf("failed to install Helm chart %q: %w", cfg.Name, err)
		}
		return nil
	}
	if err := helm.UpgradeChart(b.ctx, chart, installer.UpgradeChartOptions{}); err != nil {
		return fmt.Errorf("failed to upgrade Helm chart %q: %w", cfg.Name, err)
	}
	return nil
}
