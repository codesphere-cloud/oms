// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Test helpers — exported only during `go test` so external test packages
// (package installer_test) can access unexported fields and methods.

func (o *OpenBaoBootstrapper) SetCtx(ctx context.Context) {
	o.ctx = ctx
}

func (o *OpenBaoBootstrapper) SetUnsealSecret(secret *corev1.Secret) {
	o.unsealSecret = secret
}

func (o *OpenBaoBootstrapper) SetReplicas(replicas int) {
	o.replicas = replicas
}

func (o *OpenBaoBootstrapper) GetReplicas() int {
	return o.replicas
}

func (o *OpenBaoBootstrapper) GetUnsealSecret() *corev1.Secret {
	return o.unsealSecret
}

func (o *OpenBaoBootstrapper) GetBackupUnsealKeys() map[string][]byte {
	return o.backupUnsealKeys
}

func (o *OpenBaoBootstrapper) SetBackupUnsealKeys(keys map[string][]byte) {
	o.backupUnsealKeys = keys
}

func (o *OpenBaoBootstrapper) EnsureUnsealSecret() error {
	return o.ensureUnsealSecret(o.Clientset.CoreV1().Secrets(o.Config.Namespace))
}

func (o *OpenBaoBootstrapper) ValidateConfig() error {
	return o.validateConfig()
}

func (o *OpenBaoBootstrapper) ReadinessTimeout() time.Duration {
	return o.readinessTimeout()
}

func (o *OpenBaoBootstrapper) CheckNoForeignUnsealKeys() error {
	return o.checkNoForeignUnsealKeys(o.Clientset.CoreV1().Secrets(o.Config.Namespace))
}
