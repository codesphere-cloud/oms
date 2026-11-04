// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package values_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestValues(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cs-applications Values Suite")
}
