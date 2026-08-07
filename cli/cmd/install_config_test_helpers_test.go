// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/gomega"
)

func newPlainInstallConfigManager() installer.InstallConfigManager {
	manager, err := installer.NewInstallConfigManager("plain", "")
	Expect(err).NotTo(HaveOccurred())

	return manager
}

func newSOPSInstallConfigManager() installer.InstallConfigManager {
	manager, err := installer.NewInstallConfigManager("sops", "")
	Expect(err).NotTo(HaveOccurred())

	return manager
}
