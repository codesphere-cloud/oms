// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package argocd_test

import (
	"context"
	"errors"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CNPG Argo CD application", func() {
	var (
		kube       *MockClient
		packageBOM *bom.Config
	)

	BeforeEach(func() {
		kube = NewMockClient(GinkgoT())
		packageBOM = &bom.Config{Components: map[string]bom.ComponentConfig{
			"postgres-operator": {Files: map[string]bom.FileRef{"chart": {OciRef: "oci://original.example.com/charts/postgres-operator:1.2.3"}}},
		}}
	})
	It("uses the Argo CD registry and BOM version and waits for the operator to sync", func() {
		key := client.ObjectKey{Namespace: "argocd", Name: "cnpg"}

		var requestedSource *argov1alpha1.ApplicationSource

		kube.EXPECT().Get(mock.Anything, key, mock.Anything).Return(apierrors.NewNotFound(schema.GroupResource{Resource: "applications"}, "cnpg")).Once()
		kube.EXPECT().Create(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, obj client.Object, _ ...client.CreateOption) error {
			app := obj.(*argov1alpha1.Application)
			requestedSource = app.Spec.Source.DeepCopy()
			Expect(app.Spec.Source.RepoURL).To(Equal("mirror.example.com/custom/charts"))
			Expect(app.Spec.Source.Chart).To(Equal("postgres-operator"))
			Expect(app.Spec.Source.TargetRevision).To(Equal("1.2.3"))
			Expect(app.Spec.Source.Helm.ReleaseName).To(Equal("cnpg"))
			Expect(string(app.Spec.Source.Helm.ValuesObject.Raw)).To(MatchJSON(`{"cloudnative-pg":{"priorityClassName":""}}`))
			Expect(app.Spec.Destination.Namespace).To(Equal("codesphere"))
			Expect(app.Spec.SyncPolicy.SyncOptions).To(ContainElement("ServerSideApply=true"))

			return nil
		}).Once()
		kube.EXPECT().Get(mock.Anything, key, mock.Anything).RunAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			app := obj.(*argov1alpha1.Application)
			app.Status.Health.Status = "Healthy"
			app.Status.Sync.Status = argov1alpha1.SyncStatusCodeSynced
			app.Status.Sync.ComparedTo.Source = *requestedSource.DeepCopy()

			return nil
		}).Once()
		Expect(argocd.InstallCNPGOperator(context.Background(), kube, packageBOM, "mirror.example.com/custom/charts", map[string]any{"cloudnative-pg": map[string]any{"priorityClassName": ""}})).To(Succeed())
	})
	It("requires the operator chart in the BOM", func() {
		Expect(argocd.InstallCNPGOperator(context.Background(), kube, &bom.Config{}, "mirror.example.com/custom/charts", nil)).To(MatchError(ContainSubstring("missing from BOM")))
	})
	It("reports application API errors", func() {
		kube.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("application API unavailable")).Once()
		Expect(argocd.InstallCNPGOperator(context.Background(), kube, packageBOM, "mirror.example.com/custom/charts", nil)).To(MatchError(ContainSubstring("application API unavailable")))
	})
})
