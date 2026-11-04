// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"time"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultCodesphereReleaseName = "codesphere"

	helmManagedByLabel             = "app.kubernetes.io/managed-by"
	helmManagedByValue             = "Helm"
	helmReleaseNameAnnotation      = "meta.helm.sh/release-name"
	helmReleaseNamespaceAnnotation = "meta.helm.sh/release-namespace"

	externalSecretDeletionTimeout      = 2 * time.Minute
	externalSecretDeletionPollInterval = time.Second
)

var externalSecretGVK = schema.GroupVersionKind{Group: "external-secrets.io", Version: "v1", Kind: "ExternalSecret"}

// OrphanCodesphereExternalSecrets loads the cluster from the vault and hands the Secrets of
// the Codesphere release's ExternalSecrets over to Helm, see orphanCodesphereExternalSecrets.
func OrphanCodesphereExternalSecrets(ctx context.Context, vaultPath, privKey, vaultType string, cfg files.RootConfig) error {
	_, restConfig, err := VaultAndRESTConfig(vaultPath, privKey, vaultType, cfg)
	if err != nil {
		return err
	}

	kubeClient, err := NewKubeClient(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return orphanCodesphereExternalSecrets(ctx, kubeClient, codesphereReleaseName(cfg))
}

// orphanCodesphereExternalSecrets deletes the release's ExternalSecrets without their Secrets.
// A target release that renders them as plain Secrets (e.g. a rollback) then adopts the Secrets
// instead of failing, because ESO's ownerReference would otherwise garbage-collect them once
// Helm deletes the ExternalSecret. A target release that renders the ExternalSecrets again
// recreates them, and ESO takes the existing Secrets back.
func orphanCodesphereExternalSecrets(ctx context.Context, kubeClient ctrlclient.Client, releaseName string) error {
	for _, name := range codesphereExternalSecretNames(releaseName) {
		if err := orphanExternalSecret(ctx, kubeClient, releaseName, name); err != nil {
			return err
		}
	}

	return nil
}

func codesphereReleaseName(cfg files.RootConfig) string {
	if cfg.Codesphere.ReleaseName != "" {
		return cfg.Codesphere.ReleaseName
	}

	return defaultCodesphereReleaseName
}

// codesphereExternalSecretNames lists the ExternalSecrets the Codesphere chart renders as plain
// Secrets when global.externalSecrets is off. The OpenFGA token is an ExternalSecret either way.
func codesphereExternalSecretNames(releaseName string) []string {
	return []string{releaseName, fmt.Sprintf("cd-%s-eab-hmac-key", releaseName)}
}

func orphanExternalSecret(ctx context.Context, kubeClient ctrlclient.Client, releaseName, name string) error {
	key := ctrlclient.ObjectKey{Namespace: codesphereNamespace, Name: name}

	externalSecret := &unstructured.Unstructured{}
	externalSecret.SetGroupVersionKind(externalSecretGVK)

	if err := kubeClient.Get(ctx, key, externalSecret); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil
		}

		return fmt.Errorf("failed to get ExternalSecret %s: %w", key, err)
	}

	if externalSecret.GetAnnotations()[helmReleaseNameAnnotation] != releaseName {
		return nil
	}

	if err := ensureHelmOwnership(ctx, kubeClient, key, releaseName); err != nil {
		return err
	}

	if err := kubeClient.Delete(ctx, externalSecret, ctrlclient.PropagationPolicy(metav1.DeletePropagationOrphan)); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ExternalSecret %s: %w", key, err)
	}

	return waitForExternalSecretDeletion(ctx, kubeClient, key)
}

// ensureHelmOwnership marks the Secret as owned by the release, so Helm can adopt it. Older
// Codesphere charts don't let ESO write this metadata.
func ensureHelmOwnership(ctx context.Context, kubeClient ctrlclient.Client, key ctrlclient.ObjectKey, releaseName string) error {
	secret := &corev1.Secret{}
	if err := kubeClient.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("failed to get Secret %s: %w", key, err)
	}

	patch := ctrlclient.MergeFrom(secret.DeepCopy())
	metav1.SetMetaDataLabel(&secret.ObjectMeta, helmManagedByLabel, helmManagedByValue)
	metav1.SetMetaDataAnnotation(&secret.ObjectMeta, helmReleaseNameAnnotation, releaseName)
	metav1.SetMetaDataAnnotation(&secret.ObjectMeta, helmReleaseNamespaceAnnotation, codesphereNamespace)

	if err := kubeClient.Patch(ctx, secret, patch); err != nil {
		return fmt.Errorf("failed to mark Secret %s as owned by Helm release %s: %w", key, releaseName, err)
	}

	return nil
}

// waitForExternalSecretDeletion waits for the garbage collector, which removes the
// ownerReferences from the Secret before it deletes the orphaned ExternalSecret.
func waitForExternalSecretDeletion(ctx context.Context, kubeClient ctrlclient.Client, key ctrlclient.ObjectKey) error {
	err := wait.PollUntilContextTimeout(ctx, externalSecretDeletionPollInterval, externalSecretDeletionTimeout, true, func(ctx context.Context) (bool, error) {
		externalSecret := &unstructured.Unstructured{}
		externalSecret.SetGroupVersionKind(externalSecretGVK)

		err := kubeClient.Get(ctx, key, externalSecret)
		if apierrors.IsNotFound(err) {
			return true, nil
		}

		if err != nil {
			return false, fmt.Errorf("failed to get ExternalSecret %s: %w", key, err)
		}

		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait for deletion of ExternalSecret %s (timeout %s): %w", key, externalSecretDeletionTimeout, err)
	}

	return nil
}
