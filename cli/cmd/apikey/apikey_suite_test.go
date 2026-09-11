// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package apikey_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApiKey(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApiKey Suite")
}
