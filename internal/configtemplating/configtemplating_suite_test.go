// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package configtemplating_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfigTemplating(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Templating Suite")
}
