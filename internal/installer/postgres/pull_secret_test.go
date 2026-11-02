// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"errors"

	"github.com/codesphere-cloud/oms/internal/installer/postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Masterdata image pull credentials", func() {
	var (
		kube   *MockClient
		pg     *postgres.Installer
		source *corev1.Secret
	)

	BeforeEach(func() {
		kube = NewMockClient(GinkgoT())
		pg = &postgres.Installer{KubeClient: kube}
		source = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "docker-regcred", Namespace: "argocd", UID: "source-secret", ResourceVersion: "3"},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte(`{"auths":{"registry.example.com":{"auth":"example"}}}`)},
		}
	})

	expectSource := func() {
		GinkgoHelper()
		kube.EXPECT().Get(mock.Anything, client.ObjectKey{Namespace: "argocd", Name: "docker-regcred"}, mock.AnythingOfType("*v1.Secret")).RunAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			*obj.(*corev1.Secret) = *source.DeepCopy()
			return nil
		}).Once()
	}
	expectNamespace := func() {
		GinkgoHelper()
		kube.EXPECT().Get(mock.Anything, client.ObjectKey{Name: "codesphere"}, mock.AnythingOfType("*v1.Namespace")).Return(nil).Once()
	}

	It("creates namespace-local credentials before the operator and database need them", func() {
		expectSource()
		kube.EXPECT().Get(mock.Anything, client.ObjectKey{Name: "codesphere"}, mock.AnythingOfType("*v1.Namespace")).Return(apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, "codesphere")).Once()
		namespace := kube.EXPECT().Create(mock.Anything, mock.AnythingOfType("*v1.Namespace")).RunAndReturn(func(_ context.Context, obj client.Object, _ ...client.CreateOption) error {
			Expect(obj.GetName()).To(Equal("codesphere"))
			return nil
		}).Once()
		kube.EXPECT().Get(mock.Anything, client.ObjectKey{Namespace: "codesphere", Name: "oms-cnpg-regcred"}, mock.AnythingOfType("*v1.Secret")).Return(apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, "oms-cnpg-regcred")).Once()
		kube.EXPECT().Create(mock.Anything, mock.AnythingOfType("*v1.Secret")).RunAndReturn(func(_ context.Context, obj client.Object, _ ...client.CreateOption) error {
			secret := obj.(*corev1.Secret)
			Expect(secret.Namespace).To(Equal("codesphere"))
			Expect(secret.Name).To(Equal("oms-cnpg-regcred"))
			Expect(secret.Type).To(Equal(corev1.SecretTypeDockerConfigJson))
			Expect(secret.Data).To(Equal(source.Data))
			Expect(secret.UID).To(BeEmpty())
			Expect(secret.ResourceVersion).To(BeEmpty())

			return nil
		}).NotBefore(namespace).Once()
		Expect(pg.EnsureImagePullSecret(context.Background())).To(Succeed())
	})
	It("refreshes existing credentials without replacing destination metadata", func() {
		expectSource()
		expectNamespace()
		kube.EXPECT().Get(mock.Anything, client.ObjectKey{Namespace: "codesphere", Name: "oms-cnpg-regcred"}, mock.AnythingOfType("*v1.Secret")).RunAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			secret := obj.(*corev1.Secret)
			secret.ResourceVersion = "5"
			secret.Labels = map[string]string{"keep": "label"}
			secret.Type = corev1.SecretTypeDockerConfigJson
			secret.Data = map[string][]byte{corev1.DockerConfigJsonKey: []byte("old credentials")}

			return nil
		}).Once()
		kube.EXPECT().Update(mock.Anything, mock.AnythingOfType("*v1.Secret")).RunAndReturn(func(_ context.Context, obj client.Object, _ ...client.UpdateOption) error {
			secret := obj.(*corev1.Secret)
			Expect(secret.Data).To(Equal(source.Data))
			Expect(secret.ResourceVersion).To(Equal("5"))
			Expect(secret.Labels).To(HaveKeyWithValue("keep", "label"))

			return nil
		}).Once()
		Expect(pg.EnsureImagePullSecret(context.Background())).To(Succeed())
	})
	It("reports a missing Argo CD secret before deploying workloads", func() {
		kube.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("credentials unavailable")).Once()
		Expect(pg.EnsureImagePullSecret(context.Background())).To(MatchError(ContainSubstring("read Argo CD image pull secret")))
	})
})
