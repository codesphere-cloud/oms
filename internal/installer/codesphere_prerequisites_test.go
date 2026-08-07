// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func newPrerequisitesClient(objects ...ctrlclient.Object) ctrlclient.Client {
	return newPrerequisitesClientWithInterceptors(interceptor.Funcs{}, objects...)
}

func newPrerequisitesClientWithInterceptors(interceptors interceptor.Funcs, objects ...ctrlclient.Object) ctrlclient.Client {
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithInterceptorFuncs(interceptors).
		Build()
}

var _ = Describe("EnsureCodespherePrerequisites", func() {
	It("creates the Codesphere namespace and dummy error-page service", func() {
		kubeClient := newPrerequisitesClient()

		Expect(installer.EnsureCodespherePrerequisites(context.Background(), kubeClient)).To(Succeed())

		namespace := &corev1.Namespace{}
		Expect(kubeClient.Get(context.Background(), ctrlclient.ObjectKey{Name: "codesphere"}, namespace)).To(Succeed())

		service := &corev1.Service{}
		Expect(kubeClient.Get(context.Background(), ctrlclient.ObjectKey{Name: "error-page-server", Namespace: "codesphere"}, service)).To(Succeed())
		Expect(service.Spec.Ports).To(HaveLen(1))
		Expect(service.Spec.Ports[0].Port).To(Equal(int32(8080)))
	})

	It("leaves existing resources untouched", func() {
		existingNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   "codesphere",
			Labels: map[string]string{"owner": "existing-install"},
		}}
		existingService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "error-page-server", Namespace: "codesphere"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 9090}}},
		}
		kubeClient := newPrerequisitesClientWithInterceptors(interceptor.Funcs{
			Create: func(context.Context, ctrlclient.WithWatch, ctrlclient.Object, ...ctrlclient.CreateOption) error {
				return fmt.Errorf("existing resources must not be recreated")
			},
		}, existingNamespace, existingService)

		Expect(installer.EnsureCodespherePrerequisites(context.Background(), kubeClient)).To(Succeed())

		namespace := &corev1.Namespace{}
		Expect(kubeClient.Get(context.Background(), ctrlclient.ObjectKey{Name: "codesphere"}, namespace)).To(Succeed())
		Expect(namespace.Labels).To(HaveKeyWithValue("owner", "existing-install"))

		service := &corev1.Service{}
		Expect(kubeClient.Get(context.Background(), ctrlclient.ObjectKey{Name: "error-page-server", Namespace: "codesphere"}, service)).To(Succeed())
		Expect(service.Spec.Ports[0].Port).To(Equal(int32(9090)))
	})

	It("reports namespace creation failures", func() {
		kubeClient := newPrerequisitesClientWithInterceptors(interceptor.Funcs{
			Create: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
				if _, isNamespace := obj.(*corev1.Namespace); isNamespace {
					return fmt.Errorf("kubectl error")
				}
				return client.Create(ctx, obj, opts...)
			},
		})

		Expect(installer.EnsureCodespherePrerequisites(context.Background(), kubeClient)).To(MatchError(ContainSubstring("failed to create Codesphere namespace")))
	})

	It("reports service creation failures", func() {
		kubeClient := newPrerequisitesClientWithInterceptors(interceptor.Funcs{
			Create: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.CreateOption) error {
				if _, isService := obj.(*corev1.Service); isService {
					return fmt.Errorf("kubectl error")
				}
				return client.Create(ctx, obj, opts...)
			},
		})

		Expect(installer.EnsureCodespherePrerequisites(context.Background(), kubeClient)).To(MatchError(ContainSubstring("failed to create dummy error-page-server service")))
	})
})
