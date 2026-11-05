// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"

	"github.com/codesphere-cloud/oms/internal/configtemplating"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// ResolveLegacyCertificateKey fills the custom certificate key in the temporary
// installer config. Persistent configuration keeps the private key in the vault;
// the legacy TypeScript installer expects the PEM value in customCertificate.key.
func ResolveLegacyCertificateKey(config map[string]any, store configtemplating.SecretStore) error {
	codesphere, _ := config["codesphere"].(map[string]any)

	certificate, _ := codesphere["customCertificate"].(map[string]any)
	if certificate == nil {
		return nil
	}

	if key, _ := certificate["key"].(string); key != "" {
		return nil
	}

	key, err := store.LookupSecret(files.SecretCustomCertificateKey, "file.content")
	if err != nil {
		return fmt.Errorf("resolving custom certificate private key: %w", err)
	}

	if key == "" {
		return fmt.Errorf("custom certificate private key %q is empty", files.SecretCustomCertificateKey)
	}

	certificate["key"] = key

	return nil
}
