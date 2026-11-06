// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

// ConfigureCustomTLS validates supplied certificate material and configures the
// platform and workspace gateways. It performs no issuance or persistence.
// Unrelated settings and unresolved secret references are preserved.
func ConfigureCustomTLS(config map[string]any, cert, key []byte, secrets *files.InstallVault, domains []string) (map[string]any, error) {
	if err := validateCustomCertificate(cert, key, domains); err != nil {
		return nil, err
	}

	const encodedKeyName = "bootstrapWorkspaceTLSKeyBase64"

	manualCertificate := map[string]any{"nginx": map[string]any{"ingress": map[string]any{"certificate": map[string]any{
		"provideCertificateManually": true, "cert": base64.StdEncoding.EncodeToString(cert),
		"key": `{{ secret "` + encodedKeyName + `" }}`,
	}}}}
	updates := map[string]any{
		"codesphere": map[string]any{
			// OMS resolves the vault key into the temporary TS installer config until
			// the frontend gateway is migrated to PC Apps.
			"customCertificate": map[string]any{"cert": string(cert), "key": ""},
			"certIssuer":        map[string]any{"type": "self-signed", "acme": map[string]any{"enabled": false}},
		},
		"cluster": map[string]any{
			"certificates": map[string]any{"override": map[string]any{"issuers": map[string]any{
				"acme": map[string]any{"enabled": false}, "letsEncryptHttp": map[string]any{"enabled": false},
			}}},
			"publicGateway": map[string]any{"override": manualCertificate},
		},
		"pcApps": map[string]any{"applications": map[string]any{"public-gateway": map[string]any{"valuesObject": manualCertificate}}},
	}

	secrets.SetSecret(files.SecretEntry{Name: files.SecretCustomCertificateKey, File: &files.SecretFile{Name: "tls.key", Content: string(key)}})
	secrets.SetSecret(files.SecretEntry{Name: encodedKeyName, Fields: &files.SecretFields{Password: base64.StdEncoding.EncodeToString(key)}})

	return util.DeepMergeMaps(config, updates), nil
}

func validateCustomCertificate(cert, key []byte, domains []string) error {
	pair, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return fmt.Errorf("invalid custom TLS certificate/key pair: %w", err)
	}

	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return fmt.Errorf("parsing custom TLS certificate: %w", err)
	}

	for _, domain := range domains {
		hostname := strings.Replace(domain, "*.", "oms-certificate-validation.", 1)
		if err := leaf.VerifyHostname(hostname); err != nil {
			return fmt.Errorf("custom certificate does not cover %s: %w", domain, err)
		}
	}

	return nil
}
