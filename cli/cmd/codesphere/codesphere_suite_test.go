// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package codesphere_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCodesphere(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Codesphere Cmd Suite")
}
