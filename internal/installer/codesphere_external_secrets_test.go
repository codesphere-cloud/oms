// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var externalSecretGVK = schema.GroupVersionKind{Group: "external-secrets.io", Version: "v1", Kind: "ExternalSecret"}

func newExternalSecret(name, releaseName string) *unstructured.Unstructured {
	externalSecret := &unstructured.Unstructured{}
	externalSecret.SetGroupVersionKind(externalSecretGVK)
	externalSecret.SetNamespace("codesphere")
	externalSecret.SetName(name)
	externalSecret.SetAnnotations(map[string]string{"meta.helm.sh/release-name": releaseName})

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
			newExternalSecret("codesphere", "codesphere"), newESOSecret("codesphere"))

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "codesphere")).To(Succeed())

		Expect(policies).To(HaveKeyWithValue("codesphere", metav1.DeletePropagationOrphan))
		Expect(externalSecretExists(kubeClient, "codesphere")).To(BeFalse())
		Expect(getSecret(kubeClient, "codesphere").Data).To(HaveKeyWithValue("token", []byte("from-eso")))
	})

	It("marks the Secret as owned by the release, so Helm can adopt it", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{},
			newExternalSecret("codesphere", "codesphere"), newESOSecret("codesphere"))

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "codesphere")).To(Succeed())

		secret := getSecret(kubeClient, "codesphere")
		Expect(secret.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "Helm"))
		Expect(secret.Labels).To(HaveKeyWithValue("reconcile.external-secrets.io/managed", "true"))
		Expect(secret.Annotations).To(HaveKeyWithValue("meta.helm.sh/release-name", "codesphere"))
		Expect(secret.Annotations).To(HaveKeyWithValue("meta.helm.sh/release-namespace", "codesphere"))
	})

	It("orphans the EAB HMAC key ExternalSecret of the release", func() {
		policies := map[string]metav1.DeletionPropagation{}
		kubeClient := newExternalSecretsClient(recordDeletePropagation(policies),
			newExternalSecret("cd-preview-eab-hmac-key", "preview"), newESOSecret("cd-preview-eab-hmac-key"))

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "preview")).To(Succeed())

		Expect(policies).To(HaveKeyWithValue("cd-preview-eab-hmac-key", metav1.DeletePropagationOrphan))
		Expect(getSecret(kubeClient, "cd-preview-eab-hmac-key").Annotations).To(HaveKeyWithValue("meta.helm.sh/release-name", "preview"))
	})

	It("leaves the OpenFGA token ExternalSecret alone, because every chart renders it as ExternalSecret", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{},
			newExternalSecret("codesphere-openfga-token", "codesphere"))

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "codesphere")).To(Succeed())

		Expect(externalSecretExists(kubeClient, "codesphere-openfga-token")).To(BeTrue())
	})

	It("leaves an ExternalSecret of another release alone", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{},
			newExternalSecret("codesphere", "other-release"), newESOSecret("codesphere"))

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "codesphere")).To(Succeed())

		Expect(externalSecretExists(kubeClient, "codesphere")).To(BeTrue())
		Expect(getSecret(kubeClient, "codesphere").Labels).NotTo(HaveKey("app.kubernetes.io/managed-by"))
	})

	It("succeeds without ExternalSecrets, e.g. when the release renders plain Secrets", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{
			Delete: func(context.Context, ctrlclient.WithWatch, ctrlclient.Object, ...ctrlclient.DeleteOption) error {
				return fmt.Errorf("nothing must be deleted")
			},
		})

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "codesphere")).To(Succeed())
	})

	It("succeeds when the ExternalSecret CRD is not installed", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{
			Get: func(context.Context, ctrlclient.WithWatch, ctrlclient.ObjectKey, ctrlclient.Object, ...ctrlclient.GetOption) error {
				return &meta.NoKindMatchError{GroupKind: externalSecretGVK.GroupKind()}
			},
		})

		Expect(installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "codesphere")).To(Succeed())
	})

	It("reports a failed ExternalSecret deletion", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{
			Delete: func(context.Context, ctrlclient.WithWatch, ctrlclient.Object, ...ctrlclient.DeleteOption) error {
				return fmt.Errorf("forbidden")
			},
		}, newExternalSecret("codesphere", "codesphere"), newESOSecret("codesphere"))

		err := installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "codesphere")

		Expect(err).To(MatchError(ContainSubstring("failed to delete ExternalSecret codesphere/codesphere: forbidden")))
	})

	It("reports a failed Secret update and keeps the ExternalSecret", func() {
		kubeClient := newExternalSecretsClient(interceptor.Funcs{
			Patch: func(context.Context, ctrlclient.WithWatch, ctrlclient.Object, ctrlclient.Patch, ...ctrlclient.PatchOption) error {
				return fmt.Errorf("forbidden")
			},
		}, newExternalSecret("codesphere", "codesphere"), newESOSecret("codesphere"))

		err := installer.OrphanCodesphereExternalSecretsWithClient(context.Background(), kubeClient, "codesphere")

		Expect(err).To(MatchError(ContainSubstring("failed to mark Secret codesphere/codesphere as owned by Helm release codesphere: forbidden")))
		Expect(externalSecretExists(kubeClient, "codesphere")).To(BeTrue())
	})
})

var _ = Describe("CodesphereReleaseName", func() {
	It("defaults to codesphere like the installer", func() {
		Expect(installer.CodesphereReleaseName(files.RootConfig{})).To(Equal("codesphere"))
	})

	It("uses codesphere.releaseName from the config", func() {
		cfg := files.RootConfig{Codesphere: files.CodesphereConfig{ReleaseName: "preview"}}

		Expect(installer.CodesphereReleaseName(cfg)).To(Equal("preview"))
	})
})
