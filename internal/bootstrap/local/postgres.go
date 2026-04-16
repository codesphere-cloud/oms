// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"errors"
	"fmt"
	"time"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/codesphere-cloud/oms/internal/installer"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	cnpgRepoURL             = "https://cloudnative-pg.github.io/charts"
	cnpgReleaseName         = "cnpg"
	cnpgDatabaseClusterName = "masterdata"
	cnpgDatabaseName        = "masterdata"
	cnpgDatabaseVersion     = "15.14"
	cnpgDatabaseStorageSize = "10Gi"
	cnpgReadyTimeout        = 15 * time.Minute
	cnpgReadyPollInterval   = 5 * time.Second
	cnpgSecretPasswordKey   = "password"
)

func (b *LocalBootstrapper) InstallCloudNativePGHelmChart() error {
	if err := b.helm.UpgradeChart(b.ctx, installer.ChartConfig{
		ReleaseName:     cnpgReleaseName,
		ChartName:       "cloudnative-pg",
		RepoURL:         cnpgRepoURL,
		Namespace:       codesphereNamespace,
		CreateNamespace: true,
		Values: map[string]interface{}{
			"config": map[string]interface{}{
				"clusterWide": true,
				"data": map[string]interface{}{
					"INHERITED_LABELS": "teamId",
				},
			},
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{
					"cpu":    "0",
					"memory": "0",
				},
			},
		},
	}, installer.UpgradeChartOptions{InstallIfNotExist: true}); err != nil {
		return fmt.Errorf("failed to deploy Helm chart %q: %w", cnpgReleaseName, err)
	}

	return nil
}

func (b *LocalBootstrapper) DeployPostgresDatabase() error {
	postgresCluster := &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cnpgDatabaseClusterName,
			Namespace: codesphereNamespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, postgresCluster, func() error {
		postgresCluster.Spec = cnpgv1.ClusterSpec{
			ImageName: fmt.Sprintf("ghcr.io/cloudnative-pg/postgresql:%s-system-trixie", cnpgDatabaseVersion),
			Instances: 1,
			StorageConfiguration: cnpgv1.StorageConfiguration{
				StorageClass: ptr.To(cephStorageClassName),
				Size:         cnpgDatabaseStorageSize,
			},
			EnableSuperuserAccess: ptr.To(true),
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update PostgreSQL cluster %q: %w", cnpgDatabaseClusterName, err)
	}

	if err := b.WaitForPostgresDatabaseReady(); err != nil {
		return err
	}

	return nil
}

func (b *LocalBootstrapper) WaitForPostgresDatabaseReady() error {
	ctx, cancel := context.WithTimeout(b.ctx, cnpgReadyTimeout)
	defer cancel()

	clusterKey := client.ObjectKey{
		Name:      cnpgDatabaseClusterName,
		Namespace: codesphereNamespace,
	}

	steps := int(cnpgReadyTimeout / cnpgReadyPollInterval)
	if steps < 1 {
		steps = 1
	}

	backoff := wait.Backoff{
		Duration: cnpgReadyPollInterval,
		Factor:   1.0,
		Jitter:   0.1,
		Steps:    steps,
	}

	lastPhase := ""
	lastReadyInstances := 0
	lastInstances := 0

	err := retry.OnError(backoff, isRetryableWaitError, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}

		cluster := &cnpgv1.Cluster{}
		err := b.kubeClient.Get(ctx, clusterKey, cluster)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return &retryableWaitError{err: fmt.Errorf("PostgreSQL cluster %q not found yet", cnpgDatabaseClusterName)}
			}

			return err
		}

		lastPhase = cluster.Status.Phase
		lastReadyInstances = cluster.Status.ReadyInstances
		lastInstances = cluster.Status.Instances

		if isCNPGClusterReady(cluster) {
			return nil
		}

		return &retryableWaitError{err: fmt.Errorf(
			"PostgreSQL cluster is not ready yet (phase=%q, readyInstances=%d, instances=%d)",
			lastPhase,
			lastReadyInstances,
			lastInstances,
		)}
	})
	if err == nil {
		return nil
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || isRetryableWaitError(err) {
		return fmt.Errorf(
			"timed out waiting for PostgreSQL cluster %q to become ready (phase=%q, readyInstances=%d, instances=%d, error=%v)",
			cnpgDatabaseClusterName,
			lastPhase,
			lastReadyInstances,
			lastInstances,
			err,
		)
	}

	return fmt.Errorf("failed to fetch PostgreSQL cluster %q: %w", cnpgDatabaseClusterName, err)
}

func (b *LocalBootstrapper) ReadPostgresSuperuserPassword() (string, error) {
	clusterKey := client.ObjectKey{
		Name:      cnpgDatabaseClusterName,
		Namespace: codesphereNamespace,
	}

	cluster := &cnpgv1.Cluster{}
	if err := b.kubeClient.Get(b.ctx, clusterKey, cluster); err != nil {
		return "", fmt.Errorf("failed to get PostgreSQL cluster %q: %w", cnpgDatabaseClusterName, err)
	}

	secretName := cluster.GetSuperuserSecretName()
	secretKey := client.ObjectKey{
		Name:      secretName,
		Namespace: codesphereNamespace,
	}

	secret := &corev1.Secret{}
	if err := b.kubeClient.Get(b.ctx, secretKey, secret); err != nil {
		return "", fmt.Errorf("failed to get PostgreSQL superuser secret %q: %w", secretName, err)
	}

	passwordBytes, ok := secret.Data[cnpgSecretPasswordKey]
	if !ok {
		return "", fmt.Errorf("PostgreSQL superuser secret %q does not contain key %q", secretName, cnpgSecretPasswordKey)
	}
	if len(passwordBytes) == 0 {
		return "", fmt.Errorf("PostgreSQL superuser secret %q contains an empty %q value", secretName, cnpgSecretPasswordKey)
	}

	return string(passwordBytes), nil
}

func (b *LocalBootstrapper) ReadPostgresCA() (string, error) {
	secretName := cnpgDatabaseClusterName + "-ca"
	secretKey := client.ObjectKey{
		Name:      secretName,
		Namespace: codesphereNamespace,
	}

	secret := &corev1.Secret{}
	if err := b.kubeClient.Get(b.ctx, secretKey, secret); err != nil {
		return "", fmt.Errorf("failed to get PostgreSQL CA secret %q: %w", secretName, err)
	}

	caCert, ok := secret.Data["ca.crt"]
	if !ok {
		return "", fmt.Errorf("PostgreSQL CA secret %q does not contain key %q", secretName, "ca.crt")
	}
	if len(caCert) == 0 {
		return "", fmt.Errorf("PostgreSQL CA secret %q contains an empty %q value", secretName, "ca.crt")
	}

	return string(caCert), nil
}

func isCNPGClusterReady(cluster *cnpgv1.Cluster) bool {
	if cluster == nil {
		return false
	}

	readyCondition := apimeta.FindStatusCondition(cluster.Status.Conditions, string(cnpgv1.ConditionClusterReady))
	if readyCondition != nil && readyCondition.Status == metav1.ConditionTrue {
		return true
	}

	return cluster.Status.Instances > 0 && cluster.Status.ReadyInstances == cluster.Status.Instances
}
