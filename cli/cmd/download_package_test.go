// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"fmt"
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

		It("Uses long hash in filename", func() {
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
			mockFileWriter.EXPECT().OpenAppend(version+"-"+longHash+"-"+filename).Return(fakeFile, nil)
			mockFileWriter.EXPECT().Open(version+"-"+longHash+"-"+filename).Return(fakeFile, nil)
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

	Context("Stale partial file causes verify failure", func() {
		var (
			expectedBuildToDownload portal.Build
			fullFilename            string
			staleFile               *os.File
			freshFile               *os.File
		)

		BeforeEach(func() {
			expectedBuildToDownload = portal.Build{
				Version: version,
				Hash:    hash,
				Artifacts: []portal.Artifact{
					{Filename: filename},
				},
			}
			fullFilename = version + "-" + hash + "-" + filename

			// Create a temp file with non-zero content to simulate a stale partial download.
			var err error
			staleFile, err = os.CreateTemp("", "stale-download-*")
			Expect(err).NotTo(HaveOccurred())
			_, err = staleFile.Write([]byte("stale partial content"))
			Expect(err).NotTo(HaveOccurred())
			_, err = staleFile.Seek(0, 0)
			Expect(err).NotTo(HaveOccurred())

			freshFile = os.NewFile(uintptr(0), filename)
		})

		AfterEach(func() {
			_ = staleFile.Close() // may already be closed by the code under test
			Expect(os.Remove(staleFile.Name())).To(Succeed())
		})

		It("deletes the partial file and retries from scratch when resumed download fails verification", func() {
			mockFileWriter.EXPECT().OpenAppend(fullFilename).Return(staleFile, nil).Once()
			mockPortal.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, expectedBuildToDownload, mock.Anything, mock.AnythingOfType("int"), false).Return(nil).Once()
			mockFileWriter.EXPECT().Open(fullFilename).Return(staleFile, nil).Once()
			mockPortal.EXPECT().VerifyBuildArtifactDownload(mock.Anything, expectedBuildToDownload).Return(fmt.Errorf("invalid md5Sum: expected abc, but got xyz")).Once()
			mockFileWriter.EXPECT().Remove(fullFilename).Return(nil).Once()

			// Retry: fresh download succeeds
			mockFileWriter.EXPECT().OpenAppend(fullFilename).Return(nil, fmt.Errorf("file not found")).Once()
			mockFileWriter.EXPECT().Create(fullFilename).Return(freshFile, nil).Once()
			mockPortal.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, expectedBuildToDownload, mock.Anything, 0, false).Return(nil).Once()
			mockFileWriter.EXPECT().Open(fullFilename).Return(freshFile, nil).Once()
			mockPortal.EXPECT().VerifyBuildArtifactDownload(mock.Anything, expectedBuildToDownload).Return(nil).Once()

			err := c.DownloadBuild(mockPortal, build, filename)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns verify error without retry when file size is zero", func() {
			emptyFile, createErr := os.CreateTemp("", "empty-download-*")
			Expect(createErr).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = emptyFile.Close() // may already be closed by the code under test
				Expect(os.Remove(emptyFile.Name())).To(Succeed())
			})

			mockFileWriter.EXPECT().OpenAppend(fullFilename).Return(emptyFile, nil)
			mockPortal.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, expectedBuildToDownload, mock.Anything, 0, false).Return(nil)
			mockFileWriter.EXPECT().Open(fullFilename).Return(emptyFile, nil)
			mockPortal.EXPECT().VerifyBuildArtifactDownload(mock.Anything, expectedBuildToDownload).Return(fmt.Errorf("invalid md5Sum: expected abc, but got xyz"))

			err := c.DownloadBuild(mockPortal, build, filename)
			Expect(err).To(MatchError(ContainSubstring("failed to verify artifact")))
		})

		It("returns verify error when remove of stale file fails", func() {
			mockFileWriter.EXPECT().OpenAppend(fullFilename).Return(staleFile, nil)
			mockPortal.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, expectedBuildToDownload, mock.Anything, mock.AnythingOfType("int"), false).Return(nil)
			mockFileWriter.EXPECT().Open(fullFilename).Return(staleFile, nil)
			mockPortal.EXPECT().VerifyBuildArtifactDownload(mock.Anything, expectedBuildToDownload).Return(fmt.Errorf("invalid md5Sum: expected abc, but got xyz"))
			mockFileWriter.EXPECT().Remove(fullFilename).Return(fmt.Errorf("permission denied"))

			err := c.DownloadBuild(mockPortal, build, filename)
			Expect(err).To(MatchError(ContainSubstring("failed to verify artifact")))
		})
	})
})
