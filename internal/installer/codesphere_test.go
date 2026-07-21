// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Codesphere installer", func() {
	It("rejects serverAddress in postgres install mode before running PCInstaller", func() {
		config := files.RootConfig{
			Postgres: files.PostgresConfig{
				Mode:          "install",
				ServerAddress: "postgres.example.com:5432",
			},
		}
		configManager := installer.NewMockConfigManager(GinkgoT())
		configManager.EXPECT().ParseConfigYaml("config.yaml").Return(config, nil)
		packageManager := installer.NewMockPackageManager(GinkgoT())
		ci := &installer.CodesphereInstaller{ConfigPath: "config.yaml"}

		err := ci.Install(packageManager, configManager, nil, "linux", "amd64")

		Expect(err).To(MatchError(ContainSubstring("postgres.serverAddress must not be set when postgres.mode is 'install'")))
	})
})
