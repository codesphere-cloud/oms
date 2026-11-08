// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

// WriteAndEncryptVault exposes writeDataCenterConfig for the primary data center to the
// external test package.
func (b *GCPBootstrapper) WriteAndEncryptVault() error {
	if err := b.ensureDataCenters(); err != nil {
		return err
	}

	return b.writeDataCenterConfig(b.primaryDC())
}
