// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"
	"errors"
	"time"

	"github.com/argoproj/argo-cd/gitops-engine/pkg/health"
	synccommon "github.com/argoproj/argo-cd/gitops-engine/pkg/sync/common"
	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/codesphere-cloud/oms/internal/installer/argocd"
)

var _ = Describe("WaitForAppFreshlySyncedAndHealthy", func() {
	var (
		syncFinishedAt = metav1.NewTime(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
		beforeSync     = metav1.NewTime(syncFinishedAt.Add(-time.Second))
		afterSync      = metav1.NewTime(syncFinishedAt.Add(time.Second))
	)

	source := func(values string) argov1alpha1.ApplicationSource {
		return argov1alpha1.ApplicationSource{
			RepoURL:        "ghcr.io/codesphere-cloud/charts",
			Chart:          "codesphere",
			TargetRevision: "1.0.0",
			Helm:           &argov1alpha1.ApplicationSourceHelm{ValuesObject: &runtime.RawExtension{Raw: []byte(values)}},
		}
	}

	application := func(
		spec, compared argov1alpha1.ApplicationSource,
		healthStatus health.HealthStatusCode,
		syncStatus argov1alpha1.SyncStatusCode,
	) *argov1alpha1.Application {
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

		return argocd.WaitForAppFreshlySyncedAndHealthy(ctx, kubeClient, "pr-1-codesphere")
	}

	It("healthy and synced to the current source -> done", func() {
		app := application(
			source(`{"trigger":"b"}`),
			source(`{"trigger":"b"}`),
			health.HealthStatusHealthy,
			argov1alpha1.SyncStatusCodeSynced,
		)

		Expect(wait(app)).To(Succeed())
	})

	It("synced to the values of an earlier source -> keeps waiting", func() {
		app := application(
			source(`{"trigger":"b"}`),
			source(`{"trigger":"a"}`),
			health.HealthStatusHealthy,
			argov1alpha1.SyncStatusCodeSynced,
		)

		Expect(wait(app)).To(MatchError(ContainSubstring(`timed out waiting for ArgoCD Application "pr-1-codesphere"`)))
	})

	It("synced to the current source but degraded -> keeps waiting", func() {
		app := application(
			source(`{}`),
			source(`{}`),
			health.HealthStatusDegraded,
			argov1alpha1.SyncStatusCodeSynced,
		)

		Expect(wait(app)).To(MatchError(ContainSubstring(`health="Degraded"`)))
	})

	It("healthy but out of sync -> keeps waiting", func() {
		app := application(
			source(`{}`),
			source(`{}`),
			health.HealthStatusHealthy,
			argov1alpha1.SyncStatusCodeOutOfSync,
		)

		Expect(wait(app)).To(MatchError(ContainSubstring(`sync="OutOfSync"`)))
	})

	It("values with nulls, which Argo CD drops from the compared source -> done", func() {
		app := application(
			source(`{"global":{"tlsSecret":null,"list":[{"a":null,"b":1}],"trigger":"b"}}`),
			source(`{"global":{"list":[{"b":1}],"trigger":"b"}}`),
			health.HealthStatusHealthy,
			argov1alpha1.SyncStatusCodeSynced,
		)

		Expect(wait(app)).To(Succeed())
	})

	It("out of sync, compared after the sync of the current source succeeded -> done, drift does not block", func() {
		app := application(
			source(`{"trigger":"b"}`),
			source(`{"trigger":"b"}`),
			health.HealthStatusHealthy,
			argov1alpha1.SyncStatusCodeOutOfSync,
		)
		app.Status.OperationState = &argov1alpha1.OperationState{
			Phase:      synccommon.OperationSucceeded,
			SyncResult: &argov1alpha1.SyncOperationResult{Source: source(`{"trigger":"b"}`)},
			FinishedAt: &syncFinishedAt,
		}
		app.Status.ReconciledAt = &afterSync

		Expect(wait(app)).To(Succeed())
	})

	It("out of sync, the sync of the current source succeeded after the last comparison -> keeps waiting", func() {
		app := application(
			source(`{"trigger":"b"}`),
			source(`{"trigger":"b"}`),
			health.HealthStatusHealthy,
			argov1alpha1.SyncStatusCodeOutOfSync,
		)
		app.Status.OperationState = &argov1alpha1.OperationState{
			Phase:      synccommon.OperationSucceeded,
			SyncResult: &argov1alpha1.SyncOperationResult{Source: source(`{"trigger":"b"}`)},
			FinishedAt: &syncFinishedAt,
		}
		app.Status.ReconciledAt = &beforeSync

		Expect(wait(app)).To(MatchError(ContainSubstring("timed out")))
	})

	It("out of sync, the last succeeded sync was of an earlier source -> keeps waiting", func() {
		app := application(
			source(`{"trigger":"b"}`),
			source(`{"trigger":"b"}`),
			health.HealthStatusHealthy,
			argov1alpha1.SyncStatusCodeOutOfSync,
		)
		app.Status.OperationState = &argov1alpha1.OperationState{
			Phase:      synccommon.OperationSucceeded,
			SyncResult: &argov1alpha1.SyncOperationResult{Source: source(`{"trigger":"a"}`)},
		}

		Expect(wait(app)).To(MatchError(ContainSubstring("timed out")))
	})

	It("out of sync while the sync of the current source runs -> keeps waiting", func() {
		app := application(
			source(`{"trigger":"b"}`),
			source(`{"trigger":"b"}`),
			health.HealthStatusHealthy,
			argov1alpha1.SyncStatusCodeOutOfSync,
		)
		app.Status.OperationState = &argov1alpha1.OperationState{
			Phase:      synccommon.OperationRunning,
			SyncResult: &argov1alpha1.SyncOperationResult{Source: source(`{"trigger":"b"}`)},
		}

		Expect(wait(app)).To(MatchError(ContainSubstring("timed out")))
	})

	It("Application not created yet -> keeps waiting", func() {
		Expect(wait()).To(MatchError(ContainSubstring("timed out")))
	})

	waitWithGet := func(get func(ctx context.Context) error) error {
		scheme := runtime.NewScheme()
		Expect(argov1alpha1.AddToScheme(scheme)).To(Succeed())
		kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return get(ctx)
			},
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		return argocd.WaitForAppFreshlySyncedAndHealthy(ctx, kubeClient, "pr-1-codesphere")
	}

	It("deadline runs out during a read -> timeout error, not a read error", func() {
		err := waitWithGet(func(ctx context.Context) error {
			<-ctx.Done()
			return errors.New("client rate limiter Wait returned an error: rate: Wait(n=1) would exceed context deadline")
		})

		Expect(err).To(MatchError(ContainSubstring(`timed out waiting for ArgoCD Application "pr-1-codesphere"`)))
		Expect(err).NotTo(MatchError(ContainSubstring("failed to read")))
	})

	It("read fails for another reason -> that error, not a timeout", func() {
		err := waitWithGet(func(context.Context) error {
			return apierrors.NewForbidden(schema.GroupResource{Group: "argoproj.io", Resource: "applications"}, "pr-1-codesphere", errors.New("denied"))
		})

		Expect(err).To(MatchError(ContainSubstring(`failed to read ArgoCD Application "pr-1-codesphere"`)))
		Expect(err).NotTo(MatchError(ContainSubstring("timed out")))
	})
})

var _ = Describe("WaitForSubAppsFreshlySyncedAndHealthy", func() {
	const parentName = "pr-1-cs-applications"

	subAppResource := func(name string) argov1alpha1.ResourceStatus {
		return argov1alpha1.ResourceStatus{Group: "argoproj.io", Kind: "Application", Namespace: "argocd", Name: name}
	}

	application := func(name string, healthStatus health.HealthStatusCode, resources ...argov1alpha1.ResourceStatus) *argov1alpha1.Application {
		source := argov1alpha1.ApplicationSource{RepoURL: "ghcr.io/codesphere-cloud/charts", Chart: name, TargetRevision: "1.0.0"}

		return &argov1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "argocd"},
			Spec:       argov1alpha1.ApplicationSpec{Source: &source},
			Status: argov1alpha1.ApplicationStatus{
				Health:    argov1alpha1.AppHealthStatus{Status: healthStatus},
				Sync:      argov1alpha1.SyncStatus{Status: argov1alpha1.SyncStatusCodeSynced, ComparedTo: argov1alpha1.ComparedTo{Source: source}},
				Resources: resources,
			},
		}
	}

	parent := application(
		parentName,
		health.HealthStatusHealthy,
		argov1alpha1.ResourceStatus{Kind: "ConfigMap", Namespace: "argocd", Name: "pr-1-config"},
		subAppResource("pr-1-codesphere"),
		subAppResource("pr-1-mounter"),
	)

	wait := func(objects ...client.Object) error {
		scheme := runtime.NewScheme()
		Expect(argov1alpha1.AddToScheme(scheme)).To(Succeed())
		kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		return argocd.WaitForSubAppsFreshlySyncedAndHealthy(ctx, kubeClient, parentName)
	}

	It("all sub applications synced and healthy -> done", func() {
		Expect(wait(
			parent,
			application("pr-1-codesphere", health.HealthStatusHealthy),
			application("pr-1-mounter", health.HealthStatusHealthy),
		)).To(Succeed())
	})

	It("one sub application degraded -> error naming only that one", func() {
		err := wait(
			parent,
			application("pr-1-codesphere", health.HealthStatusHealthy),
			application("pr-1-mounter", health.HealthStatusDegraded),
		)

		Expect(err).To(MatchError(ContainSubstring(`"pr-1-mounter"`)))
		Expect(err).NotTo(MatchError(ContainSubstring(`"pr-1-codesphere"`)))
	})

	It("parent missing -> read error", func() {
		Expect(wait()).To(MatchError(ContainSubstring(`failed to read ArgoCD Application "pr-1-cs-applications"`)))
	})
})
