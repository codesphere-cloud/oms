// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package local implements single-node Codesphere cluster bootstrapping.
package local

import (
	"encoding/json"
	"fmt"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type helmApplicationConfig struct {
	Name, Chart, RepoURL, TargetRevision, Namespace string
	Values                                          map[string]interface{}
}

func (b *LocalBootstrapper) installHelmApplication(cfg helmApplicationConfig) error {
	rawValues, err := json.Marshal(cfg.Values)
	if err != nil {
		return fmt.Errorf("failed to marshal values for ArgoCD Application %q: %w", cfg.Name, err)
	}

	desired := &argov1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: cfg.Name, Namespace: "argocd"},
		Spec: argov1alpha1.ApplicationSpec{
			Project: "default",
			Source: &argov1alpha1.ApplicationSource{
				RepoURL: cfg.RepoURL, Chart: cfg.Chart, TargetRevision: cfg.TargetRevision,
				Helm: &argov1alpha1.ApplicationSourceHelm{ReleaseName: cfg.Name, ValuesObject: &runtime.RawExtension{Raw: rawValues}},
			},
			Destination: argov1alpha1.ApplicationDestination{Server: "https://kubernetes.default.svc", Namespace: cfg.Namespace},
			SyncPolicy: &argov1alpha1.SyncPolicy{
				Automated:   &argov1alpha1.SyncPolicyAutomated{Prune: ptr.To(true), SelfHeal: ptr.To(true)},
				SyncOptions: argov1alpha1.SyncOptions{"CreateNamespace=true", "ServerSideApply=true"},
			},
		},
	}

	current := &argov1alpha1.Application{ObjectMeta: desired.ObjectMeta}
	if _, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, current, func() error {
		current.Spec = desired.Spec
		return nil
	}); err != nil {
		return fmt.Errorf("failed to apply ArgoCD Application %q: %w", cfg.Name, err)
	}

	if err := argocd.WaitForApplicationHealthy(b.ctx, b.kubeClient, cfg.Name, cfg.TargetRevision, b.stlog.Logf); err != nil {
		return fmt.Errorf("failed to wait for ArgoCD Application %q: %w", cfg.Name, err)
	}

	return nil
}
