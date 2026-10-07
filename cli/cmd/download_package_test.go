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
	"github.com/codesphere-cloud/oms/cli/cmd/util"
	"github.com/codesphere-cloud/oms/internal/portal"
	intutil "github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("DownloadPackages", func() {

	var (
		c              cmd.DownloadPackageCmd
		filename       string
		version        string
		hash           string
		build          portal.Build
		mockPortal     *portal.MockPortal
		mockFileWriter *intutil.MockFileIO
	)

	BeforeEach(func() {
		filename = "installer.tar.gz"
		version = "codesphere-1.42.0"
		hash = "abc1234567"
		mockPortal = portal.NewMockPortal(GinkgoT())
		mockFileWriter = intutil.NewMockFileIO(GinkgoT())
	})
	JustBeforeEach(func() {
		c = cmd.DownloadPackageCmd{
			Opts: cmd.DownloadPackageOpts{
				GlobalOptions: &util.GlobalOptions{},
				Version:       version,
				Filename:      filename,
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
		var opts *util.GlobalOptions

		BeforeEach(func() {
			downloadCmd = cobra.Command{}
			opts = &util.GlobalOptions{}
		})

		DescribeTable("accepts progress flags", func(flags []string, quiet, noProgress bool) {
			downloadCmd.PersistentFlags().BoolVar(&opts.Verbose, "verbose", false, "Enable verbose output")
			cmd.AddDownloadPackageCmd(&downloadCmd, opts)
			packageCmd := downloadCmd.Commands()[0]
			packageCmd.RunE = func(command *cobra.Command, _ []string) error {
				Expect(command.Flags().GetBool("quiet")).To(Equal(quiet))
				Expect(command.Flags().GetBool("no-progress")).To(Equal(noProgress))

				return nil
			}

			downloadCmd.SetArgs(append([]string{"package", version}, flags...))
			Expect(downloadCmd.Execute()).To(Succeed())
		},
			Entry("default", []string{}, false, false),
			Entry("quiet", []string{"--quiet"}, true, false),
			Entry("quiet shorthand", []string{"-q"}, true, false),
			Entry("no progress", []string{"--no-progress"}, false, true),
			Entry("quiet with verbose", []string{"--quiet", "--verbose"}, true, false),
			Entry("no progress with verbose", []string{"--no-progress", "--verbose"}, false, true),
			Entry("both", []string{"--quiet", "--no-progress"}, true, true),
			Entry("explicitly disabled", []string{"--quiet=false", "--no-progress=false"}, false, false),
			Entry("quiet despite disabled no-progress", []string{"--quiet", "--no-progress=false"}, true, false),
			Entry("no-progress despite disabled quiet", []string{"--no-progress", "--quiet=false"}, false, true),
		)

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

	DescribeTable("controls download progress independently of verbose output", func(quiet, noProgress, verbose, suppressProgress bool) {
		c.Opts.Quiet = quiet
		c.Opts.NoProgress = noProgress
		c.Opts.Verbose = verbose
		expectedBuild := portal.Build{
			Version:   version,
			Hash:      hash,
			Artifacts: []portal.Artifact{{Filename: filename}},
		}
		file, err := os.CreateTemp(GinkgoT().TempDir(), "download-*")
		Expect(err).NotTo(HaveOccurred())
		mockFileWriter.EXPECT().OpenAppend(build.BuildPackageFilename(filename)).Return(file, nil)
		mockFileWriter.EXPECT().Open(build.BuildPackageFilename(filename)).Return(file, nil)
		mockPortal.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, expectedBuild, file, 0, suppressProgress).Return(nil)
		mockPortal.EXPECT().VerifyBuildArtifactDownload(file, expectedBuild).Return(nil)
		Expect(c.DownloadBuild(mockPortal, build, filename)).To(Succeed())
	},
		Entry("progress by default", false, false, false, false),
		Entry("progress with verbose", false, false, true, false),
		Entry("quiet", true, false, false, true),
		Entry("no progress", false, true, false, true),
		Entry("quiet overrides verbose", true, false, true, true),
		Entry("no progress overrides verbose", false, true, true, true),
		Entry("both flags", true, true, false, true),
		Entry("both flags override verbose", true, true, true, true),
	)

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
			Expect(err).To(MatchError("failed to download and verify build: failed to find artifact in package: artifact not found: installer-lite.tar.gz"))
		})
	})
})
