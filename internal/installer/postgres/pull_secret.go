// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/argocd"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const cnpgImagePullSecretName = "oms-cnpg-regcred"

// EnsureImagePullSecret reuses Argo CD's registry credentials in the database
// namespace before the operator and database pods are created.
func (b *Installer) EnsureImagePullSecret(ctx context.Context) error {
	source := &corev1.Secret{}
	if err := b.KubeClient.Get(ctx, client.ObjectKey{Namespace: argocd.DefaultNamespace, Name: argocd.ImagePullSecretName}, source); err != nil {
		return fmt.Errorf("read Argo CD image pull secret: %w", err)
	}

	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: b.namespace()}}
	if _, err := controllerutil.CreateOrUpdate(ctx, b.KubeClient, namespace, func() error { return nil }); err != nil {
		return fmt.Errorf("ensure database namespace: %w", err)
	}

	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: b.namespace(), Name: cnpgImagePullSecretName}}
	if _, err := controllerutil.CreateOrUpdate(ctx, b.KubeClient, secret, func() error {
		secret.Type = source.Type
		secret.Data = source.Data

		return nil
	}); err != nil {
		return fmt.Errorf("apply database image pull secret: %w", err)
	}

	return nil
}
