// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal_test

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
		apiUrl         string
		getUrl         url.URL
		headers        http.Header
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
		apiUrl = "fake-portal.com"
	})

	AfterEach(func() {
		mockEnv.AssertExpectations(GinkgoT())
		mockHttpClient.AssertExpectations(GinkgoT())
	})

	Describe("GetBody", func() {
		JustBeforeEach(func() {
			mockEnv.EXPECT().GetOmsPortalApi().Return(apiUrl)
			mockEnv.EXPECT().GetOmsPortalApiKey().Return(apiKey, apiKeyErr)
		})

		Context("when path starts with a /", func() {
			BeforeEach(func() {
				mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
					func(req *http.Request) (*http.Response, error) {
						getUrl = *req.URL
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewReader([]byte{})),
						}, nil
					})
			})

			It("executes a request against the right URL", func() {
				_, _, err := client.GetBody("/api/fake")
				Expect(err).NotTo(HaveOccurred())
				Expect(getUrl.String()).To(Equal("fake-portal.com/api/fake"))
			})
		})

		Context("when path does not start with a /", func() {
			BeforeEach(func() {
				mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
					func(req *http.Request) (*http.Response, error) {
						getUrl = *req.URL
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewReader([]byte{})),
						}, nil
					})
			})

			It("executes a request against the right URL", func() {
				_, _, err := client.GetBody("api/fake")
				Expect(err).NotTo(HaveOccurred())
				Expect(getUrl.String()).To(Equal("fake-portal.com/api/fake"))
			})
		})

		Context("when OMS_PORTAL_API_KEY is unset", func() {
			BeforeEach(func() {
				apiKey = ""
				apiKeyErr = errors.New("fake-error")
			})

			It("returns an error", func() {
				_, _, err := client.GetBody("/api/fake")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("fake-error"))
			})
		})
	})

	Describe("ListCodespherePackages", func() {
		Context("when the request succeeds", func() {
			BeforeEach(func() {
				firstBuild, _ := time.Parse("2006-01-02", "2025-04-02")
				lastBuild, _ := time.Parse("2006-01-02", "2025-05-01")

				response := portal.Builds{
					Builds: []portal.Build{
						{Hash: "lastBuild", Date: lastBuild},
						{Hash: "firstBuild", Date: firstBuild},
					},
				}
				responseBody, _ := json.Marshal(response)

				mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
					func(req *http.Request) (*http.Response, error) {
						getUrl = *req.URL
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewReader(responseBody)),
						}, nil
					})

				mockEnv.EXPECT().GetOmsPortalApi().Return(apiUrl)
				mockEnv.EXPECT().GetOmsPortalApiKey().Return(apiKey, apiKeyErr)
			})

			It("returns the builds ordered by date", func() {
				firstBuild, _ := time.Parse("2006-01-02", "2025-04-02")
				lastBuild, _ := time.Parse("2006-01-02", "2025-05-01")

				packages, err := client.ListBuilds(portal.CodesphereProduct)
				Expect(err).NotTo(HaveOccurred())
				Expect(packages.Builds).To(HaveLen(2))
				Expect(packages.Builds[0].Hash).To(Equal("firstBuild"))
				Expect(packages.Builds[0].Date).To(Equal(firstBuild))
				Expect(packages.Builds[1].Hash).To(Equal("lastBuild"))
				Expect(packages.Builds[1].Date).To(Equal(lastBuild))
				Expect(getUrl.String()).To(Equal("fake-portal.com/packages/codesphere"))
			})
		})
	})

	Describe("DownloadBuildArtifact", func() {
		var build portal.Build

		BeforeEach(func() {
			buildDate, _ := time.Parse("2006-01-02", "2025-05-01")
			build = portal.Build{Date: buildDate}

			mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
				func(req *http.Request) (*http.Response, error) {
					getUrl = *req.URL
					headers = req.Header
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader([]byte("fake-file-contents"))),
					}, nil
				})

			mockEnv.EXPECT().GetOmsPortalApi().Return(apiUrl)
			mockEnv.EXPECT().GetOmsPortalApiKey().Return(apiKey, apiKeyErr)
		})

		It("downloads the build", func() {
			fakeWriter := NewFakeWriter()
			err := client.DownloadBuildArtifact(product, build, fakeWriter, 0, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeWriter.String()).To(Equal("fake-file-contents"))
			Expect(getUrl.String()).To(Equal("fake-portal.com/packages/codesphere/download"))
		})

		It("resumes the build", func() {
			fakeWriter := NewFakeWriter()
			err := client.DownloadBuildArtifact(product, build, fakeWriter, 42, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(headers.Get("Range")).To(Equal("bytes=42-"))
		})
	})

	Describe("VerifyBuildArtifactDownload", func() {
		var testfilePath string
		var testfile *os.File
		var testfileMd5Sum string

		BeforeEach(func() {
			var err error

			tempDir := GinkgoT().TempDir()
			testfilePath = filepath.Join(tempDir, "VerifyBuildArtifactDownload-installer.tar.gz")
			testfile, err = os.Create(testfilePath)
			Expect(err).ToNot(HaveOccurred())

			hash := md5.New()
			_, err = io.Copy(hash, testfile)
			Expect(err).ToNot(HaveOccurred())

			testfileMd5Sum = hex.EncodeToString(hash.Sum(nil))
		})

		It("verifies the build successfully", func() {
			build := portal.Build{
				Artifacts: []portal.Artifact{
					{
						Filename: "installer.tar.gz",
						Md5Sum:   testfileMd5Sum,
					},
				},
			}

			err := client.VerifyBuildArtifactDownload(testfile, build)
			Expect(err).ToNot(HaveOccurred())
		})

		It("failed verification on wrong checksum", func() {
			build := portal.Build{
				Artifacts: []portal.Artifact{
					{
						Filename: "bad-installer.tar.gz",
						Md5Sum:   "anotherchecksum",
					},
				},
			}

			err := client.VerifyBuildArtifactDownload(testfile, build)
			Expect(err).To(HaveOccurred())

			expectedErr := fmt.Sprintf("invalid md5Sum: expected anotherchecksum, but got %s", testfileMd5Sum)
			Expect(err.Error()).To(ContainSubstring(expectedErr))
		})

		It("skipped verification on empty checksum", func() {
			build := portal.Build{
				Artifacts: []portal.Artifact{
					{
						Filename: "installer.tar.gz",
						Md5Sum:   "",
					},
				},
			}

			err := client.VerifyBuildArtifactDownload(testfile, build)
			Expect(err).ToNot(HaveOccurred())
		})

		It("failed verification if file is closed", func() {
			build := portal.Build{
				Artifacts: []portal.Artifact{
					{
						Filename: "installer.tar.gz",
						Md5Sum:   testfileMd5Sum,
					},
				},
			}

			log.Printf("%s", testfile.Name())

			// Close the file before using it for verification
			err := testfile.Close()
			Expect(err).ToNot(HaveOccurred())

			err = client.VerifyBuildArtifactDownload(testfile, build)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to compute checksum"))
		})

	})

	Describe("GetLatestOmsBuild", func() {
		BeforeEach(func() {
			mockEnv.EXPECT().GetOmsPortalApi().Return(apiUrl)
			mockEnv.EXPECT().GetOmsPortalApiKey().Return(apiKey, apiKeyErr)
		})

		Context("when builds are available", func() {
			BeforeEach(func() {
				firstBuild, _ := time.Parse("2006-01-02", "2025-04-02")
				lastBuild, _ := time.Parse("2006-01-02", "2025-05-01")

				response := portal.Builds{
					Builds: []portal.Build{
						{Hash: "firstBuild", Date: firstBuild, Version: "1.42.0"},
						{Hash: "lastBuild", Date: lastBuild, Version: "1.42.1"},
					},
				}
				responseBody, _ := json.Marshal(response)

				mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
					func(req *http.Request) (*http.Response, error) {
						getUrl = *req.URL
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewReader(responseBody)),
						}, nil
					})
			})

			It("returns the latest build", func() {
				lastBuild, _ := time.Parse("2006-01-02", "2025-05-01")
				build, err := client.GetBuild(portal.OmsProduct, "", "")
				Expect(err).NotTo(HaveOccurred())
				Expect(build.Hash).To(Equal("lastBuild"))
				Expect(build.Date).To(Equal(lastBuild))
				Expect(build.Version).To(Equal("1.42.1"))
			})

			It("returns the build matching version", func() {
				lastBuild, _ := time.Parse("2006-01-02", "2025-05-01")
				build, err := client.GetBuild(portal.OmsProduct, "1.42.1", "")
				Expect(err).NotTo(HaveOccurred())
				Expect(build.Hash).To(Equal("lastBuild"))
				Expect(build.Date).To(Equal(lastBuild))
				Expect(build.Version).To(Equal("1.42.1"))
			})

			It("returns the build matching version and hash", func() {
				lastBuild, _ := time.Parse("2006-01-02", "2025-05-01")
				build, err := client.GetBuild(portal.OmsProduct, "1.42.1", "lastBuild")
				Expect(err).NotTo(HaveOccurred())
				Expect(build.Hash).To(Equal("lastBuild"))
				Expect(build.Date).To(Equal(lastBuild))
				Expect(build.Version).To(Equal("1.42.1"))
			})
		})

		Context("when no builds are returned", func() {
			BeforeEach(func() {
				response := portal.Builds{Builds: []portal.Build{}}
				responseBody, _ := json.Marshal(response)

				mockHttpClient.EXPECT().Do(mock.Anything).Return(&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(responseBody)),
				}, nil)
			})

			It("returns an error", func() {
				_, err := client.GetBuild(portal.OmsProduct, "", "")
				Expect(err).To(MatchError("no builds returned"))
			})
		})
	})

	Describe("GetApiKeyId", func() {
		BeforeEach(func() {
			mockEnv.EXPECT().GetOmsPortalApi().Return(apiUrl)
		})

		Context("when the request succeeds", func() {
			BeforeEach(func() {
				response := map[string]string{"keyId": "test-key-id"}
				responseBody, _ := json.Marshal(response)

				mockHttpClient.EXPECT().Do(mock.Anything).Return(&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(responseBody)),
				}, nil)
			})

			It("returns the key ID", func() {
				result, err := client.GetApiKeyId("old-key")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("test-key-id"))
			})
		})

		Context("when the HTTP request fails", func() {
			BeforeEach(func() {
				mockHttpClient.EXPECT().Do(mock.Anything).Return(nil, errors.New("network error"))
			})

			It("returns an error", func() {
				_, err := client.GetApiKeyId("old-key")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("network error"))
			})
		})

		Context("when the server returns an error status", func() {
			BeforeEach(func() {
				mockHttpClient.EXPECT().Do(mock.Anything).Return(&http.Response{
					StatusCode: http.StatusUnauthorized,
					Body:       io.NopCloser(bytes.NewReader([]byte("Unauthorized"))),
				}, nil)
			})

			It("returns an error", func() {
				_, err := client.GetApiKeyId("old-key")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unexpected response status: 401"))
			})
		})
	})

	Describe("RegisterAPIKey", func() {
		var (
			owner, organization, role string
			expiresAt                 time.Time
			responseBody              []byte
		)

		BeforeEach(func() {
			owner = "test-owner"
			organization = "test-org"
			role = "admin"
			expiresAt, _ = time.Parse("2006-01-02", "2026-01-01")

			responseKey := portal.ApiKey{
				KeyID:        "key-123",
				Owner:        owner,
				Organization: organization,
				Role:         role,
				ExpiresAt:    expiresAt,
				ApiKey:       "secret-key-data",
			}
			responseBody, _ = json.Marshal(responseKey)

			mockEnv.EXPECT().GetOmsPortalApi().Return(apiUrl)
			mockEnv.EXPECT().GetOmsPortalApiKey().Return(apiKey, apiKeyErr)
		})

		Context("when registration succeeds", func() {
			BeforeEach(func() {
				mockHttpClient.EXPECT().Do(mock.Anything).Return(&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(responseBody)),
				}, nil)
			})

			It("returns the new API key", func() {
				key, err := client.RegisterAPIKey(owner, organization, role, expiresAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(key.KeyID).To(Equal("key-123"))
				Expect(key.ApiKey).To(Equal("secret-key-data"))
			})
		})
	})

	Describe("RevokeAPIKey", func() {
		Context("when revocation succeeds", func() {
			BeforeEach(func() {
				mockHttpClient.EXPECT().Do(mock.Anything).Return(&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte("{}"))),
				}, nil)

				mockEnv.EXPECT().GetOmsPortalApi().Return(apiUrl)
				mockEnv.EXPECT().GetOmsPortalApiKey().Return(apiKey, apiKeyErr)
			})

			It("completes without error", func() {
				err := client.RevokeAPIKey("key-to-revoke")
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("UpdateAPIKey", func() {
		Context("when update succeeds", func() {
			BeforeEach(func() {
				mockHttpClient.EXPECT().Do(mock.Anything).Return(&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte("{}"))),
				}, nil)

				mockEnv.EXPECT().GetOmsPortalApi().Return(apiUrl)
				mockEnv.EXPECT().GetOmsPortalApiKey().Return(apiKey, apiKeyErr)
			})

			It("completes without error", func() {
				expiresAt, _ := time.Parse("2006-01-02", "2027-01-01")
				err := client.UpdateAPIKey("key-to-update", expiresAt)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("ListAPIKeys", func() {
		Context("when listing succeeds", func() {
			BeforeEach(func() {
				expiresAt, _ := time.Parse("2006-01-02", "2026-01-01")
				keys := []portal.ApiKey{
					{KeyID: "key-1", Owner: "owner-1", Organization: "org-1", Role: "admin", ExpiresAt: expiresAt},
					{KeyID: "key-2", Owner: "owner-2", Organization: "org-2", Role: "viewer", ExpiresAt: expiresAt},
				}
				responseBody, _ := json.Marshal(keys)

				mockHttpClient.EXPECT().Do(mock.Anything).Return(&http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(responseBody)),
				}, nil)

				mockEnv.EXPECT().GetOmsPortalApi().Return(apiUrl)
				mockEnv.EXPECT().GetOmsPortalApiKey().Return(apiKey, apiKeyErr)
			})

			It("returns the list of API keys", func() {
				keys, err := client.ListAPIKeys()
				Expect(err).NotTo(HaveOccurred())
				Expect(keys).To(HaveLen(2))
				Expect(keys[0].KeyID).To(Equal("key-1"))
				Expect(keys[1].KeyID).To(Equal("key-2"))
			})
		})
	})
})

var _ = Describe("truncateHTMLResponse", func() {
	It("returns short non-HTML response unchanged", func() {
		body := `{"error": "not found"}`
		result := portal.TruncateHTMLResponse(body)
		Expect(result).To(Equal(body))
	})

	It("truncates long non-HTML responses", func() {
		body := strings.Repeat("a", 600)
		result := portal.TruncateHTMLResponse(body)
		Expect(result).To(HaveLen(500 + len("... (truncated)")))
		Expect(result).To(HaveSuffix("... (truncated)"))
	})

	It("extracts title from HTML response with DOCTYPE", func() {
		body := `<!DOCTYPE html><html><head><title>502 Bad Gateway</title></head><body>...</body></html>`
		result := portal.TruncateHTMLResponse(body)
		Expect(result).To(Equal("Server says: 502 Bad Gateway"))
	})

	It("extracts title from HTML response starting with html tag", func() {
		body := `<html><head><title>Service Unavailable</title></head><body>...</body></html>`
		result := portal.TruncateHTMLResponse(body)
		Expect(result).To(Equal("Server says: Service Unavailable"))
	})

	It("handles HTML without title tag", func() {
		body := `<!DOCTYPE html><html><body>Error page</body></html>`
		result := portal.TruncateHTMLResponse(body)
		Expect(result).To(Equal("Received HTML response instead of JSON"))
	})

	It("handles HTML with whitespace before DOCTYPE", func() {
		body := `   <!DOCTYPE html><html><head><title>Error</title></head></html>`
		result := portal.TruncateHTMLResponse(body)
		Expect(result).To(Equal("Server says: Error"))
	})
})

var _ = Describe("newConfiguredHttpClient", func() {
	It("creates an HTTP client with configured timeouts", func() {
		client := portal.NewConfiguredHttpClient()

		Expect(client).NotTo(BeNil())
		Expect(client.Timeout).To(Equal(10 * time.Minute))

		transport, ok := client.Transport.(*http.Transport)
		Expect(ok).To(BeTrue())
		Expect(transport.TLSHandshakeTimeout).To(Equal(30 * time.Second))
		Expect(transport.ResponseHeaderTimeout).To(Equal(2 * time.Minute))
		Expect(transport.ExpectContinueTimeout).To(Equal(1 * time.Second))
		Expect(transport.IdleConnTimeout).To(Equal(90 * time.Second))
		Expect(transport.MaxIdleConns).To(Equal(100))
		Expect(transport.MaxIdleConnsPerHost).To(Equal(10))
	})
})
