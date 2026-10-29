// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path"
	"time"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/distribution/reference"
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
	cnpgReleaseName         = "cnpg"
	cnpgDatabaseClusterName = "masterdata"
	cnpgDatabaseName        = "masterdata"
	cnpgDatabaseVersion     = "15.14"
	cnpgDatabaseStorageSize = "10Gi"
	cnpgReadyTimeout        = 15 * time.Minute
	cnpgReadyPollInterval   = 5 * time.Second
	cnpgSecretPasswordKey   = "password"
	cnpgBOMComponent        = "postgres-operator"
	// cnpgOperatorImageKey is the operator image's key in the BOM component, as the PC
	// installer reads it.
	cnpgOperatorImageKey = "cloudnativePg"
	// cnpgPullSecretName holds the mirror credentials the operator and PostgreSQL pods pull
	// with. It is kept apart from the installer's docker-regcred, which does not exist yet.
	cnpgPullSecretName = "oms-registry-credentials"
	// cnpgDatabaseImage is the PostgreSQL image of the masterdata cluster. The BOM does not list
	// it, so a mirror has to be filled with it separately from 'oms copy package'.
	cnpgDatabaseImage = "ghcr.io/cloudnative-pg/postgresql:" + cnpgDatabaseVersion + "-system-trixie"
)

func (b *LocalBootstrapper) InstallCloudNativePGHelmChart() error {
	if b.installerBOM == nil {
		return fmt.Errorf("installer BOM is not prepared")
	}

	chart, ok := b.installerBOM.GetChart(cnpgBOMComponent)
	if !ok {
		return fmt.Errorf("CloudNativePG chart is missing from BOM component %q", cnpgBOMComponent)
	}

	repoURL, err := b.Env.chartsRegistryURL()
	if err != nil {
		return err
	}

	values := map[string]interface{}{
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
	}

	imageValues, err := b.cnpgOperatorImageValues()
	if err != nil {
		return err
	}

	maps.Copy(values, imageValues)

	// The BOM component is Codesphere's wrapper chart around the upstream
	// cloudnative-pg chart. Its tag is the wrapper's own version, so it must be
	// installed using the chart name and version from the BOM.
	return b.installPrerequisiteChart(prerequisiteChartConfig{
		Name: cnpgReleaseName, Chart: path.Base(chart.Name()), RepoURL: repoURL,
		TargetRevision: chart.Tag(), Namespace: codesphereNamespace,
		Values: map[string]interface{}{
			// Values for the upstream chart are nested below its dependency name.
			"cloudnative-pg": values,
		},
	})
}

// cnpgOperatorImageValues returns the upstream chart values that pull the operator image the
// BOM names, from the mirror when one is configured. Without a mirror an older BOM that lacks
// the image keeps the chart's default. With a mirror the default would still be pulled from
// ghcr.io, so the image is required.
func (b *LocalBootstrapper) cnpgOperatorImageValues() (map[string]interface{}, error) {
	mirrored, err := b.Env.mirrorsImages()
	if err != nil {
		return nil, err
	}

	bomImage := b.installerBOM.Components[cnpgBOMComponent].ContainerImages[cnpgOperatorImageKey]
	if bomImage == "" {
		if mirrored {
			return nil, fmt.Errorf("CloudNativePG operator image %q is missing from BOM component %q, so it cannot be pulled from the configured registry",
				cnpgOperatorImageKey, cnpgBOMComponent)
		}

		return map[string]interface{}{}, nil
	}

	image, err := b.Env.resolveImage(bomImage)
	if err != nil {
		return nil, err
	}

	repository, tag, err := splitChartImage(image)
	if err != nil {
		return nil, fmt.Errorf("invalid CloudNativePG operator image: %w", err)
	}

	values := map[string]interface{}{
		"image": map[string]interface{}{
			"repository": repository,
			"tag":        tag,
		},
	}
	if mirrored {
		values["imagePullSecrets"] = []interface{}{map[string]interface{}{"name": cnpgPullSecretName}}
	}

	return values, nil
}

// splitChartImage splits an image reference into the repository and tag the CloudNativePG chart
// joins with a colon. A digest stays attached to the tag, so the image keeps resolving to it.
func splitChartImage(image string) (string, string, error) {
	parsed, err := reference.ParseNamed(image)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse image %q: %w", image, err)
	}

	tagged, ok := parsed.(reference.Tagged)
	if !ok {
		return "", "", fmt.Errorf("image %q is missing a tag", image)
	}

	tag := tagged.Tag()
	if digested, ok := parsed.(reference.Digested); ok {
		tag += "@" + digested.Digest().String()
	}

	return parsed.Name(), tag, nil
}

// EnsureRegistryPullSecret stores the mirror credentials as a pull secret for the CloudNativePG
// operator and the PostgreSQL pods. Images pulled from ghcr.io are public, so without a mirror
// there is nothing to do.
func (b *LocalBootstrapper) EnsureRegistryPullSecret() error {
	mirrored, err := b.Env.mirrorsImages()
	if err != nil {
		return err
	}

	if !mirrored {
		return nil
	}

	registry, err := b.Env.registryURL()
	if err != nil {
		return err
	}

	dockerConfig, err := installer.DockerConfigJSON(installer.RegistryHost(registry), b.Env.RegistryUser, b.Env.RegistryPassword)
	if err != nil {
		return fmt.Errorf("failed to encode registry credentials: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cnpgPullSecretName,
			Namespace: codesphereNamespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, secret, func() error {
		secret.Type = corev1.SecretTypeDockerConfigJson
		secret.Data = map[string][]byte{corev1.DockerConfigJsonKey: dockerConfig}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update registry pull secret %q: %w", cnpgPullSecretName, err)
	}

	return nil
}

// postgresClusterSpec returns the masterdata cluster's spec. With a mirror its PostgreSQL image
// is pulled from there with the mirror credentials.
func (b *LocalBootstrapper) postgresClusterSpec() (cnpgv1.ClusterSpec, error) {
	mirrored, err := b.Env.mirrorsImages()
	if err != nil {
		return cnpgv1.ClusterSpec{}, err
	}

	image, err := b.Env.resolveImage(cnpgDatabaseImage)
	if err != nil {
		return cnpgv1.ClusterSpec{}, err
	}

	spec := cnpgv1.ClusterSpec{
		ImageName: image,
		Instances: 1,
		StorageConfiguration: cnpgv1.StorageConfiguration{
			StorageClass: ptr.To(b.storageClassName()),
			Size:         cnpgDatabaseStorageSize,
		},
		EnableSuperuserAccess: ptr.To(true),
	}

	if mirrored {
		b.stlog.Logf("Pulling PostgreSQL from %s. It is not part of the installer package, so copy it there first, e.g. crane copy %s %s",
			image, cnpgDatabaseImage, image)

		spec.ImagePullSecrets = []cnpgv1.LocalObjectReference{{Name: cnpgPullSecretName}}
	}

	return spec, nil
}

func (b *LocalBootstrapper) DeployPostgresDatabase() error {
	spec, err := b.postgresClusterSpec()
	if err != nil {
		return err
	}

	postgresCluster := &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cnpgDatabaseClusterName,
			Namespace: codesphereNamespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(b.ctx, b.kubeClient, postgresCluster, func() error {
		postgresCluster.Spec = spec
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
