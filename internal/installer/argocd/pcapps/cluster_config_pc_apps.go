// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/bom"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

// ClusterConfigPcAppsValues hands cluster configuration to ArgoCD after the
// package removes the legacy TS chart. Older package BOMs retain that chart file,
// while new BOMs can still contain an image-only entry from chart dependencies.
func ClusterConfigPcAppsValues(config *files.RootConfig, vault *files.InstallVault, packageBOM *bom.Config) (files.ChartValues, error) {
	if packageBOM == nil {
		return nil, nil
	}

	if _, legacy := packageBOM.Components["cluster-config"].Files["chart"]; legacy {
		return nil, nil
	}

	if vault == nil {
		return nil, fmt.Errorf("vault is required for cluster-config registry credentials")
	}

	credentials, err := registryCredentialsValues(config, vault)
	if err != nil {
		return nil, err
	}

	additionalRegistries, err := additionalRegistriesValues(config, vault)
	if err != nil {
		return nil, err
	}

	chartValues := files.ChartValues{"registryCredentials": credentials}
	if len(additionalRegistries) > 0 {
		chartValues["additionalRegistries"] = additionalRegistries
	}

	// cluster.clusterConfig.override from the install config wins over the generated values.
	if legacy := config.Cluster.ClusterConfig; legacy != nil {
		chartValues = util.DeepMergeMaps(chartValues, legacy.Override)
	}

	return files.ChartValues{
		"applications": files.ChartValues{
			"cluster-config": files.ChartValues{
				"enabled":      true,
				"valuesObject": chartValues,
			},
		},
	}, nil
}

// registryCredentialsValues returns the credentials of the registry the installation's images are
// served from.
func registryCredentialsValues(config *files.RootConfig, vault *files.InstallVault) (files.ChartValues, error) {
	password := getSecretPassword(vault, files.SecretRegistryPassword)
	if password == "" {
		return nil, fmt.Errorf("registry password is required for cluster-config")
	}

	credentials := files.ChartValues{"password": password}
	if config.Registry != nil && config.Registry.Server != "" {
		credentials["registry"] = config.Registry.Server
	}

	if username := getSecretPassword(vault, files.SecretRegistryUsername); username != "" {
		credentials["username"] = username
	}

	return credentials, nil
}

// additionalRegistriesValues returns an enabled entry with credentials for every additional
// registry. Helm replaces lists instead of merging them, so each entry carries all values the
// chart needs to log in to the registry.
func additionalRegistriesValues(config *files.RootConfig, vault *files.InstallVault) ([]any, error) {
	registries := make([]any, 0, len(config.AdditionalRegistries))
	for _, registry := range config.AdditionalRegistries {
		username := getSecretPassword(vault, files.AdditionalRegistryUsernameSecret(registry.Name))
		if username == "" {
			return nil, fmt.Errorf("username of additional registry %s is required for cluster-config", registry.Name)
		}

		password := getSecretPassword(vault, files.AdditionalRegistryPasswordSecret(registry.Name))
		if password == "" {
			return nil, fmt.Errorf("password of additional registry %s is required for cluster-config", registry.Name)
		}

		registries = append(registries, files.ChartValues{
			"name":     registry.Name,
			"enabled":  true,
			"registry": registry.Registry,
			"username": username,
			"password": password,
		})
	}

	return registries, nil
}

// getSecretPassword returns the password field of the vault entry, or an empty string if it is not set.
func getSecretPassword(vault *files.InstallVault, name string) string {
	secret := vault.GetSecret(name)
	if secret == nil || secret.Fields == nil {
		return ""
	}

	return secret.Fields.Password
}
