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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var externalSecretGVK = schema.GroupVersionKind{Group: "external-secrets.io", Version: "v1", Kind: "ExternalSecret"}

func newExternalSecret(name string) *unstructured.Unstructured {
	externalSecret := &unstructured.Unstructured{}
	externalSecret.SetGroupVersionKind(externalSecretGVK)
	externalSecret.SetNamespace("codesphere")
	externalSecret.SetName(name)

	return externalSecret
}

func newESOSecret(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "codesphere",
			Labels:    map[string]string{"reconcile.external-secrets.io/managed": "true"},
		},
		Data: map[string][]byte{"token": []byte("from-eso")},
	}
}

func newExternalSecretsClient(interceptors interceptor.Funcs, objects ...ctrlclient.Object) ctrlclient.Client {
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithInterceptorFuncs(interceptors).
		Build()
}

func externalSecretExists(kubeClient ctrlclient.Client, name string) bool {
	externalSecret := &unstructured.Unstructured{}
	externalSecret.SetGroupVersionKind(externalSecretGVK)

	err := kubeClient.Get(context.Background(), ctrlclient.ObjectKey{Namespace: "codesphere", Name: name}, externalSecret)
	if apierrors.IsNotFound(err) {
		return false
	}

	Expect(err).NotTo(HaveOccurred())

	return true
}

func getSecret(kubeClient ctrlclient.Client, name string) *corev1.Secret {
	secret := &corev1.Secret{}
	Expect(kubeClient.Get(context.Background(), ctrlclient.ObjectKey{Namespace: "codesphere", Name: name}, secret)).To(Succeed())

	return secret
}

// recordDeletePropagation captures the propagation policy of each ExternalSecret deletion.
func recordDeletePropagation(policies map[string]metav1.DeletionPropagation) interceptor.Funcs {
	return interceptor.Funcs{
		Delete: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.DeleteOption) error {
			deleteOpts := &ctrlclient.DeleteOptions{}
			deleteOpts.ApplyOptions(opts)

			if deleteOpts.PropagationPolicy != nil {
				policies[obj.GetName()] = *deleteOpts.PropagationPolicy
			}

			return client.Delete(ctx, obj, opts...)
		},
	}
}

var _ = Describe("OrphanCodesphereExternalSecrets", func() {
	It("deletes the release ExternalSecret with orphan propagation, so its Secret is not garbage-collected", func() {
		policies := map[string]metav1.DeletionPropagation{}
		kubeClient := newExternalSecretsClient(recordDeletePropagation(policies),
			newExternalSecret("codesphere"), newESOSecret("codesphere"))

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "")).To(Succeed())

		Expect(policies).To(HaveKeyWithValue("codesphere", metav1.DeletePropagationOrphan))
		Expect(externalSecretExists(kubeClient, "codesphere")).To(BeFalse())
		Expect(getSecret(kubeClient, "codesphere").Data).To(HaveKeyWithValue("token", []byte("from-eso")))
	})

	It("marks the Secret as owned by the release, so Helm can adopt it", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{},
			newExternalSecret("codesphere"), newESOSecret("codesphere"))

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "")).To(Succeed())

		secret := getSecret(kubeClient, "codesphere")
		Expect(secret.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "Helm"))
		Expect(secret.Labels).To(HaveKeyWithValue("reconcile.external-secrets.io/managed", "true"))
		Expect(secret.Annotations).To(HaveKeyWithValue("meta.helm.sh/release-name", "codesphere"))
		Expect(secret.Annotations).To(HaveKeyWithValue("meta.helm.sh/release-namespace", "codesphere"))
	})

	It("marks the Secret only after the garbage collector removed the ExternalSecret", func() {
		calls := []string{}
		terminatingPolls := 1
		kubeClient := newExternalSecretsClient(interceptor.Funcs{
			Delete: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, opts ...ctrlclient.DeleteOption) error {
				calls = append(calls, "delete "+obj.GetName())

				return client.Delete(ctx, obj, opts...)
			},
			Get: func(ctx context.Context, client ctrlclient.WithWatch, key ctrlclient.ObjectKey, obj ctrlclient.Object, opts ...ctrlclient.GetOption) error {
				if _, isExternalSecret := obj.(*unstructured.Unstructured); isExternalSecret {
					calls = append(calls, "get "+key.Name)
					// The API server keeps an orphan-deleted object until the garbage collector removed its finalizer.
					if terminatingPolls > 0 {
						terminatingPolls--

						return nil
					}
				}

				return client.Get(ctx, key, obj, opts...)
			},
			Patch: func(ctx context.Context, client ctrlclient.WithWatch, obj ctrlclient.Object, patch ctrlclient.Patch, opts ...ctrlclient.PatchOption) error {
				calls = append(calls, "patch "+obj.GetName())

				return client.Patch(ctx, obj, patch, opts...)
			},
		}, newExternalSecret("codesphere"), newESOSecret("codesphere"))

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "")).To(Succeed())

		Expect(calls).To(Equal([]string{
			"delete codesphere", "get codesphere", "get codesphere", "patch codesphere",
			"delete cd-codesphere-eab-hmac-key",
		}))
	})

	It("reports a failed wait for the ExternalSecret deletion and leaves the Secret unmarked", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{
			Get: func(ctx context.Context, client ctrlclient.WithWatch, key ctrlclient.ObjectKey, obj ctrlclient.Object, opts ...ctrlclient.GetOption) error {
				if _, isExternalSecret := obj.(*unstructured.Unstructured); isExternalSecret {
					return fmt.Errorf("connection refused")
				}

				return client.Get(ctx, key, obj, opts...)
			},
		}, newExternalSecret("codesphere"), newESOSecret("codesphere"))

		err := installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "")

		Expect(err).To(MatchError(ContainSubstring("failed to wait for deletion of ExternalSecret codesphere/codesphere")))
		Expect(err).To(MatchError(ContainSubstring("connection refused")))
		Expect(getSecret(kubeClient, "codesphere").Labels).NotTo(HaveKey("app.kubernetes.io/managed-by"))
	})

	It("orphans the EAB HMAC key ExternalSecret of the release", func() {
		policies := map[string]metav1.DeletionPropagation{}
		kubeClient := newExternalSecretsClient(recordDeletePropagation(policies),
			newExternalSecret("cd-codesphere-eab-hmac-key"), newESOSecret("cd-codesphere-eab-hmac-key"))

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "")).To(Succeed())

		Expect(policies).To(HaveKeyWithValue("cd-codesphere-eab-hmac-key", metav1.DeletePropagationOrphan))
		Expect(getSecret(kubeClient, "cd-codesphere-eab-hmac-key").Annotations).To(HaveKeyWithValue("meta.helm.sh/release-name", "codesphere"))
	})

	It("leaves the OpenFGA token ExternalSecret alone, because every chart renders it as ExternalSecret", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{},
			newExternalSecret("codesphere-openfga-token"))

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "")).To(Succeed())

		Expect(externalSecretExists(kubeClient, "codesphere-openfga-token")).To(BeTrue())
	})

	It("finishes the handover on a retry after the ownership patch failed", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{}, newESOSecret("codesphere"))

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "")).To(Succeed())

		Expect(getSecret(kubeClient, "codesphere").Annotations).To(HaveKeyWithValue("meta.helm.sh/release-name", "codesphere"))
	})

	It("leaves the default release alone when installing a release with its own name", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{}, newExternalSecret("codesphere"), newESOSecret("codesphere"))

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "codesphere-preview")).To(Succeed())

		Expect(externalSecretExists(kubeClient, "codesphere")).To(BeTrue())
		Expect(getSecret(kubeClient, "codesphere").Labels).NotTo(HaveKey("app.kubernetes.io/managed-by"))
	})

	It("handles the default release named explicitly", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{}, newExternalSecret("codesphere"), newESOSecret("codesphere"))

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "codesphere")).To(Succeed())

		Expect(externalSecretExists(kubeClient, "codesphere")).To(BeFalse())
	})

	It("leaves the Secret alone when the release renders plain Secrets", func() {
		plainSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "codesphere", Namespace: "codesphere"}}
		kubeClient := newExternalSecretsClient(interceptor.Funcs{
			Patch: func(context.Context, ctrlclient.WithWatch, ctrlclient.Object, ctrlclient.Patch, ...ctrlclient.PatchOption) error {
				return fmt.Errorf("the plain Secret must not be patched")
			},
		}, plainSecret)

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "")).To(Succeed())
	})

	It("reports a failed ExternalSecret deletion", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{
			Delete: func(context.Context, ctrlclient.WithWatch, ctrlclient.Object, ...ctrlclient.DeleteOption) error {
				return fmt.Errorf("forbidden")
			},
		}, newExternalSecret("codesphere"), newESOSecret("codesphere"))

		err := installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "")

		Expect(err).To(MatchError(ContainSubstring("failed to delete ExternalSecret codesphere/codesphere: forbidden")))
		Expect(getSecret(kubeClient, "codesphere").Labels).NotTo(HaveKey("app.kubernetes.io/managed-by"))
	})

	It("reports a failed Secret update", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{
			Patch: func(context.Context, ctrlclient.WithWatch, ctrlclient.Object, ctrlclient.Patch, ...ctrlclient.PatchOption) error {
				return fmt.Errorf("forbidden")
			},
		}, newExternalSecret("codesphere"), newESOSecret("codesphere"))

		err := installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "")

		Expect(err).To(MatchError(ContainSubstring("failed to mark Secret codesphere/codesphere as owned by Helm release codesphere: forbidden")))
	})
})
