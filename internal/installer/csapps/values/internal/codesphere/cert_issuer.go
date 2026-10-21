// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/images"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const (
	selfSignedIssuerName       = "self-signed"
	acmeEABMacKeyName          = "acmeEabMacKey"
	acmeCustomDomainsEABMacKey = "acmeCustomDomainsEabMacKey"
)

func certIssuerValues(issuer *files.CertIssuerConfig) map[string]any {
	acme := map[string]any{"enabled": false}
	if issuer == nil || issuer.Type != files.CertIssuerTypeACME || issuer.Acme == nil {
		return map[string]any{"acme": acme}
	}

	acme["enabled"] = true
	acme["server"] = issuer.Acme.Server

	acme["email"] = issuer.Acme.Email
	if keyID, macKeyName := customDomainsEAB(issuer.Acme); keyID != "" {
		acme["eabKeyId"] = keyID
		acme["eabMacKeyName"] = macKeyName
	}

	return map[string]any{"acme": acme}
}

// customDomainsEAB returns the EAB key the custom-domain issuer registers with and the vault entry
// of its MAC key. It falls back to the cluster issuer's key for configs without a separate one.
func customDomainsEAB(acme *files.ACMEConfig) (keyID, macKeyName string) {
	if acme.CustomDomainsEABKeyID != "" {
		return acme.CustomDomainsEABKeyID, acmeCustomDomainsEABMacKey
	}

	if acme.EABKeyID != "" {
		return acme.EABKeyID, acmeEABMacKeyName
	}

	return "", ""
}

func frontendGatewayValues(
	imgs images.Images,
	issuer *files.CertIssuerConfig,
	gatewayAPIEnabled bool,
) (map[string]any, error) {
	nginx, err := imgs.Ref("frontendGatewayNginx")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the frontend gateway image: %w", err)
	}

	issuerName := selfSignedIssuerName
	if issuer != nil && issuer.Type != "" {
		issuerName = string(issuer.Type)
	}

	return map[string]any{
		"issuer":        issuerName,
		"redirectToIde": true,
		"image":         map[string]any{"name": nginx.Image, "tag": nginx.Tag},
		"gatewayApi":    map[string]any{"enabled": gatewayAPIEnabled},
	}, nil
}
