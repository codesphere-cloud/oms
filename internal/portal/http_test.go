// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal_test

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HttpWrapper", func() {
	var (
		httpWrapper    *portal.HttpWrapper
		mockHttpClient *portal.MockHttpClient
		testUrl        string
		testMethod     string
		testBody       io.Reader
		response       *http.Response
		responseBody   []byte
		responseError  error
	)

	BeforeEach(func() {
		mockHttpClient = portal.NewMockHttpClient(GinkgoT())
		httpWrapper = &portal.HttpWrapper{
			HttpClient: mockHttpClient,
		}
		testUrl = "https://test.example.com/api/endpoint"
		testMethod = "GET"
		testBody = nil
		responseBody = []byte("test response body")
		responseError = nil
	})

	AfterEach(func() {
		mockHttpClient.AssertExpectations(GinkgoT())
	})

	Describe("NewHttpWrapper", func() {
		It("creates a new HttpWrapper with default client", func() {
			wrapper := portal.NewHttpWrapper()
			Expect(wrapper).ToNot(BeNil())
			Expect(wrapper.HttpClient).ToNot(BeNil())
		})
	})

	Describe("Request", func() {
		Context("when making a successful GET request", func() {
			JustBeforeEach(func() {
				response = &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(responseBody)),
				}

				mockHttpClient.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
					return req.URL.String() == testUrl && req.Method == testMethod
				})).Return(response, responseError)
			})

			It("returns the response body", func() {
				result, err := httpWrapper.Request(testUrl, testMethod, testBody)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(responseBody))
			})
		})

		Context("when making a POST request with body", func() {
			BeforeEach(func() {
				testMethod = "POST"
				testBody = strings.NewReader("test post data")
			})

			JustBeforeEach(func() {
				response = &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(responseBody)),
				}

				mockHttpClient.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
					return req.URL.String() == testUrl && req.Method == testMethod
				})).Return(response, responseError)
			})

			It("returns the response body", func() {
				result, err := httpWrapper.Request(testUrl, testMethod, testBody)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(responseBody))
			})
		})

		Context("when the HTTP client returns an error", func() {
			BeforeEach(func() {
				responseError = errors.New("network connection failed")
			})

			JustBeforeEach(func() {
				response = &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(responseBody)),
				}

				mockHttpClient.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
					return req.URL.String() == testUrl && req.Method == testMethod
				})).Return(response, responseError)
			})

			It("returns an error", func() {
				result, err := httpWrapper.Request(testUrl, testMethod, testBody)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to send request"))
				Expect(err.Error()).To(ContainSubstring("network connection failed"))
				Expect(result).To(Equal([]byte{}))
			})
		})

		Context("when the response status code indicates an error", func() {
			JustBeforeEach(func() {
				response = &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewReader(responseBody)),
				}

				mockHttpClient.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
					return req.URL.String() == testUrl && req.Method == testMethod
				})).Return(response, responseError)
			})

			It("returns an error", func() {
				result, err := httpWrapper.Request(testUrl, testMethod, testBody)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed request with status: 400"))
				Expect(result).To(Equal([]byte{}))
			})
		})

		Context("when reading the response body fails", func() {
			JustBeforeEach(func() {
				response = &http.Response{
					StatusCode: http.StatusOK,
					Body:       &FailingReader{},
				}

				mockHttpClient.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
					return req.URL.String() == testUrl && req.Method == testMethod
				})).Return(response, responseError)
			})

			It("returns an error", func() {
				result, err := httpWrapper.Request(testUrl, testMethod, testBody)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to read response body"))
				Expect(result).To(Equal([]byte{}))
			})
		})
	})

	Describe("Get", func() {
		Context("when making a successful request", func() {
			JustBeforeEach(func() {
				response = &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(responseBody)),
				}

				mockHttpClient.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
					return req.URL.String() == testUrl && req.Method == "GET"
				})).Return(response, responseError)
			})

			It("returns the response body", func() {
				result, err := httpWrapper.Get(testUrl)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(responseBody))
			})
		})

		Context("when the request fails", func() {
			BeforeEach(func() {
				responseError = errors.New("DNS resolution failed")
			})

			JustBeforeEach(func() {
				response = &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(responseBody)),
				}

				mockHttpClient.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
					return req.URL.String() == testUrl && req.Method == "GET"
				})).Return(response, responseError)
			})

			It("returns an error", func() {
				result, err := httpWrapper.Get(testUrl)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to send request"))
				Expect(err.Error()).To(ContainSubstring("DNS resolution failed"))
				Expect(result).To(Equal([]byte{}))
			})
		})
	})

	Describe("Download", func() {
		var (
			testWriter      *TestWriter
			downloadContent string
			quiet           bool
		)

		BeforeEach(func() {
			testWriter = NewTestWriter()
			downloadContent = "file content to download"
			quiet = false
		})

		Context("when downloading successfully", func() {
			JustBeforeEach(func() {
				response = &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(downloadContent)),
				}

				mockHttpClient.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
					return req.URL.String() == testUrl && req.Method == "GET"
				})).Return(response, responseError)
			})

			It("downloads content and shows progress", func() {
				// Capture log output to verify progress is shown
				var logBuf bytes.Buffer
				prev := log.Writer()
				log.SetOutput(&logBuf)
				defer log.SetOutput(prev)

				err := httpWrapper.Download(testUrl, testWriter, quiet)
				Expect(err).ToNot(HaveOccurred())
				Expect(testWriter.String()).To(Equal(downloadContent))
				Expect(logBuf.String()).To(ContainSubstring("Downloading..."))
				Expect(logBuf.String()).To(ContainSubstring("Download finished successfully"))
			})

			It("downloads content without showing progress", func() {
				quiet = true // Set quiet to true to suppress progress output

				var logBuf bytes.Buffer
				prev := log.Writer()
				log.SetOutput(&logBuf)
				defer log.SetOutput(prev)

				err := httpWrapper.Download(testUrl, testWriter, quiet)
				Expect(err).ToNot(HaveOccurred())
				Expect(testWriter.String()).To(Equal(downloadContent))
				Expect(logBuf.String()).To(Not(ContainSubstring("Downloading...")))
				Expect(logBuf.String()).To(ContainSubstring("Download finished successfully"))
			})
		})

		Context("when the HTTP client returns an error", func() {
			BeforeEach(func() {
				responseError = errors.New("connection timeout")
			})

			JustBeforeEach(func() {
				response = &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(downloadContent)),
				}

				mockHttpClient.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
					return req.URL.String() == testUrl && req.Method == "GET"
				})).Return(response, responseError)
			})

			It("returns an error", func() {
				err := httpWrapper.Download(testUrl, testWriter, quiet)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to send request"))
				Expect(err.Error()).To(ContainSubstring("connection timeout"))
				Expect(testWriter.String()).To(BeEmpty())
			})
		})

		Context("when the server returns an error status", func() {
			JustBeforeEach(func() {
				response = &http.Response{
					StatusCode: http.StatusForbidden,
					Body:       io.NopCloser(strings.NewReader("Access denied")),
				}

				mockHttpClient.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
					return req.URL.String() == testUrl && req.Method == "GET"
				})).Return(response, responseError)
			})

			It("returns an error", func() {
				err := httpWrapper.Download(testUrl, testWriter, quiet)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get body: 403"))
				Expect(testWriter.String()).To(BeEmpty())
			})
		})

		Context("when copying the response body fails", func() {
			JustBeforeEach(func() {
				response = &http.Response{
					StatusCode: http.StatusOK,
					Body:       &FailingReader{},
				}

				mockHttpClient.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
					return req.URL.String() == testUrl && req.Method == "GET"
				})).Return(response, responseError)
			})

			It("returns an error", func() {
				err := httpWrapper.Download(testUrl, testWriter, quiet)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to copy response body to file"))
				Expect(err.Error()).To(ContainSubstring("simulated read error"))
			})
		})

		Context("when the writer fails", func() {
			JustBeforeEach(func() {
				response = &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(downloadContent)),
				}

				mockHttpClient.EXPECT().Do(mock.MatchedBy(func(req *http.Request) bool {
					return req.URL.String() == testUrl && req.Method == "GET"
				})).Return(response, responseError)
			})

			It("handles write errors gracefully", func() {
				// Use a failing writer
				failingWriter := &FailingWriter{}

				err := httpWrapper.Download(testUrl, failingWriter, quiet)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to copy response body to file"))
			})
		})
	})

	Describe("GetApiKeyByHeader", func() {
		var (
			oldApiKey      string
			keyId          string
			expectedNewKey string
			responseBody   []byte
			requestHeader  string
		)

		BeforeEach(func() {
			oldApiKey = "old-key-format-1234"
			keyId = "test-key-id-12345"
			expectedNewKey = keyId + oldApiKey
			requestHeader = ""

			mockEnv.EXPECT().GetOmsPortalApi().Return(apiUrl)
		})

		Context("when the request succeeds", func() {
			BeforeEach(func() {
				response := map[string]string{
					"keyId": keyId,
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
				Expect(result).To(Equal(expectedNewKey))
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

		Context("when the response is missing the keyId field", func() {
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

			It("returns only the old key", func() {
				result, err := client.GetApiKeyByHeader(oldApiKey)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(oldApiKey))
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

// Helper types for testing
type TestWriter struct {
	bytes.Buffer
}

var _ io.Writer = (*TestWriter)(nil)

func NewTestWriter() *TestWriter {
	return &TestWriter{}
}

type FailingReader struct{}

func (fr *FailingReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated read error")
}

func (fr *FailingReader) Close() error {
	return nil
}

type FailingWriter struct{}

func (fw *FailingWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("simulated write error")
}
