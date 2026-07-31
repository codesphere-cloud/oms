// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
)

// DataCenterScopedSecretNames are the secrets that belong to exactly one data center and must
// therefore be regenerated for every additional data center of a multi-DC installation.
//
// Everything not listed here (and not matched by DataCenterScopedSecretPrefixes) is shared:
// the postgres roles because both data centers talk to the same server, and the auth and
// encryption keys because tokens minted and rows written in one data center are consumed in
// the other.
var DataCenterScopedSecretNames = []string{
	// Cluster ingress CA — paired with cluster.certificates.ca.certPem.
	files.SecretSelfSignedCaKeyPem,
	// cephadm SSH key — paired with ceph.cephAdmSshKey.publicKey.
	files.SecretCephSshPrivateKey,
	// Written by the installer's kubernetes step; points at one cluster's API server.
	files.SecretKubeConfig,
	// ACME external account binding — paired with codesphere.certIssuer.acme.eabKeyId.
	files.SecretAcmeEabMacKey,
	// Only present in recovered vaults; keyed to a single cluster's nix cache.
	files.SecretPrivNixSigningKey,
	files.SecretPubNixSigningKey,
}

// DataCenterScopedSecretPrefixes cover the Ceph cluster credentials that the installer's ceph
// step writes back into the vault (cephFsId, cephfsAdmin, csiRbdNode, rgwAdminAccessKey, ...).
// The installer owns those names, so they are matched by prefix rather than enumerated.
var DataCenterScopedSecretPrefixes = []string{"ceph", "csi", "rgw"}

// IsDataCenterScopedSecret reports whether a vault entry belongs to a single data center.
func IsDataCenterScopedSecret(name string) bool {
	for _, scoped := range DataCenterScopedSecretNames {
		if name == scoped {
			return true
		}
	}
	for _, prefix := range DataCenterScopedSecretPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// DeriveDataCenterVault returns a copy of the primary data center's vault with all
// data-center-scoped secrets removed. Running EnsureSecrets over the result regenerates exactly
// those, while the shared secrets stay byte-identical across data centers.
func DeriveDataCenterVault(primary *files.InstallVault) *files.InstallVault {
	derived := primary.Clone()
	if derived == nil {
		return nil
	}

	kept := make([]files.SecretEntry, 0, len(derived.Secrets))
	for _, entry := range derived.Secrets {
		if IsDataCenterScopedSecret(entry.Name) {
			continue
		}
		kept = append(kept, entry)
	}
	derived.Secrets = kept

	return derived
}

// ClearDataCenterScopedConfig resets the config fields that are written by the Ensure* functions
// alongside a data-center-scoped vault secret. Those pairs must always be mutated together: the
// Ensure* functions are gated on the vault entry, so a config field left in place would keep a
// stale value paired with a freshly generated key.
func ClearDataCenterScopedConfig(config *files.RootConfig) {
	// Paired with selfSignedCaKeyPem, written by EnsureIngressCA.
	config.Cluster.Certificates.CA.CertPem = ""
	// Paired with cephSshPrivateKey, written by EnsureCephSSHKeys.
	config.Ceph.CephAdmSSHKey.PublicKey = ""
	// Paired with acmeEabMacKey, obtained per data center from the ACME CA.
	if config.Codesphere.CertIssuer.Acme != nil {
		config.Codesphere.CertIssuer.Acme.EABKeyID = ""
	}
}
