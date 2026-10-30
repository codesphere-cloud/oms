// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere

import (
	"errors"
	"fmt"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

const (
	acmeEABMacKeyName          = "acmeEabMacKey"
	acmeCustomDomainsEABMacKey = "acmeCustomDomainsEabMacKey"
)

// CertIssuer is the ACME issuer of the workspaces' custom domains.
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

func certIssuerValues(issuer *files.CertIssuerConfig) (CertIssuer, error) {
	if issuer == nil {
		return CertIssuer{ACME: ACME{Enabled: false}}, nil
	}

	switch issuer.Type {
	case "", files.CertIssuerTypeSelfSigned:
		return CertIssuer{ACME: ACME{Enabled: false}}, nil
	case files.CertIssuerTypeACME:
		return acmeCertIssuerValues(issuer.Acme)
	default:
		return CertIssuer{}, fmt.Errorf("unknown certIssuer.type %q", issuer.Type)
	}
}

func acmeCertIssuerValues(acme *files.ACMEConfig) (CertIssuer, error) {
	if acme == nil {
		return CertIssuer{}, errors.New("certIssuer.acme is required when certIssuer.type is acme")
	}

	if !acme.Enabled {
		return CertIssuer{}, errors.New("certIssuer.acme.enabled must be true when certIssuer.type is acme")
	}

	keyID, macKeyName := customDomainsEAB(acme)

	return CertIssuer{ACME: ACME{Enabled: true, ACMEAccount: &ACMEAccount{
		Server:        acme.Server,
		Email:         acme.Email,
		EABKeyID:      keyID,
		EABMacKeyName: macKeyName,
	}}}, nil
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
