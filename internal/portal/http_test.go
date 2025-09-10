package portal_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type FakeWriter struct {
	bytes.Buffer
}

var _ io.Writer = (*FakeWriter)(nil)

func NewFakeWriter() *FakeWriter {
	return &FakeWriter{}
}

var _ = Describe("PortalClient", func() {
	var (
		client         portal.PortalClient
		mockEnv        *env.MockEnv
		mockHttpClient *portal.MockHttpClient
		status         int
		apiUrl         string
		getUrl         url.URL
		getResponse    []byte
		product        portal.Product
	)
	BeforeEach(func() {
		product = portal.CodesphereProduct
		mockEnv = env.NewMockEnv(GinkgoT())
		mockHttpClient = portal.NewMockHttpClient(GinkgoT())

		client = portal.PortalClient{
			Env:        mockEnv,
			HttpClient: mockHttpClient,
		}
		status = http.StatusOK
		apiUrl = "fake-portal.com"

		mockEnv.EXPECT().GetOmsPortalApi().Return(apiUrl)
		mockEnv.EXPECT().GetOmsPortalApiKey().Return("fake-api-key", nil)
	})
	AfterEach(func() {
		mockEnv.AssertExpectations(GinkgoT())
		mockHttpClient.AssertExpectations(GinkgoT())
	})

	Describe("GetBody", func() {
		JustBeforeEach(func() {
			mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
				func(req *http.Request) (*http.Response, error) {
					getUrl = *req.URL
					return &http.Response{
						StatusCode: status,
						Body:       io.NopCloser(bytes.NewReader(getResponse)),
					}, nil
				})
		})

		Context("when path starts with a /", func() {
			It("Executes a request against the right URL", func() {
				_, status, err := client.GetBody("/api/fake")
				Expect(status).To(Equal(status))
				Expect(err).NotTo(HaveOccurred())
				Expect(getUrl.String()).To(Equal("fake-portal.com/api/fake"))
			})
		})

		Context("when path does not with a /", func() {
			It("Executes a request against the right URL", func() {
				_, status, err := client.GetBody("api/fake")
				Expect(status).To(Equal(status))
				Expect(err).NotTo(HaveOccurred())
				Expect(getUrl.String()).To(Equal("fake-portal.com/api/fake"))
			})
		})
	})

	Describe("ListCodespherePackages", func() {
		JustBeforeEach(func() {
			mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
				func(req *http.Request) (*http.Response, error) {
					getUrl = *req.URL
					return &http.Response{
						StatusCode: status,
						Body:       io.NopCloser(bytes.NewReader(getResponse)),
					}, nil
				})
		})
		Context("when the request suceeds", func() {
			var expectedResult portal.Builds
			BeforeEach(func() {
				firstBuild, _ := time.Parse("2006-01-02", "2025-04-02")
				lastBuild, _ := time.Parse("2006-01-02", "2025-05-01")

				getPackagesResponse := portal.Builds{
					Builds: []portal.Build{
						{
							Hash: "lastBuild",
							Date: lastBuild,
						},
						{
							Hash: "firstBuild",
							Date: firstBuild,
						},
					},
				}
				getResponse, _ = json.Marshal(getPackagesResponse)

				expectedResult = portal.Builds{
					Builds: []portal.Build{
						{
							Hash: "firstBuild",
							Date: firstBuild,
						},
						{
							Hash: "lastBuild",
							Date: lastBuild,
						},
					},
				}
			})

			It("returns the builds ordered by date", func() {
				packages, err := client.ListBuilds(portal.CodesphereProduct)
				Expect(err).NotTo(HaveOccurred())
				Expect(packages).To(Equal(expectedResult))
				Expect(getUrl.String()).To(Equal("fake-portal.com/packages/codesphere"))
			})
		})
	})

	Describe("GetCodesphereBuildByVersion", func() {
		var (
			lastBuild, firstBuild time.Time
		)
		JustBeforeEach(func() {
			mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
				func(req *http.Request) (*http.Response, error) {
					getUrl = *req.URL
					return &http.Response{
						StatusCode: status,
						Body:       io.NopCloser(bytes.NewReader(getResponse)),
					}, nil
				})
		})
		BeforeEach(func() {
			firstBuild, _ = time.Parse("2006-01-02", "2025-04-02")
			lastBuild, _ = time.Parse("2006-01-02", "2025-05-01")

			getPackagesResponse := portal.Builds{
				Builds: []portal.Build{
					{
						Hash:    "lastBuild",
						Date:    lastBuild,
						Version: "1.42.0",
					},
					{
						Hash:    "firstBuild",
						Date:    firstBuild,
						Version: "1.42.1",
					},
				},
			}
			getResponse, _ = json.Marshal(getPackagesResponse)
		})

		Context("When the build is included", func() {
			It("returns the build", func() {
				expectedResult := portal.Build{
					Hash:    "lastBuild",
					Date:    lastBuild,
					Version: "1.42.0",
				}
				packages, err := client.GetCodesphereBuildByVersion("1.42.0")
				Expect(err).NotTo(HaveOccurred())
				Expect(packages).To(Equal(expectedResult))
				Expect(getUrl.String()).To(Equal("fake-portal.com/packages/codesphere"))
			})
		})

		Context("When the build is not included", func() {
			It("returns an error and an empty build", func() {
				expectedResult := portal.Build{}
				packages, err := client.GetCodesphereBuildByVersion("1.42.3")
				Expect(err.Error()).To(MatchRegexp(".*version 1.42.3 not found"))
				Expect(packages).To(Equal(expectedResult))
				Expect(getUrl.String()).To(Equal("fake-portal.com/packages/codesphere"))
			})
		})
	})

	Describe("DownloadBuildArtifact", func() {
		var (
			build            portal.Build
			downloadResponse string
		)
		BeforeEach(func() {
			buildDate, _ := time.Parse("2006-01-02", "2025-05-01")

			downloadResponse = "fake-file-contents"

			build = portal.Build{
				Date: buildDate,
			}

			mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
				func(req *http.Request) (*http.Response, error) {
					getUrl = *req.URL
					return &http.Response{
						StatusCode: status,
						Body:       io.NopCloser(bytes.NewReader([]byte(downloadResponse))),
					}, nil
				})
		})

		It("downloads the build", func() {
			fakeWriter := NewFakeWriter()
			err := client.DownloadBuildArtifact(product, build, fakeWriter)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeWriter.String()).To(Equal(downloadResponse))
			Expect(getUrl.String()).To(Equal("fake-portal.com/packages/codesphere/download"))
		})
	})

	Describe("GetLatestOmsBuild", func() {
		var (
			lastBuild, firstBuild time.Time
			getPackagesResponse   portal.Builds
		)
		JustBeforeEach(func() {
			getResponse, _ = json.Marshal(getPackagesResponse)
			mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
				func(req *http.Request) (*http.Response, error) {
					getUrl = *req.URL
					return &http.Response{
						StatusCode: status,
						Body:       io.NopCloser(bytes.NewReader(getResponse)),
					}, nil
				})
		})

		Context("When the build is included", func() {
			BeforeEach(func() {
				firstBuild, _ = time.Parse("2006-01-02", "2025-04-02")
				lastBuild, _ = time.Parse("2006-01-02", "2025-05-01")

				getPackagesResponse = portal.Builds{
					Builds: []portal.Build{
						{
							Hash:    "firstBuild",
							Date:    firstBuild,
							Version: "1.42.0",
						},
						{
							Hash:    "lastBuild",
							Date:    lastBuild,
							Version: "1.42.1",
						},
					},
				}
			})
			It("returns the build", func() {
				expectedResult := portal.Build{
					Hash:    "lastBuild",
					Date:    lastBuild,
					Version: "1.42.1",
				}
				packages, err := client.GetLatestBuild(portal.OmsProduct, "")
				Expect(err).NotTo(HaveOccurred())
				Expect(packages).To(Equal(expectedResult))
				Expect(getUrl.String()).To(Equal("fake-portal.com/packages/oms"))
			})
		})

		Context("When no builds are returned", func() {
			BeforeEach(func() {
				firstBuild, _ = time.Parse("2006-01-02", "2025-04-02")
				lastBuild, _ = time.Parse("2006-01-02", "2025-05-01")

				getPackagesResponse = portal.Builds{
					Builds: []portal.Build{},
				}
			})
			It("returns an error and an empty build", func() {
				expectedResult := portal.Build{}
				packages, err := client.GetLatestBuild(portal.OmsProduct, "")
				Expect(err).To(MatchError("no builds returned"))
				Expect(packages).To(Equal(expectedResult))
				Expect(getUrl.String()).To(Equal("fake-portal.com/packages/oms"))
			})
		})
	})
})
