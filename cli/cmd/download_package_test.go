// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("ListPackages", func() {

	var (
		c              cmd.DownloadPackageCmd
		filename       string
		version        string
		build          portal.Build
		mockPortal     *portal.MockPortal
		mockFileWriter *util.MockFileIO
	)

	BeforeEach(func() {
		filename = "installer.tar.gz"
		version = "codesphere-1.42.0"
		mockPortal = portal.NewMockPortal(GinkgoT())
		mockFileWriter = util.NewMockFileIO(GinkgoT())

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

	Context("File exists", func() {
		It("Downloads the correct artifact to the correct output file", func() {
			expectedBuildToDownload := portal.Build{
				Version: version,
				Artifacts: []portal.Artifact{
					{Filename: filename},
				},
			}

			fakeFile := os.NewFile(uintptr(0), filename)
			mockFileWriter.EXPECT().Create(version+"-"+filename).Return(fakeFile, nil)
			mockPortal.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, expectedBuildToDownload, mock.Anything, false).Return(nil)
			err := c.DownloadBuild(mockPortal, build, filename)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("File doesn't exist in build", func() {
		It("Returns an error", func() {
			err := c.DownloadBuild(mockPortal, build, "installer-lite.tar.gz")
			Expect(err).To(MatchError("failed to find artifact in package: artifact not found: installer-lite.tar.gz"))
		})
	})
})
