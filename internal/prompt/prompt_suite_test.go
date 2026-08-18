// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package prompt

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPrompt(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Prompt Suite")
}
