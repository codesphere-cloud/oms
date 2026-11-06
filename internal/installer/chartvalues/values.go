// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package chartvalues builds common Kubernetes values shared by installer charts.
package chartvalues

// RegistryPullSecretName is the registry credential secret used by installer workloads.
const RegistryPullSecretName = "docker-regcred"

// RegistryPullSecrets returns the registry credential references for Kubernetes workloads.
func RegistryPullSecrets() []any {
	return []any{map[string]any{"name": RegistryPullSecretName}}
}

// RegistryPullSecretNames returns secret names for charts that expect strings.
func RegistryPullSecretNames() []string {
	return []string{RegistryPullSecretName}
}

// EnvVar builds a literal Kubernetes environment variable.
func EnvVar(name, value string) map[string]any {
	return map[string]any{"name": name, "value": value}
}
