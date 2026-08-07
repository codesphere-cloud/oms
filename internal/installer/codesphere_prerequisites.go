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
)

// EnsureCodespherePrerequisites creates the resources required before the
// Codesphere dependency charts are installed. Existing resources are left
// untouched so Helm or a previous installation can continue to own them.
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
				Type: corev1.ServiceTypeClusterIP,
				Ports: []corev1.ServicePort{{
					Port:       8080,
					TargetPort: intstr.FromInt32(8080),
				}},
				Selector: map[string]string{"app": dummyErrorPageServiceName},
			},
		}
		if err := kubeClient.Create(ctx, service); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create dummy error-page-server service: %w", err)
		}
	}

	return nil
}
