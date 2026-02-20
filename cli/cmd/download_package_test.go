// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("DownloadPackages", func() {

	var (
		c              cmd.DownloadPackageCmd
		filename       string
		version        string
		hash           string
		build          portal.Build
		mockPortal     *portal.MockPortal
		mockFileWriter *util.MockFileIO
	)

	BeforeEach(func() {
		filename = "installer.tar.gz"
		version = "codesphere-1.42.0"
		hash = "abc1234567"
		mockPortal = portal.NewMockPortal(GinkgoT())
		mockFileWriter = util.NewMockFileIO(GinkgoT())
	})
	JustBeforeEach(func() {
		c = cmd.DownloadPackageCmd{
			Opts: cmd.DownloadPackageOpts{
				Version:  version,
				Filename: filename,
				Quiet:    false,
			},
			FileWriter: mockFileWriter,
		}
		build = portal.Build{
			Version: version,
			Hash:    hash,
			Artifacts: []portal.Artifact{
				{Filename: filename},
				{Filename: "otherFilename.tar.gz"},
			},
		}
	})
	AfterEach(func() {
		mockPortal.AssertExpectations(GinkgoT())
		mockFileWriter.AssertExpectations(GinkgoT())
	})

	Context("AddDownloadPackageCmd", func() {
		var downloadCmd cobra.Command
		var opts *cmd.GlobalOptions

		BeforeEach(func() {
			downloadCmd = cobra.Command{}
			opts = &cmd.GlobalOptions{}
		})

		It("valid package with version as flag", func() {
			downloadCmd.SetArgs([]string{
				"package",
				"--version", version + "-" + filename,
			})

			cmd.AddDownloadPackageCmd(&downloadCmd, opts)

			downloadCmd.Commands()[0].RunE = func(cmd *cobra.Command, args []string) error {
				return nil
			}

			err := downloadCmd.Execute()
			Expect(err).NotTo(HaveOccurred())
		})

		It("valid package with version and file as flag", func() {
			downloadCmd.SetArgs([]string{
				"package",
				"--version", version + "-" + filename,
				"--file", "installer-lite.tar.gz",
			})

			cmd.AddDownloadPackageCmd(&downloadCmd, opts)

			downloadCmd.Commands()[0].RunE = func(cmd *cobra.Command, args []string) error {
				return nil
			}

			err := downloadCmd.Execute()
			Expect(err).NotTo(HaveOccurred())
		})

		It("valid package with version as positional argument", func() {
			downloadCmd.SetArgs([]string{
				"package",
				version + "-" + filename,
			})

			cmd.AddDownloadPackageCmd(&downloadCmd, opts)

			downloadCmd.Commands()[0].RunE = func(cmd *cobra.Command, args []string) error {
				return nil
			}

			err := downloadCmd.Execute()
			Expect(err).NotTo(HaveOccurred())
		})

		It("valid package with version as positional argument and file as flag", func() {
			downloadCmd.SetArgs([]string{
				"package",
				version + "-" + filename,
				"--file", "installer-lite.tar.gz",
			})

			cmd.AddDownloadPackageCmd(&downloadCmd, opts)

			downloadCmd.Commands()[0].RunE = func(cmd *cobra.Command, args []string) error {
				return nil
			}

			err := downloadCmd.Execute()
			Expect(err).NotTo(HaveOccurred())
		})

		It("invalid package command without version", func() {
			downloadCmd.SetArgs([]string{
				"package",
			})

			cmd.AddDownloadPackageCmd(&downloadCmd, opts)

			downloadCmd.Commands()[0].RunE = func(cmd *cobra.Command, args []string) error {
				return nil
			}

			err := downloadCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("accepts 1 arg(s), received 0"))
		})

		It("invalid package command with duplicated version arg", func() {
			downloadCmd.SetArgs([]string{
				"package",
				version + "-" + filename,
				"--version", version + "-" + filename,
			})

			cmd.AddDownloadPackageCmd(&downloadCmd, opts)

			downloadCmd.Commands()[0].RunE = func(cmd *cobra.Command, args []string) error {
				return nil
			}

			err := downloadCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown command"))
		})
	})

	Context("File exists", func() {
		It("Downloads the correct artifact to the correct output file", func() {
			expectedBuildToDownload := portal.Build{
				Version: version,
				Hash:    hash,
				Artifacts: []portal.Artifact{
					{Filename: filename},
				},
			}

			fakeFile := os.NewFile(uintptr(0), filename)
			mockFileWriter.EXPECT().OpenAppend(version+"-"+hash+"-"+filename).Return(fakeFile, nil)
			mockFileWriter.EXPECT().Open(version+"-"+hash+"-"+filename).Return(fakeFile, nil)
			mockPortal.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, expectedBuildToDownload, mock.Anything, 0, false).Return(nil)
			mockPortal.EXPECT().VerifyBuildArtifactDownload(mock.Anything, expectedBuildToDownload).Return(nil)
			err := c.DownloadBuild(mockPortal, build, filename)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Truncates long hash to 10 characters in filename", func() {
			longHash := "abc1234567890defghij"
			buildWithLongHash := portal.Build{
				Version: version,
				Hash:    longHash,
				Artifacts: []portal.Artifact{
					{Filename: filename},
					{Filename: "otherFilename.tar.gz"},
				},
			}
			expectedBuildToDownload := portal.Build{
				Version: version,
				Hash:    longHash,
				Artifacts: []portal.Artifact{
					{Filename: filename},
				},
			}

			fakeFile := os.NewFile(uintptr(0), filename)
			mockFileWriter.EXPECT().OpenAppend(version+"-abc1234567-"+filename).Return(fakeFile, nil)
			mockFileWriter.EXPECT().Open(version+"-abc1234567-"+filename).Return(fakeFile, nil)
			mockPortal.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, expectedBuildToDownload, mock.Anything, 0, false).Return(nil)
			mockPortal.EXPECT().VerifyBuildArtifactDownload(mock.Anything, expectedBuildToDownload).Return(nil)
			err := c.DownloadBuild(mockPortal, buildWithLongHash, filename)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("Version contains a slash", func() {
			BeforeEach(func() {
				version = "other/version/v1.42.0"
			})
			It("Downloads the correct artifact to the correct output file", func() {
				expectedBuildToDownload := portal.Build{
					Version: version,
					Hash:    hash,
					Artifacts: []portal.Artifact{
						{Filename: filename},
					},
				}

				fakeFile := os.NewFile(uintptr(0), filename)
				mockFileWriter.EXPECT().OpenAppend("other-version-v1.42.0-"+hash+"-"+filename).Return(fakeFile, nil)
				mockFileWriter.EXPECT().Open("other-version-v1.42.0-"+hash+"-"+filename).Return(fakeFile, nil)
				mockPortal.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, expectedBuildToDownload, mock.Anything, 0, false).Return(nil)
				mockPortal.EXPECT().VerifyBuildArtifactDownload(mock.Anything, expectedBuildToDownload).Return(nil)
				err := c.DownloadBuild(mockPortal, build, filename)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("File doesn't exist in build", func() {
		It("Returns an error", func() {
			err := c.DownloadBuild(mockPortal, build, "installer-lite.tar.gz")
			Expect(err).To(MatchError("failed to find artifact in package: artifact not found: installer-lite.tar.gz"))
		})
	})
})
