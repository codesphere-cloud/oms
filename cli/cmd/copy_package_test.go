// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/prompt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
})
