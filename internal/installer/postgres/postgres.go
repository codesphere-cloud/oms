// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package postgres installs and provides bootstrap access to a CloudNativePG database.
package postgres

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/bom"
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

// Default names and addresses for the Codesphere bootstrap database.
const (
	ReleaseName             = "cnpg"
	ClusterName             = "masterdata"
	DatabaseName            = "masterdata"
	Namespace               = "codesphere"
	ServiceName             = ClusterName + "-rw"
	ServiceAddress          = ServiceName + "." + Namespace + ".svc.cluster.local"
	cnpgDatabaseVersion     = "15.14"
	cnpgDatabaseStorageSize = "10Gi"
	cnpgReadyTimeout        = 15 * time.Minute
	cnpgReadyPollInterval   = 5 * time.Second
	cnpgSecretPasswordKey   = "password"
	cnpgBOMComponent        = "postgres-operator"
)

type retryableWaitError struct{ err error }

func (e *retryableWaitError) Error() string { return e.err.Error() }
func (e *retryableWaitError) Unwrap() error { return e.err }

func isRetryableWaitError(err error) bool {
	var target *retryableWaitError
	return errors.As(err, &target)
}

// Installer owns the CNPG operator and database used by a bootstrap installation.
type Installer struct {
	Context          context.Context
	KubeClient       client.Client
	HelmClient       installer.HelmClient
	BOM              *bom.Config
	Cluster          string
	ClusterNamespace string
	RegistryURL      string
	RegistryUser     string
	RegistryPassword string
	StorageClass     string
	Version          string
	StorageSize      string
	OperatorValues   map[string]interface{}
}

func (b *Installer) clusterName() string {
	if b.Cluster != "" {
		return b.Cluster
	}

	return ClusterName
}

func (b *Installer) namespace() string {
	if b.ClusterNamespace != "" {
		return b.ClusterNamespace
	}

	return Namespace
}

func (b *Installer) serviceName() string { return b.clusterName() + "-rw" }

// InstallOperator installs or updates the CloudNativePG operator chart.
func (b *Installer) InstallOperator() error {
	if b.BOM == nil {
		return fmt.Errorf("installer BOM is not prepared")
	}

	chart, ok := b.BOM.GetChart(cnpgBOMComponent)
	if !ok {
		return fmt.Errorf("CloudNativePG chart is missing from BOM component %q", cnpgBOMComponent)
	}

	repoURL := b.RegistryURL

	// The BOM component is Codesphere's wrapper chart around the upstream
	// cloudnative-pg chart. Its tag is the wrapper's own version, so it must be
	// installed using the chart name and version from the BOM.
	values := b.OperatorValues
	if values == nil {
		values = map[string]interface{}{
			// Values for the upstream chart are nested below its dependency name.
			"cloudnative-pg": map[string]interface{}{
				// Reset the wrapper's production defaults that don't exist on a local cluster.
				"priorityClassName": "",
				"monitoring": map[string]interface{}{
					"podMonitorEnabled": false,
				},
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
		}
	}

	return b.installChart(path.Base(chart.Name()), repoURL, chart.Tag(), values)
}

func (b *Installer) installChart(chartName, repoURL, version string, values map[string]interface{}) error {
	if b.HelmClient == nil {
		return fmt.Errorf("helm client is not set")
	}

	if !strings.HasPrefix(repoURL, "https://") && !strings.HasPrefix(repoURL, "http://") {
		repoURL = strings.TrimPrefix(repoURL, "oci://")
		if err := b.HelmClient.LoginRegistry(b.Context, installer.RegistryHost(repoURL), b.RegistryUser, b.RegistryPassword); err != nil {
			return fmt.Errorf("failed to log in to chart registry: %w", err)
		}

		chartName = "oci://" + strings.TrimSuffix(repoURL, "/") + "/" + chartName
		repoURL = ""
	}

	config := installer.ChartConfig{ReleaseName: ReleaseName, ChartName: chartName, RepoURL: repoURL, Namespace: b.namespace(), Version: version, Values: values, CreateNamespace: true}

	release, err := b.HelmClient.FindRelease(b.namespace(), ReleaseName)
	if err != nil {
		return fmt.Errorf("failed to find Helm release %q: %w", ReleaseName, err)
	}

	if release == nil {
		if err := b.HelmClient.InstallChart(b.Context, config, installer.InstallChartOptions{}); err != nil {
			return fmt.Errorf("failed to install Helm chart %q: %w", ReleaseName, err)
		}

		return nil
	}

	if err := b.HelmClient.UpgradeChart(b.Context, config, installer.UpgradeChartOptions{}); err != nil {
		return fmt.Errorf("failed to upgrade Helm chart %q: %w", ReleaseName, err)
	}

	return nil
}

// DeployDatabase creates or updates the PostgreSQL cluster and waits for readiness.
func (b *Installer) DeployDatabase() error {
	postgresCluster := &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.clusterName(),
			Namespace: b.namespace(),
		},
	}

	_, err := controllerutil.CreateOrUpdate(b.Context, b.KubeClient, postgresCluster, func() error {
		postgresCluster.Spec = cnpgv1.ClusterSpec{
			ImageName: fmt.Sprintf("ghcr.io/cloudnative-pg/postgresql:%s-system-trixie", cmp.Or(b.Version, cnpgDatabaseVersion)),
			Instances: 1,
			StorageConfiguration: cnpgv1.StorageConfiguration{
				StorageClass: ptr.To(b.StorageClass),
				Size:         cmp.Or(b.StorageSize, cnpgDatabaseStorageSize),
			},
			EnableSuperuserAccess: ptr.To(true),
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update PostgreSQL cluster %q: %w", b.clusterName(), err)
	}

	if err := b.WaitForDatabaseReady(); err != nil {
		return err
	}

	return nil
}

// WaitForDatabaseReady waits until CNPG reports a ready database instance.
func (b *Installer) WaitForDatabaseReady() error {
	ctx, cancel := context.WithTimeout(b.Context, cnpgReadyTimeout)
	defer cancel()

	clusterKey := client.ObjectKey{
		Name:      b.clusterName(),
		Namespace: b.namespace(),
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
			return fmt.Errorf("waiting for PostgreSQL cluster: %w", err)
		}

		cluster := &cnpgv1.Cluster{}

		err := b.KubeClient.Get(ctx, clusterKey, cluster)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return &retryableWaitError{err: fmt.Errorf("PostgreSQL cluster %q not found yet", b.clusterName())}
			}

			return fmt.Errorf("get PostgreSQL cluster: %w", err)
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
			b.clusterName(),
			lastPhase,
			lastReadyInstances,
			lastInstances,
			err,
		)
	}

	return fmt.Errorf("failed to fetch PostgreSQL cluster %q: %w", b.clusterName(), err)
}

// ReadSuperuserPassword reads the CNPG generated superuser password.
func (b *Installer) ReadSuperuserPassword() (string, error) {
	clusterKey := client.ObjectKey{
		Name:      b.clusterName(),
		Namespace: b.namespace(),
	}

	cluster := &cnpgv1.Cluster{}
	if err := b.KubeClient.Get(b.Context, clusterKey, cluster); err != nil {
		return "", fmt.Errorf("failed to get PostgreSQL cluster %q: %w", b.clusterName(), err)
	}

	secretName := cluster.GetSuperuserSecretName()
	secretKey := client.ObjectKey{
		Name:      secretName,
		Namespace: b.namespace(),
	}

	secret := &corev1.Secret{}
	if err := b.KubeClient.Get(b.Context, secretKey, secret); err != nil {
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

// ReadCA reads the CNPG generated CA certificate.
func (b *Installer) ReadCA() (string, error) {
	secretName := b.clusterName() + "-ca"
	secretKey := client.ObjectKey{
		Name:      secretName,
		Namespace: b.namespace(),
	}

	secret := &corev1.Secret{}
	if err := b.KubeClient.Get(b.Context, secretKey, secret); err != nil {
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
