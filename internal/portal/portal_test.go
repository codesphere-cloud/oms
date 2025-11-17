// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
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
		headers        http.Header
		getResponse    []byte
		product        portal.Product
		apiKey         string
		apiKeyErr      error
	)
	BeforeEach(func() {
		apiKey = "fake-api-key"
		apiKeyErr = nil

		product = portal.CodesphereProduct
		mockEnv = env.NewMockEnv(GinkgoT())
		mockHttpClient = portal.NewMockHttpClient(GinkgoT())

		client = portal.PortalClient{
			Env:        mockEnv,
			HttpClient: mockHttpClient,
		}
		status = http.StatusOK
		apiUrl = "fake-portal.com"
	})
	JustBeforeEach(func() {
		mockEnv.EXPECT().GetOmsPortalApi().Return(apiUrl)
		mockEnv.EXPECT().GetOmsPortalApiKey().Return(apiKey, apiKeyErr).Maybe()
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
				}).Maybe()
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

		Context("when OMS_PORTAL_API_KEY is unset", func() {
			BeforeEach(func() {
				apiKey = ""
				apiKeyErr = errors.New("fake-error")
			})

			It("Returns an error", func() {
				_, status, err := client.GetBody("/api/fake")
				Expect(status).To(Equal(status))
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(MatchRegexp(".*fake-error"))
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
					headers = req.Header
					return &http.Response{
						StatusCode: status,
						Body:       io.NopCloser(bytes.NewReader([]byte(downloadResponse))),
					}, nil
				})
		})

		It("downloads the build", func() {
			fakeWriter := NewFakeWriter()
			err := client.DownloadBuildArtifact(product, build, fakeWriter, 0, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeWriter.String()).To(Equal(downloadResponse))
			Expect(getUrl.String()).To(Equal("fake-portal.com/packages/codesphere/download"))
		})

		It("resumes the build", func() {
			fakeWriter := NewFakeWriter()
			err := client.DownloadBuildArtifact(product, build, fakeWriter, 42, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(headers.Get("Range")).To(Equal("bytes=42-"))
			Expect(fakeWriter.String()).To(Equal(downloadResponse))
			Expect(getUrl.String()).To(Equal("fake-portal.com/packages/codesphere/download"))
		})

		It("emits progress logs when not quiet", func() {
			var logBuf bytes.Buffer
			prev := log.Writer()
			log.SetOutput(&logBuf)
			defer log.SetOutput(prev)

			fakeWriter := NewFakeWriter()
			err := client.DownloadBuildArtifact(product, build, fakeWriter, 0, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(logBuf.String()).To(ContainSubstring("Downloading..."))
		})

		It("does not emit progress logs when quiet", func() {
			var logBuf bytes.Buffer
			prev := log.Writer()
			log.SetOutput(&logBuf)
			defer log.SetOutput(prev)

			fakeWriter := NewFakeWriter()
			err := client.DownloadBuildArtifact(product, build, fakeWriter, 0, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(logBuf.String()).NotTo(ContainSubstring("Downloading..."))
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
				packages, err := client.GetBuild(portal.OmsProduct, "", "")
				Expect(err).NotTo(HaveOccurred())
				Expect(packages).To(Equal(expectedResult))
				Expect(getUrl.String()).To(Equal("fake-portal.com/packages/oms"))
			})
		})

		Context("When the build with version is included", func() {
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
				packages, err := client.GetBuild(portal.OmsProduct, "1.42.1", "")
				Expect(err).NotTo(HaveOccurred())
				Expect(packages).To(Equal(expectedResult))
				Expect(getUrl.String()).To(Equal("fake-portal.com/packages/oms"))
			})
		})

		Context("When the build with version and hash is included", func() {
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
				packages, err := client.GetBuild(portal.OmsProduct, "1.42.1", "lastBuild")
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
				packages, err := client.GetBuild(portal.OmsProduct, "", "")
				Expect(err).To(MatchError("no builds returned"))
				Expect(packages).To(Equal(expectedResult))
				Expect(getUrl.String()).To(Equal("fake-portal.com/packages/oms"))
			})
		})
	})
})
