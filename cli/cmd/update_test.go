package cmd_test

import (
	"embed"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/version"
)

// I didn't find a good way to do this in memory without just reversing the code under test.
// While this is not ideal, at least it doesn't read from file during the test run but during compilation.
//
//go:generate mkdir -p testdata
//go:generate sh -c "echo fake-cli > testdata/oms-cli"
//go:generate sh -c "cd testdata && tar cfz testcli.tar.gz oms-cli"
//go:embed testdata
var testdata embed.FS

var _ = Describe("Update", func() {

	var (
		mockPortal      *portal.MockPortal
		mockVersion     *version.MockVersion
		mockUpdater     *cmd.MockUpdater
		latestBuild     portal.Build
		buildToDownload portal.Build
		c               cmd.UpdateCmd
	)

	BeforeEach(func() {
		mockPortal = portal.NewMockPortal(GinkgoT())
		mockVersion = version.NewMockVersion(GinkgoT())
		mockUpdater = cmd.NewMockUpdater(GinkgoT())

		latestBuild = portal.Build{
			Version: "0.0.42",
			Artifacts: []portal.Artifact{
				{Filename: "fakeos_fakearch.tar.gz"},
				{Filename: "fakeos2_fakearch2.tar.gz"},
				{Filename: "fakeos3_fakearch3.tar.gz"},
			},
		}
		buildToDownload = portal.Build{
			Version: "0.0.42",
			Artifacts: []portal.Artifact{
				{Filename: "fakeos_fakearch.tar.gz"},
			},
		}
		c = cmd.UpdateCmd{
			Version: mockVersion,
			Updater: mockUpdater,
		}
	})

	Describe("SelfUpdate", func() {
		It("Extracts oms-cli from the downloaded archive", func() {
			mockVersion.EXPECT().Arch().Return("fakearch")
			mockVersion.EXPECT().Version().Return("0.0.0")
			mockVersion.EXPECT().Os().Return("fakeos")
			mockPortal.EXPECT().GetLatestBuild(portal.OmsProduct).Return(latestBuild, nil)
			mockPortal.EXPECT().DownloadBuildArtifact(portal.OmsProduct, buildToDownload, mock.Anything).RunAndReturn(
				func(product portal.Product, build portal.Build, file io.Writer) error {
					embeddedFile, err := testdata.Open("testdata/testcli.tar.gz")
					if err != nil {
						Expect(err).NotTo(HaveOccurred())
					}
					defer func() { _ = embeddedFile.Close() }()

					if _, err := io.Copy(file, embeddedFile); err != nil {
						Expect(err).NotTo(HaveOccurred())
					}
					return nil
				})
			mockUpdater.EXPECT().Apply(mock.Anything).RunAndReturn(func(update io.Reader) error {
				output, err := io.ReadAll(update)
				Expect(err).NotTo(HaveOccurred())
				// file content written in go:generate
				Expect(string(output)).To(Equal("fake-cli\n"))
				return nil
			})
			err := c.SelfUpdate(mockPortal)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Detects when current verison is latest version", func() {
			mockVersion.EXPECT().Version().Return(latestBuild.Version)
			mockPortal.EXPECT().GetLatestBuild(portal.OmsProduct).Return(latestBuild, nil)
			err := c.SelfUpdate(mockPortal)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
