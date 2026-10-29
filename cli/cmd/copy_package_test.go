// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/prompt"
	intutil "github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("CopyPackageCmd", func() {
	var (
		packageManager *installer.MockPackageManager
		fileIO         *intutil.MockFileIO
	)

	BeforeEach(func() {
		packageManager = installer.NewMockPackageManager(GinkgoT())
		fileIO = intutil.NewMockFileIO(GinkgoT())
	})

	preparePackageManager := func() {
		packageManager.EXPECT().Extract(false, false).Return(nil)
		packageManager.EXPECT().GetDependencyPath("bom-index.json").Return("bom-index.json")
		packageManager.EXPECT().GetDependencyPath("bom.json").Return("bom.json")
		packageManager.EXPECT().FileIO().Return(fileIO)
		fileIO.EXPECT().ReadFile("bom-index.json").Return(nil, os.ErrNotExist)
		fileIO.EXPECT().ReadFile("bom.json").Return([]byte(`{
			"components": {"codesphere": {"containerImages": {"api": "ghcr.io/codesphere/api:v1"}}}
		}`), nil)
	}

	It("asks for confirmation before copying", func() {
		preparePackageManager()

		prompter := prompt.NewMockPrompter(GinkgoT())
		prompter.EXPECT().Bool("Copy these artifacts?", false).Return(true)

		copier := installer.NewMockArtifactCopier(GinkgoT())
		copier.EXPECT().Copy(context.Background(), "ghcr.io/codesphere/api:v1", "registry.example.com/mirror/ghcr_io/codesphere/api:v1").Return(nil).Once()
		command := &CopyPackageCmd{
			Opts:     CopyPackageOpts{Dest: "registry.example.com/mirror"},
			Prompter: prompter,
		}

		Expect(command.CopyPackage(context.Background(), packageManager, copier)).To(Succeed())
	})

	It("cancels without copying when confirmation is declined", func() {
		preparePackageManager()

		prompter := prompt.NewMockPrompter(GinkgoT())
		prompter.EXPECT().Bool("Copy these artifacts?", false).Return(false)

		copier := installer.NewMockArtifactCopier(GinkgoT())
		command := &CopyPackageCmd{
			Opts:     CopyPackageOpts{Dest: "registry.example.com/mirror"},
			Prompter: prompter,
		}

		err := command.CopyPackage(context.Background(), packageManager, copier)
		Expect(err).To(MatchError("transfer cancelled"))
		copier.AssertNotCalled(GinkgoT(), "Copy", mock.Anything, mock.Anything, mock.Anything)
	})

	It("skips confirmation with --yes", func() {
		preparePackageManager()

		copier := installer.NewMockArtifactCopier(GinkgoT())
		copier.EXPECT().Copy(context.Background(), "ghcr.io/codesphere/api:v1", "registry.example.com/mirror/ghcr_io/codesphere/api:v1").Return(nil).Once()
		command := &CopyPackageCmd{
			Opts:     CopyPackageOpts{Dest: "registry.example.com/mirror", Yes: true},
			Prompter: prompt.NewMockPrompter(GinkgoT()),
		}

		Expect(command.CopyPackage(context.Background(), packageManager, copier)).To(Succeed())
	})

	It("copies from an index without reading the legacy BOM", func() {
		packageManager.EXPECT().Extract(false, false).Return(nil)
		packageManager.EXPECT().GetDependencyPath("bom-index.json").Return("bom-index.json")
		packageManager.EXPECT().FileIO().Return(fileIO)
		fileIO.EXPECT().ReadFile("bom-index.json").Return([]byte(`{
			"version": "v3", "components": [{"name": "app", "version": "2",
			"containerImages": {"api": {"ref": "ghcr.io/codesphere/api:v2"}}}]
		}`), nil)

		copier := installer.NewMockArtifactCopier(GinkgoT())
		copier.EXPECT().Copy(context.Background(), "ghcr.io/codesphere/api:v2", "registry.example.com/mirror/ghcr_io/codesphere/api:v2").Return(nil).Once()

		command := &CopyPackageCmd{Opts: CopyPackageOpts{Dest: "registry.example.com/mirror", Yes: true}}

		Expect(command.CopyPackage(context.Background(), packageManager, copier)).To(Succeed())
	})

	It("registers the command and validates package source flags", func() {
		root := GetRootCmd()
		copyPackage, _, err := root.Find([]string{"copy", "package"})
		Expect(err).NotTo(HaveOccurred())
		Expect(copyPackage).NotTo(BeNil())
		Expect(copyPackage.Flags().Lookup("dest")).NotTo(BeNil())
		Expect(copyPackage.Flags().Lookup("yes")).NotTo(BeNil())

		root.SetArgs([]string{"copy", "package", "--dest", "registry.example.com", "--yes"})
		Expect(root.Execute()).To(MatchError("exactly one of --package or --version must be specified"))
	})

	Describe("resolvePackage with --version", func() {
		var (
			mockEnv        *env.MockEnv
			mockPortal     *portal.MockPortal
			mockFileWriter *intutil.MockFileIO
			workdir        string
			build          portal.Build
			command        *CopyPackageCmd
		)

		BeforeEach(func() {
			workdir = GinkgoT().TempDir()
			mockEnv = env.NewMockEnv(GinkgoT())
			mockEnv.EXPECT().GetOmsWorkdir().Return(workdir)

			mockPortal = portal.NewMockPortal(GinkgoT())
			mockFileWriter = intutil.NewMockFileIO(GinkgoT())

			build = portal.Build{
				Version:   "codesphere-v1.70.0",
				Hash:      "abc1234567",
				Artifacts: []portal.Artifact{{Filename: defaultInstallerPackageArtifact}},
			}

			command = &CopyPackageCmd{
				Opts:       CopyPackageOpts{Version: build.Version, Filename: defaultInstallerPackageArtifact},
				Env:        mockEnv,
				FileWriter: mockFileWriter,
			}
		})

		It("downloads and verifies the upstream package", func() {
			mockPortal.EXPECT().GetBuild(portal.CodesphereProduct, build.Version, "").Return(build, nil)

			destination := filepath.Join(workdir, build.BuildPackageFilename(defaultInstallerPackageArtifact))
			fakeFile := os.NewFile(uintptr(0), destination)
			mockFileWriter.EXPECT().Create(destination).Return(fakeFile, nil)
			mockFileWriter.EXPECT().Open(destination).Return(fakeFile, nil)
			mockPortal.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, mock.Anything, mock.Anything, 0, false).Return(nil)
			mockPortal.EXPECT().VerifyBuildArtifactDownload(mock.Anything, mock.Anything).Return(nil)

			path, err := command.resolvePackage(mockPortal)
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal(destination))
		})

		It("returns an error when the upstream package cannot be found", func() {
			mockPortal.EXPECT().GetBuild(portal.CodesphereProduct, build.Version, "").Return(portal.Build{}, fmt.Errorf("build not found"))

			_, err := command.resolvePackage(mockPortal)
			Expect(err).To(MatchError(ContainSubstring("failed to get upstream package")))
		})
	})
})
