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
					return &http.Response{
						StatusCode: status,
						Body:       io.NopCloser(bytes.NewReader([]byte(downloadResponse))),
					}, nil
				})
		})

		It("downloads the build", func() {
			fakeWriter := NewFakeWriter()
			err := client.DownloadBuildArtifact(product, build, fakeWriter, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeWriter.String()).To(Equal(downloadResponse))
			Expect(getUrl.String()).To(Equal("fake-portal.com/packages/codesphere/download"))
		})

		It("emits progress logs when not quiet", func() {
			var logBuf bytes.Buffer
			prev := log.Writer()
			log.SetOutput(&logBuf)
			defer log.SetOutput(prev)

			fakeWriter := NewFakeWriter()
			err := client.DownloadBuildArtifact(product, build, fakeWriter, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(logBuf.String()).To(ContainSubstring("Downloading..."))
		})

		It("does not emit progress logs when quiet", func() {
			var logBuf bytes.Buffer
			prev := log.Writer()
			log.SetOutput(&logBuf)
			defer log.SetOutput(prev)

			fakeWriter := NewFakeWriter()
			err := client.DownloadBuildArtifact(product, build, fakeWriter, true)
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

	Describe("GetApiKeyByHeader", func() {
		var (
			oldApiKey     string
			newApiKey     string
			responseBody  []byte
			requestHeader string
		)

		BeforeEach(func() {
			oldApiKey = "old-key-format-1234"
			newApiKey = "new-key-format-very-long-string-12345678"
			requestHeader = ""

			mockEnv.EXPECT().GetOmsPortalApi().Return(apiUrl)
		})

		Context("when the request succeeds", func() {
			BeforeEach(func() {
				response := map[string]string{
					"apiKey": newApiKey,
				}
				responseBody, _ = json.Marshal(response)

				mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
					func(req *http.Request) (*http.Response, error) {
						getUrl = *req.URL
						requestHeader = req.Header.Get("X-API-Key")
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewReader(responseBody)),
						}, nil
					})
			})

			It("returns the new API key", func() {
				result, err := client.GetApiKeyByHeader(oldApiKey)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(newApiKey))
				Expect(getUrl.String()).To(Equal("fake-portal.com/key"))
				Expect(requestHeader).To(Equal(oldApiKey))
			})

			It("sends the old API key in the X-API-Key header", func() {
				_, err := client.GetApiKeyByHeader(oldApiKey)
				Expect(err).NotTo(HaveOccurred())
				Expect(requestHeader).To(Equal(oldApiKey))
			})
		})

		Context("when the HTTP request fails", func() {
			BeforeEach(func() {
				mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
					func(req *http.Request) (*http.Response, error) {
						return nil, errors.New("network error")
					})
			})

			It("returns an error", func() {
				_, err := client.GetApiKeyByHeader(oldApiKey)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to send request"))
				Expect(err.Error()).To(ContainSubstring("network error"))
			})
		})

		Context("when the server returns an error status code", func() {
			BeforeEach(func() {
				errorResponse := "Unauthorized"
				responseBody = []byte(errorResponse)

				mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
					func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusUnauthorized,
							Body:       io.NopCloser(bytes.NewReader(responseBody)),
						}, nil
					})
			})

			It("returns an error with the status code", func() {
				_, err := client.GetApiKeyByHeader(oldApiKey)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unexpected response status: 401"))
				Expect(err.Error()).To(ContainSubstring("Unauthorized"))
			})
		})

		Context("when the response body is not valid JSON", func() {
			BeforeEach(func() {
				responseBody = []byte("invalid json {")

				mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
					func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewReader(responseBody)),
						}, nil
					})
			})

			It("returns an error", func() {
				_, err := client.GetApiKeyByHeader(oldApiKey)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to decode response"))
			})
		})

		Context("when the response is missing the apiKey field", func() {
			BeforeEach(func() {
				response := map[string]string{
					"someOtherField": "value",
				}
				responseBody, _ = json.Marshal(response)

				mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
					func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewReader(responseBody)),
						}, nil
					})
			})

			It("returns an empty string", func() {
				result, err := client.GetApiKeyByHeader(oldApiKey)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(""))
			})
		})

		Context("when the server returns 404", func() {
			BeforeEach(func() {
				errorResponse := "Not Found"
				responseBody = []byte(errorResponse)

				mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
					func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusNotFound,
							Body:       io.NopCloser(bytes.NewReader(responseBody)),
						}, nil
					})
			})

			It("returns an error with the status code", func() {
				_, err := client.GetApiKeyByHeader(oldApiKey)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unexpected response status: 404"))
			})
		})

		Context("when the server returns 500", func() {
			BeforeEach(func() {
				errorResponse := "Internal Server Error"
				responseBody = []byte(errorResponse)

				mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
					func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: http.StatusInternalServerError,
							Body:       io.NopCloser(bytes.NewReader(responseBody)),
						}, nil
					})
			})

			It("returns an error with the status code", func() {
				_, err := client.GetApiKeyByHeader(oldApiKey)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unexpected response status: 500"))
			})
		})
	})
})
