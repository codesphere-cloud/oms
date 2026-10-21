// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"
	"time"

	"github.com/argoproj/argo-cd/gitops-engine/pkg/health"
	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/codesphere-cloud/oms/internal/installer/argocd"
)

var _ = Describe("WaitForApplicationSynced", func() {
	source := func(values string) argov1alpha1.ApplicationSource {
		return argov1alpha1.ApplicationSource{
			RepoURL:        "ghcr.io/codesphere-cloud/charts",
			Chart:          "codesphere",
			TargetRevision: "1.0.0",
			Helm:           &argov1alpha1.ApplicationSourceHelm{ValuesObject: &runtime.RawExtension{Raw: []byte(values)}},
		}
	}

	application := func(spec, compared argov1alpha1.ApplicationSource, healthStatus health.HealthStatusCode, syncStatus argov1alpha1.SyncStatusCode) *argov1alpha1.Application {
		return &argov1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "pr-1-codesphere", Namespace: "argocd"},
			Spec:       argov1alpha1.ApplicationSpec{Source: &spec},
			Status: argov1alpha1.ApplicationStatus{
				Health: argov1alpha1.AppHealthStatus{Status: healthStatus},
				Sync:   argov1alpha1.SyncStatus{Status: syncStatus, ComparedTo: argov1alpha1.ComparedTo{Source: compared}},
			},
		}
	}

	wait := func(objects ...client.Object) error {
		scheme := runtime.NewScheme()
		Expect(argov1alpha1.AddToScheme(scheme)).To(Succeed())
		kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		return argocd.WaitForApplicationSynced(ctx, kubeClient, "pr-1-codesphere", nil)
	}

	It("healthy and synced to the current source -> done", func() {
		app := application(source(`{"trigger":"b"}`), source(`{"trigger":"b"}`), health.HealthStatusHealthy, argov1alpha1.SyncStatusCodeSynced)

		Expect(wait(app)).To(Succeed())
	})

	It("synced to the values of an earlier source -> keeps waiting", func() {
		app := application(source(`{"trigger":"b"}`), source(`{"trigger":"a"}`), health.HealthStatusHealthy, argov1alpha1.SyncStatusCodeSynced)

		Expect(wait(app)).To(MatchError(ContainSubstring(`timed out waiting for ArgoCD Application "pr-1-codesphere"`)))
	})

	It("synced to the current source but degraded -> keeps waiting", func() {
		app := application(source(`{}`), source(`{}`), health.HealthStatusDegraded, argov1alpha1.SyncStatusCodeSynced)

		Expect(wait(app)).To(MatchError(ContainSubstring(`health="Degraded"`)))
	})

	It("healthy but out of sync -> keeps waiting", func() {
		app := application(source(`{}`), source(`{}`), health.HealthStatusHealthy, argov1alpha1.SyncStatusCodeOutOfSync)

		Expect(wait(app)).To(MatchError(ContainSubstring(`sync="OutOfSync"`)))
	})

	It("Application not created yet -> keeps waiting", func() {
		Expect(wait()).To(MatchError(ContainSubstring("timed out")))
	})
})
