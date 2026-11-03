// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package csapps

import (
	"context"
	"fmt"

	"github.com/Masterminds/semver/v3"
	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
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
