// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"path/filepath"

	"github.com/codesphere-cloud/oms/internal/env"
	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("K0sctl", func() {
	It("reuses the cached binary for an unpinned repeated install", func() {
		mockEnv := env.NewMockEnv(GinkgoT())
		mockHTTP := portal.NewMockHttp(GinkgoT())
		mockFileWriter := util.NewMockFileIO(GinkgoT())
		cacheDir := GinkgoT().TempDir()
		cachedPath := filepath.Join(cacheDir, "k0sctl")

		mockEnv.EXPECT().GetOmsCacheDir().Return(cacheDir, nil)
		mockFileWriter.EXPECT().Exists(cachedPath).Return(true)

		k0sctl := installer.NewK0sctl(mockHTTP, mockEnv, mockFileWriter)
		path, err := k0sctl.Download("", false, false)

		Expect(err).NotTo(HaveOccurred())
		Expect(path).To(Equal(cachedPath))
	})
})
