// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package k0s_test

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	. "github.com/onsi/gomega"
)

func newPlainInstallConfigManager() installer.InstallConfigManager {
	manager, err := installer.NewInstallConfigManager("plain", "")
	Expect(err).NotTo(HaveOccurred())
	return manager
}
