// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("local bootstrap install config version", func() {
	DescribeTable("uses config defaults while preserving explicit package choices",
		func(version, hash, local, configuredHash, expectedVersion, expectedHash string) {
			cfg := files.NewRootConfig()
			cfg.SetCodespherePackage("1.111.0", configuredHash)

			manager := installer.NewMockInstallConfigManager(GinkgoT())
			manager.EXPECT().GetInstallConfig().Return(&cfg)
			manager.EXPECT().ApplyProfile("minimal").Return(nil)

			fw := util.NewMockFileIO(GinkgoT())
			fw.EXPECT().Exists("config.yaml").Return(false)
			bootstrapper := &LocalBootstrapper{icg: manager, fw: fw, Env: &CodesphereEnvironment{
				Profile: "minimal", InstallConfigPath: "config.yaml", InstallVersion: version, InstallHash: hash, InstallLocal: local,
			}}
			Expect(bootstrapper.EnsureInstallConfig()).To(Succeed())
			Expect(bootstrapper.Env.InstallVersion).To(Equal(expectedVersion))
			Expect(bootstrapper.Env.InstallHash).To(Equal(expectedHash))
			Expect(bootstrapper.Env.InstallLocal).To(Equal(local))
		},
		Entry("config with hash", "", "", "", "abc123", "1.111.0", "abc123"),
		Entry("config without hash", "", "", "", "", "1.111.0", ""),
		Entry("explicit hash overrides config hash", "", "override", "", "abc123", "1.111.0", "override"),
		Entry("explicit version overrides config", "master", "", "", "abc123", "master", ""),
		Entry("explicit archive overrides config", "", "", "installer.tar.gz", "abc123", "", ""),
	)

	DescribeTable("stores the requested version without losing operation skips", func(hash string) {
		cfg := files.NewRootConfig()
		cfg.Operations = &files.OperationsConfig{Skip: []string{"ceph"}}
		manager := installer.NewMockInstallConfigManager(GinkgoT())
		manager.EXPECT().GetInstallConfig().Return(&cfg)
		manager.EXPECT().ApplyProfile("minimal").Return(nil)

		fw := util.NewMockFileIO(GinkgoT())
		fw.EXPECT().Exists("config.yaml").Return(false)
		bootstrapper := &LocalBootstrapper{icg: manager, fw: fw, Env: &CodesphereEnvironment{
			Profile: "minimal", InstallConfigPath: "config.yaml", InstallVersion: "1.111.0", InstallHash: hash,
		}}
		Expect(bootstrapper.EnsureInstallConfig()).To(Succeed())
		Expect(bootstrapper.Env.InstallConfig.Operations.Codesphere).To(Equal(files.CodespherePackageConfig{Version: "1.111.0", Hash: hash}))
		Expect(bootstrapper.Env.InstallConfig.Operations.Skip).To(Equal([]string{"ceph"}))
	}, Entry("version only", ""), Entry("pinned build", "abc123"))
})
