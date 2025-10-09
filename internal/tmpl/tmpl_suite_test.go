// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package tmpl_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTmpl(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tmpl Suite")
}