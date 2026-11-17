// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	codesphereNamespace       = "codesphere"
	dummyErrorPageServiceName = "error-page-server"
	// dummyErrorPageServicePort and its name match the Service of the error-page-server chart,
	// so pc-applications can take the Service over.
	// Server-side apply keeps ports it doesn't own,
	// and the API server rejects a second port next to an unnamed one.
	dummyErrorPageServicePort = 8083
	// legacyDummyErrorPageServicePort is the unnamed port older OMS versions gave the placeholder.
	legacyDummyErrorPageServicePort = 8080
)

// EnsureCodespherePrerequisites creates the resources required before the
// Codesphere dependency charts are installed. Existing resources are left
// untouched so Helm or a previous installation can continue to own them.
// The one exception is an older OMS's error-page-server placeholder,
// which moves to the port of the error-page-server chart.
func EnsureCodespherePrerequisites(ctx context.Context, kubeClient ctrlclient.Client) error {
	namespace := &corev1.Namespace{}
	if err := kubeClient.Get(ctx, ctrlclient.ObjectKey{Name: codesphereNamespace}, namespace); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to check Codesphere namespace: %w", err)
		}

		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: codesphereNamespace},
		}
		if err := kubeClient.Create(ctx, namespace); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create Codesphere namespace: %w", err)
		}
	}

	service := &corev1.Service{}

	serviceKey := ctrlclient.ObjectKey{Name: dummyErrorPageServiceName, Namespace: codesphereNamespace}
	if err := kubeClient.Get(ctx, serviceKey, service); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to check dummy error-page-server service: %w", err)
		}

		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dummyErrorPageServiceName,
				Namespace: codesphereNamespace,
			},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Ports:    dummyErrorPageServicePorts(),
				Selector: map[string]string{"app": dummyErrorPageServiceName},
			},
		}
		if err := kubeClient.Create(ctx, service); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create dummy error-page-server service: %w", err)
		}

		return nil
	}

	if isLegacyDummyErrorPageService(service) {
		patch := ctrlclient.MergeFrom(service.DeepCopy())
		service.Spec.Ports = dummyErrorPageServicePorts()
		if err := kubeClient.Patch(ctx, service, patch); err != nil {
			return fmt.Errorf("failed to migrate dummy error-page-server service to port %d: %w", dummyErrorPageServicePort, err)
		}
	}

	return nil
}

func dummyErrorPageServicePorts() []corev1.ServicePort {
	return []corev1.ServicePort{{
		Name:       "web",
		Port:       dummyErrorPageServicePort,
		TargetPort: intstr.FromInt32(dummyErrorPageServicePort),
	}}
}

// isLegacyDummyErrorPageService reports whether service is the placeholder of an older OMS:
// a single unnamed port 8080, managed by neither Helm nor ArgoCD.
// An earlier dependencies install can leave it behind,
// when the platform install that replaces it never ran.
func isLegacyDummyErrorPageService(service *corev1.Service) bool {
	if _, helm := service.Annotations["meta.helm.sh/release-name"]; helm {
		return false
	}

	if _, argoCD := service.Annotations["argocd.argoproj.io/tracking-id"]; argoCD {
		return false
	}

	ports := service.Spec.Ports

	return len(ports) == 1 && ports[0].Name == "" && ports[0].Port == legacyDummyErrorPageServicePort
}
