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

// CertIssuer is the issuer of Codesphere's certificates.
type CertIssuer struct {
	ACME ACME `yaml:"acme"`
}

// ACME is the ACME issuer; disabled, it has no account.
type ACME struct {
	Enabled      bool `yaml:"enabled"`
	*ACMEAccount `yaml:",inline"`
}

// ACMEAccount is the account the ACME issuer registers with.
type ACMEAccount struct {
	Server        string `yaml:"server"`
	Email         string `yaml:"email"`
	EABKeyID      string `yaml:"eabKeyId,omitempty"`
	EABMacKeyName string `yaml:"eabMacKeyName,omitempty"`
}

// FrontendGateway is the gateway that serves Codesphere itself.
type FrontendGateway struct {
	Issuer        string     `yaml:"issuer"`
	RedirectToIde bool       `yaml:"redirectToIde"`
	Image         Image      `yaml:"image"`
	GatewayAPI    GatewayAPI `yaml:"gatewayApi"`
}

// Image is an image reference split into name and tag.
type Image struct {
	Name string `yaml:"name"`
	Tag  string `yaml:"tag"`
}

// GatewayAPI says whether a gateway is a Gateway API Gateway.
type GatewayAPI struct {
	Enabled bool `yaml:"enabled"`
}

func certIssuerValues(issuer *files.CertIssuerConfig) CertIssuer {
	if issuer == nil || issuer.Type != files.CertIssuerTypeACME || issuer.Acme == nil {
		return CertIssuer{ACME: ACME{Enabled: false}}
	}

	keyID, macKeyName := customDomainsEAB(issuer.Acme)

	return CertIssuer{ACME: ACME{Enabled: true, ACMEAccount: &ACMEAccount{
		Server:        issuer.Acme.Server,
		Email:         issuer.Acme.Email,
		EABKeyID:      keyID,
		EABMacKeyName: macKeyName,
	}}}
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
) (FrontendGateway, error) {
	image, err := imgs.Name("frontendGatewayNginx")
	if err != nil {
		return FrontendGateway{}, fmt.Errorf("failed to resolve the frontend gateway image: %w", err)
	}

	tag, err := imgs.Tag("frontendGatewayNginx")
	if err != nil {
		return FrontendGateway{}, fmt.Errorf("failed to resolve the frontend gateway image: %w", err)
	}

	issuerName := selfSignedIssuerName
	if issuer != nil && issuer.Type != "" {
		issuerName = string(issuer.Type)
	}

	return FrontendGateway{
		Issuer:        issuerName,
		RedirectToIde: true,
		Image:         Image{Name: image, Tag: tag},
		GatewayAPI:    GatewayAPI{Enabled: gatewayAPIEnabled},
	}, nil
}
