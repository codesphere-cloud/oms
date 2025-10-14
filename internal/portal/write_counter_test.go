// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package portal_test

import (
	"bytes"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/portal"
)

var _ = Describe("WriteCounter", func() {
	It("emits progress logs on write", func() {
		// capture log output
		var logBuf bytes.Buffer
		prev := log.Writer()
		log.SetOutput(&logBuf)
		defer log.SetOutput(prev)

		var underlying bytes.Buffer
		wc := portal.NewWriteCounter(&underlying)

		// force an update by setting LastUpdate sufficiently in the past
		wc.LastUpdate = time.Now().Add(-time.Second)

		_, err := wc.Write([]byte("hello world"))
		Expect(err).NotTo(HaveOccurred())

		out := logBuf.String()
		Expect(out).NotTo(BeEmpty())
	})
})
