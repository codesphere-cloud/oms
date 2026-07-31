// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package datacenter_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDataCenter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DataCenter Suite")
}
