// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/clusteradmin"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/vault"
	"github.com/codesphere-cloud/oms/internal/util"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ResolveVaultPath returns the explicit vault path or falls back to
// prod.vault.yaml in the config's secrets baseDir.
func ResolveVaultPath(vaultPath string, config files.RootConfig) (string, error) {
	if strings.TrimSpace(vaultPath) != "" {
		return vaultPath, nil
	}

	if strings.TrimSpace(config.Secrets.BaseDir) == "" {
		return "", fmt.Errorf("vault path is not set and config.yaml secrets.baseDir is empty")
	}

	return filepath.Join(config.Secrets.BaseDir, "prod.vault.yaml"), nil
}

// VaultAndRESTConfig loads the selected vault implementation and builds
// a Kubernetes REST config from its kubeconfig secret.
func VaultAndRESTConfig(vaultPath, privKey, vaultType string, cfg files.RootConfig) (*files.InstallVault, *rest.Config, error) {
	resolvedPath, err := ResolveVaultPath(vaultPath, cfg)
	if err != nil {
		return nil, nil, err
	}

	store, err := vault.NewFromString(vaultType, vault.Options{Path: resolvedPath, AgeKey: privKey})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize vault backend: %w", err)
	}

	data, err := store.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load vault %s: %w", resolvedPath, err)
	}

	kubeConfigContent, err := kubeConfigContentFromVault(data)
	if err != nil {
		return nil, nil, err
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeConfigContent))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load kubernetes config from vault: %w", err)
	}

	return data, restConfig, nil
}

func kubeConfigContentFromVault(vault *files.InstallVault) (string, error) {
	if vault == nil {
		return "", fmt.Errorf("vault is not loaded")
	}

	kubeConfig := vault.GetSecret(files.SecretKubeConfig)
	if kubeConfig == nil || kubeConfig.File == nil || strings.TrimSpace(kubeConfig.File.Content) == "" {
		return "", fmt.Errorf("kubeconfig not found in vault (secret %q)", files.SecretKubeConfig)
	}

	return kubeConfig.File.Content, nil
}

// EnsureClusterAdminSecret applies the cluster admin email configured via
// codesphere.clusterAdminEmail to the cluster-admin-email secret before the
// platform is installed, so the auth-service finds it on first start.
// It is a no-op when the config does not set an email.
func EnsureClusterAdminSecret(ctx context.Context, vaultPath, privKey, vaultType string, cfg files.RootConfig) error {
	email := cfg.Codesphere.ClusterAdminEmail
	if email == "" {
		return nil
	}

	_, restConfig, err := VaultAndRESTConfig(vaultPath, privKey, vaultType, cfg)
	if err != nil {
		return err
	}
	clientset, _, err := util.NewClientsFromRESTConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return clusteradmin.AddClusterAdmin(ctx, clientset, clusteradmin.Opts{
		Email:           email,
		Namespace:       clusteradmin.DefaultNamespace,
		SecretName:      clusteradmin.DefaultSecretName,
		CreateNamespace: true,
	})
}
