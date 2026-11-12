// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"context"
	"errors"
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

		err := ci.Install(context.Background(), packageManager, configManager, nil, "linux", "amd64")

		Expect(err).To(MatchError(ContainSubstring("postgres.serverAddress must not be set when postgres.mode is 'install'")))
	})

	It("rejects a missing public IP before extracting the package when the codesphere step runs", func() {
		configManager := installer.NewMockConfigManager(GinkgoT())
		configManager.EXPECT().ParseConfigYaml("config.yaml").Return(files.RootConfig{}, nil)

		packageManager := installer.NewMockPackageManager(GinkgoT())
		ci := &installer.CodesphereInstaller{ConfigPath: "config.yaml"}

		err := ci.Install(context.Background(), packageManager, configManager, nil, "linux", "amd64")

		Expect(err).To(MatchError(ContainSubstring("invalid codesphere configuration: codesphere.publicIp is required")))
	})

	It("rejects an invalid public IP when the codesphere step runs", func() {
		config := files.RootConfig{Codesphere: files.CodesphereConfig{PublicIP: "not-an-ip"}}
		configManager := installer.NewMockConfigManager(GinkgoT())
		configManager.EXPECT().ParseConfigYaml("config.yaml").Return(config, nil)

		packageManager := installer.NewMockPackageManager(GinkgoT())
		ci := &installer.CodesphereInstaller{ConfigPath: "config.yaml"}

		err := ci.Install(context.Background(), packageManager, configManager, nil, "linux", "amd64")

		Expect(err).To(MatchError(ContainSubstring("invalid codesphere.publicIp: not-an-ip")))
	})

	It("does not require a public IP when the codesphere step is skipped", func() {
		configManager := installer.NewMockConfigManager(GinkgoT())
		configManager.EXPECT().ParseConfigYaml("config.yaml").Return(files.RootConfig{}, nil)

		packageManager := installer.NewMockPackageManager(GinkgoT())
		packageManager.EXPECT().Extract(false, false).Return(errors.New("extract failed"))

		ci := &installer.CodesphereInstaller{ConfigPath: "config.yaml", SkipSteps: []string{installer.CodesphereStep}}

		err := ci.Install(context.Background(), packageManager, configManager, nil, "linux", "amd64")

		Expect(err).To(MatchError(ContainSubstring("extract failed")))
	})

	It("does not require a public IP for local component installs", func() {
		configManager := installer.NewMockConfigManager(GinkgoT())
		configManager.EXPECT().ParseConfigYaml("config.yaml").Return(files.RootConfig{}, nil)

		packageManager := installer.NewMockPackageManager(GinkgoT())
		packageManager.EXPECT().Extract(false, false).Return(errors.New("extract failed"))

		ci := &installer.CodesphereInstaller{ConfigPath: "config.yaml", LocalComponents: true}

		err := ci.Install(context.Background(), packageManager, configManager, nil, "linux", "amd64")

		Expect(err).To(MatchError(ContainSubstring("extract failed")))
	})

	It("accepts a valid public IP when the codesphere step runs", func() {
		config := files.RootConfig{Codesphere: files.CodesphereConfig{PublicIP: "203.0.113.10"}}
		configManager := installer.NewMockConfigManager(GinkgoT())
		configManager.EXPECT().ParseConfigYaml("config.yaml").Return(config, nil)

		packageManager := installer.NewMockPackageManager(GinkgoT())
		packageManager.EXPECT().Extract(false, false).Return(errors.New("extract failed"))

		ci := &installer.CodesphereInstaller{ConfigPath: "config.yaml"}

		err := ci.Install(context.Background(), packageManager, configManager, nil, "linux", "amd64")

		Expect(err).To(MatchError(ContainSubstring("extract failed")))
	})

	// The config profiles default to 127.0.0.1, so a generated config that was never edited hits this.
	It("rejects the loopback placeholder public IP when the codesphere step runs", func() {
		config := files.RootConfig{Codesphere: files.CodesphereConfig{PublicIP: "127.0.0.1"}}
		configManager := installer.NewMockConfigManager(GinkgoT())
		configManager.EXPECT().ParseConfigYaml("config.yaml").Return(config, nil)

		packageManager := installer.NewMockPackageManager(GinkgoT())
		ci := &installer.CodesphereInstaller{ConfigPath: "config.yaml"}

		err := ci.Install(context.Background(), packageManager, configManager, nil, "linux", "amd64")

		Expect(err).To(MatchError(ContainSubstring("codesphere.publicIp 127.0.0.1 is not reachable from other machines")))
	})

	It("rejects an unspecified public IP when the codesphere step runs", func() {
		config := files.RootConfig{Codesphere: files.CodesphereConfig{PublicIP: "0.0.0.0"}}
		configManager := installer.NewMockConfigManager(GinkgoT())
		configManager.EXPECT().ParseConfigYaml("config.yaml").Return(config, nil)

		packageManager := installer.NewMockPackageManager(GinkgoT())
		ci := &installer.CodesphereInstaller{ConfigPath: "config.yaml"}

		err := ci.Install(context.Background(), packageManager, configManager, nil, "linux", "amd64")

		Expect(err).To(MatchError(ContainSubstring("codesphere.publicIp 0.0.0.0 is not reachable from other machines")))
	})

	It("accepts a private public IP for on-premises and VM-based installs", func() {
		config := files.RootConfig{Codesphere: files.CodesphereConfig{PublicIP: "192.168.5.15"}}
		configManager := installer.NewMockConfigManager(GinkgoT())
		configManager.EXPECT().ParseConfigYaml("config.yaml").Return(config, nil)

		packageManager := installer.NewMockPackageManager(GinkgoT())
		packageManager.EXPECT().Extract(false, false).Return(errors.New("extract failed"))

		ci := &installer.CodesphereInstaller{ConfigPath: "config.yaml"}

		err := ci.Install(context.Background(), packageManager, configManager, nil, "linux", "amd64")

		Expect(err).To(MatchError(ContainSubstring("extract failed")))
	})

	It("allows the loopback placeholder public IP for local component installs", func() {
		config := files.RootConfig{Codesphere: files.CodesphereConfig{PublicIP: "127.0.0.1"}}
		configManager := installer.NewMockConfigManager(GinkgoT())
		configManager.EXPECT().ParseConfigYaml("config.yaml").Return(config, nil)

		packageManager := installer.NewMockPackageManager(GinkgoT())
		packageManager.EXPECT().Extract(false, false).Return(errors.New("extract failed"))

		ci := &installer.CodesphereInstaller{ConfigPath: "config.yaml", LocalComponents: true}

		err := ci.Install(context.Background(), packageManager, configManager, nil, "linux", "amd64")

		Expect(err).To(MatchError(ContainSubstring("extract failed")))
	})
})
