// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Local Rook pc-apps configuration", func() {
	It("disables Rook in pc-apps while preserving other application settings", func() {
		config := files.NewRootConfig()
		config.PcApps = files.ChartValues{"applications": map[string]any{
			"rook-ceph": map[string]any{"enabled": true, "custom": "kept"},
			"other-app": map[string]any{"enabled": true},
		}}
		b := &LocalBootstrapper{Env: &CodesphereEnvironment{InstallConfig: &config, StorageEngine: StorageEngineRookCeph}}
		b.configureStorageValues()

		apps := config.PcApps["applications"].(map[string]any)
		Expect(apps["rook-ceph"]).To(Equal(map[string]any{"enabled": false, "custom": "kept"}))
		Expect(apps["other-app"]).To(Equal(map[string]any{"enabled": true}))
	})

	It("disables Rook in pc-apps when using local CSI storage", func() {
		config := files.NewRootConfig()
		b := &LocalBootstrapper{Env: &CodesphereEnvironment{InstallConfig: &config, StorageEngine: StorageEngineLocal}}
		b.configureStorageValues()

		apps := config.PcApps["applications"].(map[string]any)
		Expect(apps["rook-ceph"]).To(Equal(map[string]any{"enabled": false}))
	})
})
