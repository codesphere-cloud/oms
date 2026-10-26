// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package csapps

import (
	"context"
	"fmt"
	"os"

	"github.com/Masterminds/semver/v3"
	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"go.yaml.in/yaml/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// InstalledVersion returns the Codesphere version ArgoCD last deployed for the release cfg
// configures, or "" when ArgoCD has not deployed it yet.
func InstalledVersion(ctx context.Context, kubeClient client.Client, cfg *files.RootConfig) (string, error) {
	name := releaseName(cfg) + codesphereAppSuffix

	app := &argov1alpha1.Application{}
	if err := kubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: argocd.DefaultNamespace}, app); err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}

		return "", fmt.Errorf("failed to read ArgoCD Application %q: %w", name, err)
	}

	if len(app.Status.History) == 0 {
		return "", nil
	}

	revision := app.Status.History.LastRevisionHistory().Revision

	// The chart version is the Codesphere version with the image tag appended as a prerelease,
	// which the installer's migration order would sort before the version itself.
	version, err := semver.StrictNewVersion(revision)
	if err != nil {
		return "", fmt.Errorf("ArgoCD Application %q deployed chart version %q, which is not a semantic version: %w", name, revision, err)
	}

	return fmt.Sprintf("%d.%d.%d", version.Major(), version.Minor(), version.Patch()), nil
}

// HandOverToInstaller tells the Codesphere installer, through the merged config at configPath,
// that OMS deploys the chart, and which version is installed. The installer would otherwise read
// the installed version from a Helm release, which ArgoCD does not create.
func HandOverToInstaller(configPath, installedVersion string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config %s: %w", configPath, err)
	}

	config := map[string]any{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config %s: %w", configPath, err)
	}

	codesphere, ok := config["codesphere"].(map[string]any)
	if !ok {
		codesphere = map[string]any{}
		config["codesphere"] = codesphere
	}

	codesphere["skipChartDeploy"] = true
	if installedVersion != "" {
		codesphere["installedVersion"] = installedVersion
	}

	updated, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, updated, 0o600); err != nil {
		return fmt.Errorf("failed to write config %s: %w", configPath, err)
	}

	return nil
}
