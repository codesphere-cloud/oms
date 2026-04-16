// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testuser

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTestuser(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Testuser Suite")
}
