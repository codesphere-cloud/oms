// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package pcapps_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPcApps(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PC-Apps Suite")
}
