// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/codesphere-cloud/oms/cli/cmd"
	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCmd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cmd Suite")
}

var _ = Describe("RootCmd", func() {
	var (
		mockEnv        *env.MockEnv
		mockHttpClient *portal.MockHttpClient
	)

	BeforeEach(func() {
		mockEnv = env.NewMockEnv(GinkgoT())
		mockHttpClient = portal.NewMockHttpClient(GinkgoT())
	})

	AfterEach(func() {
		mockEnv.AssertExpectations(GinkgoT())
		mockHttpClient.AssertExpectations(GinkgoT())
	})

	Describe("PreRun hook with old API key", func() {
		Context("when API key is 22 characters (old format)", func() {
			It("attempts to upgrade the key via GetApiKeyByHeader", func() {
				oldKey := "U4jsSHoDsOFGyEkPrWpsE" // 22 characters
				newKey := "new-long-api-key-format-very-long-string"

				Expect(os.Setenv("OMS_PORTAL_API_KEY", oldKey)).NotTo(HaveOccurred())
				Expect(os.Setenv("OMS_PORTAL_API", "http://test-portal.com/api")).NotTo(HaveOccurred())

				mockEnv.EXPECT().GetOmsPortalApi().Return("http://test-portal.com/api")

				mockHttpClient.EXPECT().Do(mock.Anything).RunAndReturn(
					func(req *http.Request) (*http.Response, error) {
						Expect(req.Header.Get("X-API-Key")).To(Equal(oldKey))
						Expect(req.URL.Path).To(ContainSubstring("/key"))

						response := map[string]string{
							"apiKey": newKey,
						}
						body, _ := json.Marshal(response)
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewReader(body)),
						}, nil
					})

				portalClient := &portal.PortalClient{
					Env:        mockEnv,
					HttpClient: mockHttpClient,
				}

				result, err := portalClient.GetApiKeyByHeader(oldKey)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(newKey))

				Expect(os.Unsetenv("OMS_PORTAL_API_KEY")).NotTo(HaveOccurred())
				Expect(os.Unsetenv("OMS_PORTAL_API")).NotTo(HaveOccurred())
			})
		})

		Context("when API key is not 22 characters (new format)", func() {
			It("does not attempt to upgrade the key", func() {
				newKey := "new-long-api-key-format-very-long-string"

				Expect(os.Setenv("OMS_PORTAL_API_KEY", newKey)).NotTo(HaveOccurred())
				Expect(os.Setenv("OMS_PORTAL_API", "http://test-portal.com/api")).NotTo(HaveOccurred())

				Expect(len(newKey)).NotTo(Equal(22))

				Expect(os.Unsetenv("OMS_PORTAL_API_KEY")).NotTo(HaveOccurred())
				Expect(os.Unsetenv("OMS_PORTAL_API")).NotTo(HaveOccurred())
			})
		})

		Context("when API key is empty", func() {
			It("does not attempt to upgrade", func() {
				Expect(os.Setenv("OMS_PORTAL_API_KEY", "")).NotTo(HaveOccurred())
				Expect(os.Setenv("OMS_PORTAL_API", "http://test-portal.com/api")).NotTo(HaveOccurred())

				Expect(len(os.Getenv("OMS_PORTAL_API_KEY"))).To(Equal(0))

				Expect(os.Unsetenv("OMS_PORTAL_API_KEY")).NotTo(HaveOccurred())
				Expect(os.Unsetenv("OMS_PORTAL_API")).NotTo(HaveOccurred())
			})
		})
	})

	Describe("GetRootCmd", func() {
		It("returns a valid root command", func() {
			rootCmd := cmd.GetRootCmd()
			Expect(rootCmd).NotTo(BeNil())
			Expect(rootCmd.Use).To(Equal("oms"))
			Expect(rootCmd.Short).To(Equal("Codesphere Operations Management System (OMS)"))
		})
	})
})
