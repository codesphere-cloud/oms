// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/images"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const selfSignedIssuerName = "self-signed"

// FrontendGateway is the gateway that serves Codesphere itself.
type FrontendGateway struct {
	Issuer        string             `yaml:"issuer"`
	RedirectToIde bool               `yaml:"redirectToIde"`
	Image         images.Image       `yaml:"image"`
	GatewayAPI    FrontendGatewayAPI `yaml:"gatewayApi"`
}

// FrontendGatewayAPI says whether the frontend gateway is a Gateway API Gateway.
type FrontendGatewayAPI struct {
	Enabled bool `yaml:"enabled"`
}

func frontendGatewayValues(
	imgs images.Images,
	issuer *files.CertIssuerConfig,
	gatewayAPIEnabled bool,
) (FrontendGateway, error) {
	image, err := imgs.Image("frontendGatewayNginx")
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
		Image:         image,
		GatewayAPI:    FrontendGatewayAPI{Enabled: gatewayAPIEnabled},
	}, nil
}
