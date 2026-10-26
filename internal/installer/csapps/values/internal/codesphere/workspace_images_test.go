// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer/csapps/values/internal/codesphere"
	"github.com/codesphere-cloud/oms/internal/installer/files"
)

var _ = Describe("Workspace images", func() {
	It("BOM without the workspace server -> error", func() {
		imgs := defaultTestImages()
		delete(imgs, "workspace-server")

		_, err := codesphere.Values(files.CodesphereConfig{}, imgs, defaultTestGateways(), noMetalLB, noRegistry)

		Expect(err).To(MatchError(ContainSubstring("workspace-server not found")))
	})

	It("BOM without the VPN -> error", func() {
		imgs := defaultTestImages()
		delete(imgs, "vpn")

		_, err := codesphere.Values(files.CodesphereConfig{}, imgs, defaultTestGateways(), noMetalLB, noRegistry)

		Expect(err).To(MatchError(ContainSubstring("vpn not found")))
	})
})
