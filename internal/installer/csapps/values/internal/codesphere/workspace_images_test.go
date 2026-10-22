// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Workspace images", func() {
	It("BOM without the workspace server -> error", func() {
		imgs := testImages(nil)
		delete(imgs, "workspace-server")

		_, err := input{images: imgs}.values()

		Expect(err).To(MatchError(ContainSubstring("workspace-server not found")))
	})
})
