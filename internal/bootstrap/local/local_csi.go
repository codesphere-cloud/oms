// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"fmt"
	"strings"

	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	localCSIReleaseName = "local-csi-provisioner"
	localCSINamespace   = "local-csi"
	localCSIRepoURL     = "oci://ghcr.io/codesphere-cloud/charts"
	localCSIVersion     = "0.2.0"
)

func (b *LocalBootstrapper) InstallLocalCSIProvisioner() error {
	if err := b.installPrerequisiteChart(prerequisiteChartConfig{
		Name:           localCSIReleaseName,
		Chart:          localCSIReleaseName,
		RepoURL:        localCSIRepoURL,
		TargetRevision: localCSIVersion,
		Namespace:      localCSINamespace,
		Values: map[string]interface{}{
			"storageClass": map[string]interface{}{
				"name":      localStorageClassName,
				"isDefault": true,
			},
		},
	}); err != nil {
		return err
	}
	return b.setLocalStorageClassDefault()
}

func (b *LocalBootstrapper) setLocalStorageClassDefault() error {
	key := client.ObjectKey{Name: localStorageClassName}
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		storageClass := &storagev1.StorageClass{}
		if err := b.kubeClient.Get(b.ctx, key, storageClass); err != nil {
			return err
		}
		before := storageClass.DeepCopy()
		if storageClass.Annotations == nil {
			storageClass.Annotations = map[string]string{}
		}
		storageClass.Annotations[defaultStorageClassAnnotation] = "true"
		delete(storageClass.Annotations, legacyDefaultClassAnnotation)
		return b.kubeClient.Patch(b.ctx, storageClass, client.MergeFrom(before))
	}); err != nil {
		return fmt.Errorf("failed to make StorageClass %q the default: %w", localStorageClassName, err)
	}

	classes := &storagev1.StorageClassList{}
	if err := b.kubeClient.List(b.ctx, classes); err != nil {
		return fmt.Errorf("failed to list StorageClasses: %w", err)
	}
	for _, class := range classes.Items {
		if class.Name == localStorageClassName || !isDefaultStorageClass(class.Annotations) {
			continue
		}
		key := client.ObjectKey{Name: class.Name}
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			storageClass := &storagev1.StorageClass{}
			if err := b.kubeClient.Get(b.ctx, key, storageClass); err != nil {
				return err
			}
			if !isDefaultStorageClass(storageClass.Annotations) {
				return nil
			}
			before := storageClass.DeepCopy()
			delete(storageClass.Annotations, defaultStorageClassAnnotation)
			delete(storageClass.Annotations, legacyDefaultClassAnnotation)
			return b.kubeClient.Patch(b.ctx, storageClass, client.MergeFrom(before))
		}); err != nil {
			return fmt.Errorf("failed to remove default annotation from StorageClass %q: %w", class.Name, err)
		}
	}
	return nil
}

func isDefaultStorageClass(annotations map[string]string) bool {
	return strings.EqualFold(annotations[defaultStorageClassAnnotation], "true") ||
		strings.EqualFold(annotations[legacyDefaultClassAnnotation], "true")
}
