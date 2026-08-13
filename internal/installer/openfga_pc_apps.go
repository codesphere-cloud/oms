// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"log"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// openFgaPresharedKeysSecret is the Secret the openfga chart reads the preshared key from.
// Its content is synced out of the installation vault by the chart's own ExternalSecret, so
// oms only has to name it.
const openFgaPresharedKeysSecret = "openfga-preshared-keys"

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

	if authn := authnValues(vault); authn != nil {
		chartValues["openfga"] = files.ChartValues{"authn": authn}
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
func authnValues(vault *files.InstallVault) files.ChartValues {
	if !hasOpenFgaPresharedKey(vault) {
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
