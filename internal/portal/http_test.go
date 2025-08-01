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

var _ = Describe("PortalClient", func() {
	var (
		client         portal.PortalClient
		mockEnv        env.MockEnv
		mockHttpClient portal.MockHttpClient
		status         int
		apiUrl         string
		getUrl         url.URL
		getResponse    []byte
	)
	BeforeEach(func() {
		client = portal.PortalClient{
			Env:        &mockEnv,
			HttpClient: &mockHttpClient,
		}
		status = http.StatusOK
		apiUrl = "fake-portal.com"

		mockEnv.EXPECT().GetOmsPortalApi().Return(apiUrl)
		mockEnv.EXPECT().GetOmsPortalApiKey().Return("fake-api-key", nil)
	})
	Describe("Get", func() {
		BeforeEach(func() {
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
				_, status, err := client.Get("/api/fake")
				Expect(status).To(Equal(status))
				Expect(err).NotTo(HaveOccurred())
				Expect(getUrl.String()).To(Equal("fake-portal.com/api/fake"))
			})
		})

		Context("when path does not with a /", func() {
			It("Executes a request against the right URL", func() {
				_, status, err := client.Get("api/fake")
				Expect(status).To(Equal(status))
				Expect(err).NotTo(HaveOccurred())
				Expect(getUrl.String()).To(Equal("fake-portal.com/api/fake"))
			})
		})
	})

	Describe("ListCodespherePackages", func() {
		Context("when the request suceeds", func() {
			var expectedResult portal.CodesphereBuilds
			BeforeEach(func() {
				firstBuild, _ := time.Parse("2006-01-02", "2025-04-02")
				lastBuild, _ := time.Parse("2006-01-02", "2025-05-01")

				getPackagesResponse := portal.CodesphereBuilds{
					Builds: []portal.CodesphereBuild{
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

				expectedResult = portal.CodesphereBuilds{
					Builds: []portal.CodesphereBuild{
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
				packages, err := client.ListCodespherePackages()
				Expect(err).NotTo(HaveOccurred())
				Expect(packages).To(Equal(expectedResult))
				Expect(getUrl.String()).To(Equal("fake-portal.com/packages/codesphere"))
			})
		})
	})
})
