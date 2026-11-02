// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package postgres installs and provides bootstrap access to a CloudNativePG database.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Default names and addresses for the Codesphere bootstrap database.
const (
	ReleaseName           = "cnpg"
	ClusterName           = "masterdata"
	DatabaseName          = files.MasterdataDatabaseName
	Namespace             = "codesphere"
	ServiceName           = ClusterName + "-rw"
	ServiceAddress        = ServiceName + "." + Namespace + ".svc.cluster.local"
	cnpgReadyTimeout      = 15 * time.Minute
	cnpgReadyPollInterval = 5 * time.Second
	cnpgSecretPasswordKey = "password"
)

type retryableWaitError struct{ err error }

func (e *retryableWaitError) Error() string { return e.err.Error() }
func (e *retryableWaitError) Unwrap() error { return e.err }

func isRetryableWaitError(err error) bool {
	var target *retryableWaitError
	return errors.As(err, &target)
}

// Installer manages the masterdata PostgreSQL cluster and bootstrap access.
type Installer struct {
	Context          context.Context
	KubeClient       client.Client
	Cluster          string
	ClusterNamespace string
	Config           *files.K8sManagedConfig
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

// DeployDatabase creates or updates the PostgreSQL cluster and waits for readiness.
func (b *Installer) DeployDatabase() error {
	postgresCluster := &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.clusterName(),
			Namespace: b.namespace(),
		},
	}

	_, err := controllerutil.CreateOrUpdate(b.Context, b.KubeClient, postgresCluster, func() error {
		spec, err := b.ClusterSpec()
		if err != nil {
			return err
		}

		postgresCluster.Spec = spec

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
