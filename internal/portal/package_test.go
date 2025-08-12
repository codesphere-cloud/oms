package portal_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/portal"
)

var _ = Describe("GetBuildForDownload", func() {
	It("Extracts a build with a single matching artifact", func() {

		build := portal.Build{
			Artifacts: []portal.Artifact{
				{Filename: "a.txt"},
				{Filename: "b.txt"},
				{Filename: "c.txt"},
			},
		}

		expectedBuild := portal.Build{
			Artifacts: []portal.Artifact{
				{Filename: "b.txt"},
			},
		}

		res, err := build.GetBuildForDownload("b.txt")
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(expectedBuild))
	})

})
