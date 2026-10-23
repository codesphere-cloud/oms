// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal_test

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/portal"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

var _ = Describe("EnsureCodespherePackage", func() {
	const (
		version  = "codesphere-v1.95.0"
		artifact = "installer-lite.tar.gz"
	)

	var (
		p       *portal.MockPortal
		workdir string
		build   portal.Build
	)

	BeforeEach(func() {
		p = portal.NewMockPortal(GinkgoT())
		workdir = filepath.Join(GinkgoT().TempDir(), "oms-workdir")
		build = portal.Build{Version: version, Hash: "abc123", Artifacts: []portal.Artifact{{Filename: artifact}}}
	})
	It("delegates an existing archive to the shared downloader and verifies only afterwards", func() {
		p.EXPECT().GetBuild(portal.CodesphereProduct, version, "").Return(build, nil)
		Expect(os.MkdirAll(workdir, 0755)).To(Succeed())
		cached := filepath.Join(workdir, build.BuildPackageFilename(artifact))

		const contents = "existing package"
		Expect(os.WriteFile(cached, []byte(contents), 0644)).To(Succeed())

		downloadFinished := false

		p.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, build, mock.Anything, len(contents), false).Run(func(_ portal.Product, _ portal.Build, _ io.Writer, _ int, _ bool) {
			downloadFinished = true
		}).Return(nil).Once()
		p.EXPECT().VerifyBuildArtifactDownload(mock.Anything, build).Run(func(_ io.Reader, _ portal.Build) {
			Expect(downloadFinished).To(BeTrue())
		}).Return(nil).Once()
		result, err := portal.EnsureCodespherePackage(p, workdir, version, "", artifact, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(cached))
	})
	DescribeTable("handles a completed cached archive through the HTTP downloader", func(status int, contents string, withChecksum, succeeds bool) {
		const complete = "package contents"
		if withChecksum {
			sum := md5.Sum([]byte(complete))
			build.Artifacts[0].Md5Sum = hex.EncodeToString(sum[:])
		}

		Expect(os.MkdirAll(workdir, 0755)).To(Succeed())
		cached := filepath.Join(workdir, build.BuildPackageFilename(artifact))
		Expect(os.WriteFile(cached, []byte(contents), 0600)).To(Succeed())

		downloadRequests := 0

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()

			switch r.URL.Path {
			case "/health":
				w.Header().Set("X-Service-Name", "oms-portal")
				w.WriteHeader(http.StatusOK)
			case "/packages/codesphere":
				Expect(json.NewEncoder(w).Encode(portal.Builds{Builds: []portal.Build{build}})).To(Succeed())
			case "/packages/codesphere/download":
				downloadRequests++

				Expect(r.Header.Get("Range")).To(Equal(fmt.Sprintf("bytes=%d-", len(contents))))

				if status == http.StatusRequestedRangeNotSatisfiable {
					w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(complete)))
					http.Error(w, "range not satisfiable", status)
				} else {
					w.WriteHeader(status)
				}
			default:
				Fail("unexpected portal request: " + r.URL.Path)
			}
		}))
		defer server.Close()

		GinkgoT().Setenv("OMS_PORTAL_API", server.URL)
		GinkgoT().Setenv("OMS_PORTAL_API_KEY", "test-key")

		client := portal.NewPortalClient()

		result, err := portal.EnsureCodespherePackage(client, workdir, version, "", artifact, true)
		if succeeds {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(cached))
		} else {
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeEmpty())

			if withChecksum {
				Expect(err.Error()).To(ContainSubstring("invalid md5Sum"))
			}
		}

		Expect(downloadRequests).To(Equal(1))

		remaining, err := os.ReadFile(cached)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(remaining)).To(Equal(contents))
	},
		Entry("empty 206 response verifies the complete file", http.StatusPartialContent, "package contents", true, true),
		Entry("416 response verifies the complete file", http.StatusRequestedRangeNotSatisfiable, "package contents", true, true),
		Entry("416 response rejects a corrupt file", http.StatusRequestedRangeNotSatisfiable, "corrupt contents", true, false),
		Entry("416 response cannot accept a file without a checksum", http.StatusRequestedRangeNotSatisfiable, "package contents", false, false),
	)

	It("downloads and verifies a missing package", func() {
		p.EXPECT().GetBuild(portal.CodesphereProduct, version, "").Return(build, nil).Once()
		p.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, build, mock.Anything, 0, true).Run(func(_ portal.Product, _ portal.Build, out io.Writer, _ int, _ bool) {
			_, err := out.Write([]byte("package contents"))
			Expect(err).NotTo(HaveOccurred())
		}).Return(nil)
		p.EXPECT().VerifyBuildArtifactDownload(mock.Anything, build).Return(nil).Once()
		result, err := portal.EnsureCodespherePackage(p, workdir, version, "", artifact, true)
		Expect(err).NotTo(HaveOccurred())
		contents, err := os.ReadFile(result)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("package contents"))
	})
	It("resumes an interrupted download from its existing byte offset", func() {
		const prefix = "partial "

		p.EXPECT().GetBuild(portal.CodesphereProduct, version, "").Return(build, nil).Twice()
		p.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, build, mock.Anything, 0, false).Run(func(_ portal.Product, _ portal.Build, out io.Writer, _ int, _ bool) {
			_, err := out.Write([]byte(prefix))
			Expect(err).NotTo(HaveOccurred())
		}).Return(fmt.Errorf("interrupted")).Once()
		_, err := portal.EnsureCodespherePackage(p, workdir, version, "", artifact, false)
		Expect(err).To(MatchError(ContainSubstring("interrupted")))
		interrupted, err := os.ReadFile(filepath.Join(workdir, build.BuildPackageFilename(artifact)))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(interrupted)).To(Equal(prefix))

		p.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, build, mock.Anything, len(prefix), false).Run(func(_ portal.Product, _ portal.Build, out io.Writer, _ int, _ bool) {
			_, err := out.Write([]byte("contents"))
			Expect(err).NotTo(HaveOccurred())
		}).Return(nil).Once()
		p.EXPECT().VerifyBuildArtifactDownload(mock.Anything, build).Run(func(in io.Reader, _ portal.Build) {
			contents, err := io.ReadAll(in)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(contents)).To(Equal("partial contents"))
		}).Return(nil).Once()
		result, err := portal.EnsureCodespherePackage(p, workdir, version, "", artifact, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(filepath.Join(workdir, build.BuildPackageFilename(artifact))))
	})

	It("returns no usable package when verification fails", func() {
		p.EXPECT().GetBuild(portal.CodesphereProduct, version, "").Return(build, nil)
		p.EXPECT().DownloadBuildArtifact(portal.CodesphereProduct, build, mock.Anything, 0, false).Return(nil)
		p.EXPECT().VerifyBuildArtifactDownload(mock.Anything, build).Return(fmt.Errorf("checksum mismatch"))
		result, err := portal.EnsureCodespherePackage(p, workdir, version, "", artifact, false)
		Expect(err).To(MatchError(ContainSubstring("checksum mismatch")))
		Expect(result).To(BeEmpty())
	})
	It("propagates lookup errors without creating a cached package", func() {
		p.EXPECT().GetBuild(portal.CodesphereProduct, version, "abc").Return(portal.Build{}, fmt.Errorf("unavailable"))
		_, err := portal.EnsureCodespherePackage(p, workdir, version, "abc", artifact, false)
		Expect(err).To(MatchError(ContainSubstring("unavailable")))
		Expect(workdir).NotTo(BeADirectory())
	})
})
