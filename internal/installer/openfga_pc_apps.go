// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// OpenFgaPcAppsValues translates the customer-facing codesphere.openFga block of the install
// config into pc-applications values.
//
// OpenFGA is deployed by pc-applications, but whether a data center runs its own instance and
// whether that instance is published is an installation-level decision, not a chart detail — so
// operators configure it in config.yaml and this derives the chart values from it. The result is
// the *base* of the pc-apps values: an explicit `pcApps` block in config.yaml and any
// --pc-apps-values file still override it.
//
// Returns nil when the config says nothing about OpenFGA, leaving the pc-applications chart
// defaults untouched.
func OpenFgaPcAppsValues(config *files.RootConfig) files.ChartValues {
	fga := config.Codesphere.OpenFga
	if fga == nil {
		return nil
	}

	openfga := files.ChartValues{"enabled": fga.DeploysOpenFga()}

	if fga.Expose != nil {
		gateway := files.ChartValues{"enabled": fga.Expose.Enabled}
		if fga.Expose.Host != "" {
			gateway["host"] = fga.Expose.Host
		}
		// The cert-manager ClusterIssuer the cluster step creates is named after the
		// configured issuer type, so the gateway certificate follows the same issuer as
		// the Codesphere frontend gateway.
		gateway["tls"] = files.ChartValues{
			"certificate": files.ChartValues{
				"issuerRef": files.ChartValues{"name": certIssuerName(config)},
			},
		}
		openfga["valuesObject"] = files.ChartValues{"gateway": gateway}
	}

	return files.ChartValues{
		"applications": files.ChartValues{
			"openfga": openfga,
		},
	}
}

// certIssuerName returns the name of the ClusterIssuer for this installation, matching the
// naming the cluster step uses (the issuer type is the issuer name).
func certIssuerName(config *files.RootConfig) string {
	if issuer := config.Codesphere.CertIssuer; issuer != nil && issuer.Type != "" {
		return string(issuer.Type)
	}
	return string(files.CertIssuerTypeSelfSigned)
}
