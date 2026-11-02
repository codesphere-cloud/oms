// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// InstallCNPGOperator registers the BOM operator chart with Argo CD and waits
// for its CRDs and controller before callers create database clusters. The repository
// uses the registry URL and credentials already configured by the Argo CD step.
func InstallCNPGOperator(ctx context.Context, kube client.Client, packageBOM *bom.Config, repoURL string, values map[string]any) error {
	if packageBOM == nil {
		return fmt.Errorf("installer BOM is not prepared")
	}

	chart, ok := packageBOM.GetChart("postgres-operator")
	if !ok {
		return fmt.Errorf("CloudNativePG chart is missing from BOM component %q", "postgres-operator")
	}

	raw, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("marshal CNPG operator values: %w", err)
	}

	app := &argov1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "cnpg", Namespace: DefaultNamespace}}

	_, err = controllerutil.CreateOrUpdate(ctx, kube, app, func() error {
		app.Spec = argov1alpha1.ApplicationSpec{
			Project:     "default",
			Source:      &argov1alpha1.ApplicationSource{RepoURL: repoURL, Chart: path.Base(chart.Name()), TargetRevision: chart.Tag(), Helm: &argov1alpha1.ApplicationSourceHelm{ReleaseName: "cnpg", ValuesObject: &runtime.RawExtension{Raw: raw}}},
			Destination: argov1alpha1.ApplicationDestination{Server: "https://kubernetes.default.svc", Namespace: "codesphere"},
			SyncPolicy:  &argov1alpha1.SyncPolicy{Automated: &argov1alpha1.SyncPolicyAutomated{Prune: ptr.To(true)}, SyncOptions: argov1alpha1.SyncOptions{"CreateNamespace=true", "ServerSideApply=true"}},
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("apply CNPG Argo CD application: %w", err)
	}

	return WaitForApplicationHealthy(ctx, kube, app.Name, *app.Spec.Source, 0, log.Printf)
}
