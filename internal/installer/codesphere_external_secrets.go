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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	codesphereReleaseName = "codesphere"

	helmManagedByLabel             = "app.kubernetes.io/managed-by"
	helmManagedByValue             = "Helm"
	helmReleaseNameAnnotation      = "meta.helm.sh/release-name"
	helmReleaseNamespaceAnnotation = "meta.helm.sh/release-namespace"

	esoManagedLabel = "reconcile.external-secrets.io/managed"

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

	return orphanCodesphereExternalSecrets(ctx, kubeClient, cfg.Codesphere.ReleaseName)
}

// orphanCodesphereExternalSecrets deletes the release's ExternalSecrets without their Secrets.
// A target release that renders them as plain Secrets (e.g. a rollback) then adopts the Secrets
// instead of failing, because ESO's ownerReference would otherwise garbage-collect them once
// Helm deletes the ExternalSecret. A target release that renders the ExternalSecrets again
// recreates them, and ESO takes the existing Secrets back.
//
// Releases with their own name (master-dev, previews) are skipped: they set externalSecrets
// explicitly, and the names below belong to the default release, which may share the cluster.
//
// TODO(nate): remove once ESO is persisted, i.e. no release without external secrets is left to roll back to.
func orphanCodesphereExternalSecrets(ctx context.Context, kubeClient ctrlclient.Client, releaseName string) error {
	if releaseName != "" && releaseName != codesphereReleaseName {
		return nil
	}

	for _, name := range codesphereExternalSecretNames {
		if err := orphanExternalSecret(ctx, kubeClient, name); err != nil {
			return err
		}
	}

	return nil
}

// codesphereExternalSecretNames lists the ExternalSecrets the Codesphere chart renders as plain
// Secrets when global.externalSecrets is off. The OpenFGA token is an ExternalSecret either way.
var codesphereExternalSecretNames = []string{codesphereReleaseName, "cd-" + codesphereReleaseName + "-eab-hmac-key"}

func orphanExternalSecret(ctx context.Context, kubeClient ctrlclient.Client, name string) error {
	key := ctrlclient.ObjectKey{Namespace: codesphereNamespace, Name: name}

	externalSecret := &unstructured.Unstructured{}
	externalSecret.SetGroupVersionKind(externalSecretGVK)
	externalSecret.SetNamespace(key.Namespace)
	externalSecret.SetName(key.Name)

	err := kubeClient.Delete(ctx, externalSecret, ctrlclient.PropagationPolicy(metav1.DeletePropagationOrphan))
	switch {
	case err == nil:
		if err := waitForExternalSecretDeletion(ctx, kubeClient, key); err != nil {
			return err
		}
	case !apierrors.IsNotFound(err):
		return fmt.Errorf("failed to delete ExternalSecret %s: %w", key, err)
	}

	return ensureHelmOwnership(ctx, kubeClient, key)
}

// ensureHelmOwnership marks a Secret ESO wrote as owned by the release, so Helm can adopt it.
// It runs after the ExternalSecret is gone, so ESO can't sync the metadata away, and also when
// it was already gone, so a retry finishes a handover whose patch failed.
func ensureHelmOwnership(ctx context.Context, kubeClient ctrlclient.Client, key ctrlclient.ObjectKey) error {
	secret := &corev1.Secret{}
	if err := kubeClient.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("failed to get Secret %s: %w", key, err)
	}

	if secret.Labels[esoManagedLabel] != "true" {
		return nil
	}

	patch := ctrlclient.MergeFrom(secret.DeepCopy())
	metav1.SetMetaDataLabel(&secret.ObjectMeta, helmManagedByLabel, helmManagedByValue)
	metav1.SetMetaDataAnnotation(&secret.ObjectMeta, helmReleaseNameAnnotation, codesphereReleaseName)
	metav1.SetMetaDataAnnotation(&secret.ObjectMeta, helmReleaseNamespaceAnnotation, codesphereNamespace)

	if err := kubeClient.Patch(ctx, secret, patch); err != nil {
		return fmt.Errorf("failed to mark Secret %s as owned by Helm release %s: %w", key, codesphereReleaseName, err)
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
