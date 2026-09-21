// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Codesphere installer", func() {
	It("passes verbose output through package extraction", func() {
		packageDir := GinkgoT().TempDir()
		for _, name := range []string{"deps.tar.gz", "private-cloud-installer.js", "node"} {
			Expect(os.WriteFile(filepath.Join(packageDir, name), nil, 0600)).To(Succeed())
		}

		entries, err := os.ReadDir(packageDir)
		Expect(err).NotTo(HaveOccurred())

		fileIO := util.NewMockFileIO(GinkgoT())
		fileIO.EXPECT().Exists(packageDir).Return(true)
		fileIO.EXPECT().ReadDir(packageDir).Return(entries, nil)

		packageManager := installer.NewMockPackageManager(GinkgoT())
		packageManager.EXPECT().Extract(false, true).Return(nil)
		packageManager.EXPECT().GetWorkDir().Return(packageDir)
		packageManager.EXPECT().FileIO().Return(fileIO).Twice()
		packageManager.EXPECT().ExtractDependency("bom.json", false, true).Return(nil)

		ci := &installer.CodesphereInstaller{Verbose: true}

		err = ci.ExtractAndValidatePackage(packageManager)

		Expect(err).NotTo(HaveOccurred())
	})

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
