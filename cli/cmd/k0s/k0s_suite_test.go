// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package k0s_test

import (
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestK0s(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "K0s Command Suite")
}
