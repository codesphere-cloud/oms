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
	DescribeTable("image missing from the BOM -> error",
		func(name string) {
			imgs := defaultTestImages()
			delete(imgs, name)

			_, err := codesphere.Values(files.CodesphereConfig{}, imgs, noRegistry)

			Expect(err).To(MatchError(ContainSubstring(name + " not found")))
		},
		Entry("server", "workspace-server"),
		Entry("VPN", "vpn"),
	)
})
