// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package imageref_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestImageref(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Imageref Suite")
}
