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

type commandRecordingCopier struct {
	copies []installer.PackageArtifact
}

func (c *commandRecordingCopier) Copy(_ context.Context, source, destination string) error {
	c.copies = append(c.copies, installer.PackageArtifact{Source: source, Destination: destination})
	return nil
}

var _ = Describe("CopyPackageCmd", func() {
	var (
		packageManager *installer.MockPackageManager
		bomPath        string
	)

	BeforeEach(func() {
		packageManager = installer.NewMockPackageManager(GinkgoT())
		bomPath = filepath.Join(GinkgoT().TempDir(), "bom.json")
		Expect(os.WriteFile(bomPath, []byte(`{
			"components": {"codesphere": {"containerImages": {"api": "ghcr.io/codesphere/api:v1"}}}
		}`), 0644)).To(Succeed())
	})

	preparePackageManager := func() {
		packageManager.EXPECT().Extract(false).Return(nil)
		packageManager.EXPECT().GetDependencyPath("bom.json").Return(bomPath)
	}

	It("asks for confirmation before copying", func() {
		preparePackageManager()

		prompter := prompt.NewMockPrompter(GinkgoT())
		prompter.EXPECT().Bool("Copy these artifacts?", false).Return(true)

		copier := &commandRecordingCopier{}
		command := &CopyPackageCmd{
			Opts:     CopyPackageOpts{Dest: "registry.example.com/mirror"},
			Prompter: prompter,
		}

		Expect(command.CopyPackage(context.Background(), packageManager, copier)).To(Succeed())
		Expect(copier.copies).To(Equal([]installer.PackageArtifact{{
			Source:      "ghcr.io/codesphere/api:v1",
			Destination: "registry.example.com/mirror/codesphere/api:v1",
		}}))
	})

	It("cancels without copying when confirmation is declined", func() {
		preparePackageManager()

		prompter := prompt.NewMockPrompter(GinkgoT())
		prompter.EXPECT().Bool("Copy these artifacts?", false).Return(false)

		copier := &commandRecordingCopier{}
		command := &CopyPackageCmd{
			Opts:     CopyPackageOpts{Dest: "registry.example.com/mirror"},
			Prompter: prompter,
		}

		err := command.CopyPackage(context.Background(), packageManager, copier)
		Expect(err).To(MatchError("transfer cancelled"))
		Expect(copier.copies).To(BeEmpty())
	})

	It("skips confirmation with --yes", func() {
		preparePackageManager()

		copier := &commandRecordingCopier{}
		command := &CopyPackageCmd{
			Opts:     CopyPackageOpts{Dest: "registry.example.com/mirror", Yes: true},
			Prompter: prompt.NewMockPrompter(GinkgoT()),
		}

		Expect(command.CopyPackage(context.Background(), packageManager, copier)).To(Succeed())
		Expect(copier.copies).To(HaveLen(1))
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
