// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// openFgaPresharedKeysSecret is the Secret the openfga chart reads the preshared key from.
// Its content is synced out of the installation vault by the chart's own ExternalSecret, so
// oms only has to name it.
const openFgaPresharedKeysSecret = "openfga-preshared-keys"

// OpenFgaPcAppsValues translates the customer-facing codesphere.openFga block of the install
// config into pc-applications values.
//
// OpenFGA is deployed by pc-applications, but whether a data center runs its own instance and
// whether that instance is published is an installation-level decision, not a chart detail — so
// operators configure it in config.yaml and this derives the chart values from it. The result is
// the *base* of the pc-apps values: an explicit `pcApps` block in config.yaml and any
// --pc-apps-values file still override it.
//
// Authentication follows the vault rather than the config: OpenFGA requires the preshared key
// exactly when the installation has one, which keeps it in step with the Codesphere services
// (they take the same key from the same vault entry, and treat it as optional). Pods that were
// started before the key existed keep running without it until the release rolls out, so an
// installation that adds the key mid-life restarts its Codesphere services.
//
// Returns nil when neither the config nor the vault says anything about OpenFGA, leaving the
// pc-applications chart defaults untouched.
func OpenFgaPcAppsValues(config *files.RootConfig, vault *files.InstallVault) files.ChartValues {
	openfga := files.ChartValues{}
	chartValues := files.ChartValues{}

	if fga := config.Codesphere.OpenFga; fga != nil {
		openfga["enabled"] = fga.DeploysOpenFga()

		if fga.Expose != nil {
			chartValues["gateway"] = gatewayValues(config, fga.Expose)
		}
	}

	if hasOpenFgaPresharedKey(vault) {
		chartValues["openfga"] = files.ChartValues{
			"authn": files.ChartValues{
				"method": "preshared",
				"preshared": files.ChartValues{
					"keysSecret": openFgaPresharedKeysSecret,
				},
			},
		}
	}

	if len(chartValues) > 0 {
		openfga["valuesObject"] = chartValues
	}

	if len(openfga) == 0 {
		return nil
	}

	return files.ChartValues{
		"applications": files.ChartValues{
			"openfga": openfga,
		},
	}
}

// gatewayValues publishes a locally deployed OpenFGA through the Codesphere gateway.
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

// hasOpenFgaPresharedKey reports whether the vault holds a usable preshared key. A vault
// written by an older oms has no entry at all; `oms update install-config` adds one.
func hasOpenFgaPresharedKey(vault *files.InstallVault) bool {
	if vault == nil {
		return false
	}

	secret := vault.GetSecret(files.SecretOpenFgaPresharedKey)

	return secret != nil && secret.Fields != nil && secret.Fields.Password != ""
}

// certIssuerName returns the name of the ClusterIssuer for this installation, matching the
// naming the cluster step uses (the issuer type is the issuer name).
func certIssuerName(config *files.RootConfig) string {
	if issuer := config.Codesphere.CertIssuer; issuer != nil && issuer.Type != "" {
		return string(issuer.Type)
	}

	return string(files.CertIssuerTypeSelfSigned)
}
