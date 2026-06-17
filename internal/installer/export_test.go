// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"

	corev1 "k8s.io/api/core/v1"
)

// Test helpers — exported only during `go test` so external test packages
// (package installer_test) can access unexported fields and methods.

func (o *OpenBaoInstaller) SetCtx(ctx context.Context) {
	o.ctx = ctx
}

func (o *OpenBaoInstaller) SetUnsealSecret(secret *corev1.Secret) {
	o.unsealSecret = secret
}

func (o *OpenBaoInstaller) SetPassword(password string) {
	o.password = password
}

func (o *OpenBaoInstaller) GetDRBackupExists() bool {
	return o.drBackupExists
}

func (o *OpenBaoInstaller) GetUnsealSecret() *corev1.Secret {
	return o.unsealSecret
}

func (o *OpenBaoInstaller) HasExistingDeployment() (bool, error) {
	return o.hasExistingDeployment()
}

func (o *OpenBaoInstaller) SetBackupUnsealKeys(keys map[string][]byte) {
	o.backupUnsealKeys = keys
}

func (o *OpenBaoInstaller) EnsureUnsealSecret() error {
	return o.ensureUnsealSecret(o.Clientset.CoreV1().Secrets(o.Config.Namespace))
}

func (o *OpenBaoInstaller) ReleaseExistsInTargetNamespace(releaseName string) (bool, error) {
	return o.releaseExistsInTargetNamespace(releaseName)
}

func (o *OpenBaoInstaller) OperatorInstalledClusterWide() (bool, error) {
	return o.operatorInstalledClusterWide()
}

func BuildRetryJoinAddrs(replicas int, namespace string) []string {
	return buildRetryJoinAddrs(replicas, namespace)
}
