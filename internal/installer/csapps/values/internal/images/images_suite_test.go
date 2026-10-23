// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package images_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestImages(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Images Suite")
}
