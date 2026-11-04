// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"os"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ReadPackageBaseImage", func() {
	var (
		pm     *installer.MockPackageManager
		fileIO *util.MockFileIO
	)

	BeforeEach(func() {
		pm = installer.NewMockPackageManager(GinkgoT())
		fileIO = util.NewMockFileIO(GinkgoT())
	})

	readIndex := func(data string, err error) {
		pm.EXPECT().FileIO().Return(fileIO)
		pm.EXPECT().GetDependencyPath("bom-index.json").Return("deps/bom-index.json")
		fileIO.EXPECT().ReadFile("deps/bom-index.json").Return([]byte(data), err).Once()
	}

	It("rejects an empty base image before reading the BOM", func() {
		_, err := installer.ReadPackageBaseImage(pm, "")
		Expect(err).To(MatchError("baseimage not specified"))
	})

	It("prefers the named codesphere image from the index and preserves its build tag", func() {
		readIndex(`{"version":"v3","components":[
			{"name":"other","version":"2","containerImages":{"workspace":{"ref":"example.com/wrong:v1"}}},
			{"name":"codesphere","version":"2","containerImages":{
				"workspace":{"ref":"example.com/workspace:codesphere-v2","digest":"sha256:abc"}}}
		]}`, nil)

		ref, err := installer.ReadPackageBaseImage(pm, "workspace")
		Expect(err).NotTo(HaveOccurred())
		Expect(ref).To(Equal("example.com/workspace:codesphere-v2"))
	})

	It("falls back to the legacy BOM when the index is absent", func() {
		readIndex("", os.ErrNotExist)
		pm.EXPECT().GetDependencyPath("bom.json").Return("deps/bom.json")
		fileIO.EXPECT().ReadFile("deps/bom.json").Return([]byte(`{
			"components":{"codesphere":{"containerImages":{"workspace":"example.com/workspace:v1"}}}
		}`), nil).Once()

		ref, err := installer.ReadPackageBaseImage(pm, "workspace")
		Expect(err).NotTo(HaveOccurred())
		Expect(ref).To(Equal("example.com/workspace:v1"))
	})

	It("reports a missing codesphere component without falling back", func() {
		readIndex(`{"version":"v3","components":[{"name":"other","version":"2"}]}`, nil)

		_, err := installer.ReadPackageBaseImage(pm, "workspace")
		Expect(err).To(MatchError(ContainSubstring("codesphere component not found in BOM index")))
	})

	It("reports a missing image without falling back", func() {
		readIndex(`{"version":"v3","components":[{"name":"codesphere","version":"2"}]}`, nil)

		_, err := installer.ReadPackageBaseImage(pm, "workspace")
		Expect(err).To(MatchError("baseimage workspace not found in package BOM"))
	})

	It("rejects a malformed index without falling back", func() {
		readIndex(`{"version":`, nil)

		_, err := installer.ReadPackageBaseImage(pm, "workspace")
		Expect(err).To(MatchError(ContainSubstring("failed to parse JSON BOM index")))
	})

	It("rejects an unsupported index version without falling back", func() {
		readIndex(`{"version":"v4"}`, nil)

		_, err := installer.ReadPackageBaseImage(pm, "workspace")
		Expect(err).To(MatchError(ContainSubstring(`unsupported BOM index version "v4"`)))
	})

	It("rejects an unsupported component version without falling back", func() {
		readIndex(`{"version":"v3","components":[{"name":"codesphere","version":"3"}]}`, nil)

		_, err := installer.ReadPackageBaseImage(pm, "workspace")
		Expect(err).To(MatchError(ContainSubstring(`unsupported BOM component "codesphere" version "3"`)))
	})

	It("preserves index read errors without falling back", func() {
		readIndex("", os.ErrPermission)

		_, err := installer.ReadPackageBaseImage(pm, "workspace")
		Expect(err).To(MatchError(ContainSubstring("failed to read BOM index")))
		Expect(err).To(MatchError(ContainSubstring(os.ErrPermission.Error())))
	})
})
