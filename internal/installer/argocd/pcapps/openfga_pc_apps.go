// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package pcapps provides app specific logic to configure apps that are part of the pc-applications.
package pcapps

import (
	"log"
	"slices"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/installer/secrets"
)

// openFgaPresharedKeysSecret is the Secret the openfga chart reads the preshared key from.
// Its content is synced out of the installation vault by the chart's own ExternalSecret, so
// oms only has to name it.
const openFgaPresharedKeysSecret = "openfga-preshared-keys"

// cnpgClustersStorageClass is the Ceph RBD class with reclaim policy Retain that the PC
// installer creates with rook-external-cluster (storageClassCnpgClusters in the monorepo's
// script-lib), so the database volumes survive a deleted CNPG Cluster.
const cnpgClustersStorageClass = "codesphere-rbd-cnpg-clusters"

// OpenFgaPcAppsValues derives the pc-applications values for OpenFGA. They are the *base* of
// the pc-apps values: an explicit `pcApps` block in config.yaml and any --pc-apps-values file
// still override them. Returns nil when there is nothing to say, leaving the chart defaults
// untouched.
func OpenFgaPcAppsValues(config *files.RootConfig, vault *files.InstallVault) files.ChartValues {
	application := files.ChartValues{}
	chartValues := files.ChartValues{}

	if fga := config.Codesphere.OpenFga; fga != nil {
		application["enabled"] = fga.DeploysOpenFga()

		if fga.Expose != nil {
			chartValues["gateway"] = gatewayValues(config, fga.Expose)
		}
	}

	if authn := authnValues(config, vault); authn != nil {
		chartValues["openfga"] = files.ChartValues{"authn": authn}
	}

	if postgres := postgresValues(config); postgres != nil {
		chartValues["postgres"] = postgres
	}

	if len(chartValues) > 0 {
		application["valuesObject"] = chartValues
	}

	if len(application) == 0 {
		return nil
	}

	return files.ChartValues{
		"applications": files.ChartValues{
			"openfga": application,
		},
	}
}

// authnValues makes OpenFGA require the preshared key exactly when the installation has one,
// so it stays in step with the Codesphere services: they read the same vault entry and treat
// it as optional too. Returns nil for an installation without the key, which runs an
// unauthenticated OpenFGA. Services that started before the key existed only send it once
// their pods roll.
func authnValues(config *files.RootConfig, vault *files.InstallVault) files.ChartValues {
	if secrets.OpenFgaPresharedKeyMustBeCopied(vault, config) {
		log.Printf("OpenFGA: %s\n", secrets.CopyOpenFgaPresharedKeyHint)

		return nil
	}

	if !secrets.HasOpenFgaPresharedKey(vault) {
		log.Printf(
			"OpenFGA: %s is not in the vault, deploying OpenFGA without authentication."+
				" Add the key with `oms update install-config` — a future version will require it.\n",
			files.SecretOpenFgaPresharedKey,
		)

		return nil
	}

	return files.ChartValues{
		"method": "preshared",
		"preshared": files.ChartValues{
			"keysSecret": openFgaPresharedKeysSecret,
		},
	}
}

// postgresValues configures the OpenFGA database: its backups and, where the installation
// has it, the retaining storage class. Returns nil when there is nothing to set.
func postgresValues(config *files.RootConfig) files.ChartValues {
	postgres := files.ChartValues{}

	if backup := backupValues(config.Codesphere.OpenfgaBackups); backup != nil {
		postgres["backup"] = backup
	}

	if hasCnpgClustersStorageClass(config) {
		postgres["storage"] = files.ChartValues{"storageClass": cnpgClustersStorageClass}
	}

	if len(postgres) == 0 {
		return nil
	}

	return postgres
}

// hasCnpgClustersStorageClass mirrors when the PC installer installs rook-external-cluster,
// which creates the class: unless the component is disabled or workspaces live on PVCs.
// Elsewhere the class does not exist and the database volumes would never bind.
func hasCnpgClustersStorageClass(config *files.RootConfig) bool {
	if rook := config.Cluster.RookExternalCluster; rook != nil && !rook.Enabled {
		return false
	}

	// The installer still reads the deprecated experiments.
	pvcStorage := config.Codesphere.IsEnabled(files.WorkspacePVCStorageFlag) ||
		slices.Contains(config.Codesphere.Experiments, files.WorkspacePVCStorageFlag) //nolint:staticcheck

	return !pvcStorage
}

// backupValues configures the CNPG backups of the OpenFGA database. Schedule and retention are
// only set when configured, so the chart defaults apply otherwise. Returns nil when the config
// has no openfgaBackups block, leaving the chart default (backups off) untouched.
func backupValues(backups *files.OpenfgaBackupsConfig) files.ChartValues {
	if backups == nil {
		return nil
	}

	backup := files.ChartValues{"enabled": backups.Enabled}
	if backups.Schedule != "" {
		backup["schedule"] = backups.Schedule
	}

	if backups.DestinationPath != "" {
		backup["destinationPath"] = backups.DestinationPath
	}

	if backups.EndpointURL != "" {
		backup["endpointURL"] = backups.EndpointURL
	}

	if backups.RetentionPolicy != "" {
		backup["retentionPolicy"] = backups.RetentionPolicy
	}

	return backup
}

// gatewayValues publishes a locally deployed OpenFGA through the Codesphere gateway, so the
// Codesphere services of the other data centers can reach it.
func gatewayValues(config *files.RootConfig, expose *files.OpenFgaExposeConfig) files.ChartValues {
	gateway := files.ChartValues{"enabled": expose.Enabled}
	if expose.Host != "" {
		gateway["host"] = expose.Host
	}

	// The cert-manager ClusterIssuer the cluster step creates is named after the
	// configured issuer type, so the gateway certificate follows the same issuer as
	// the Codesphere frontend gateway.
	gateway["tls"] = files.ChartValues{
		"certificate": files.ChartValues{
			"issuerRef": files.ChartValues{"name": certIssuerName(config)},
		},
	}

	return gateway
}

// certIssuerName returns the name of the ClusterIssuer for this installation, matching the
// naming the cluster step uses (the issuer type is the issuer name).
func certIssuerName(config *files.RootConfig) string {
	if issuer := config.Codesphere.CertIssuer; issuer != nil && issuer.Type != "" {
		return string(issuer.Type)
	}

	return string(files.CertIssuerTypeSelfSigned)
}
