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

	Describe("NewWriteCounterWithTotal", func() {
		It("creates a WriteCounter with total and start bytes", func() {
			var underlying bytes.Buffer
			wc := portal.NewWriteCounterWithTotal(&underlying, 1000, 100)

			Expect(wc.Total).To(Equal(int64(1000)))
			Expect(wc.StartBytes).To(Equal(int64(100)))
			Expect(wc.Written).To(Equal(int64(0)))
			Expect(wc.Writer).To(Equal(&underlying))
		})

		It("writes data correctly to underlying writer", func() {
			var underlying bytes.Buffer
			wc := portal.NewWriteCounterWithTotal(&underlying, 100, 0)

			data := []byte("test data")
			n, err := wc.Write(data)

			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(len(data)))
			Expect(underlying.String()).To(Equal("test data"))
			Expect(wc.Written).To(Equal(int64(len(data))))
		})

		It("emits progress with percentage when total is known", func() {
			var logBuf bytes.Buffer
			prev := log.Writer()
			log.SetOutput(&logBuf)
			defer log.SetOutput(prev)

			var underlying bytes.Buffer
			wc := portal.NewWriteCounterWithTotal(&underlying, 100, 0)
			wc.LastUpdate = time.Now().Add(-time.Second)

			_, err := wc.Write([]byte("1234567890")) // 10 bytes of 100 total = 10%
			Expect(err).NotTo(HaveOccurred())

			out := logBuf.String()
			Expect(out).To(ContainSubstring("%"))
			Expect(out).To(ContainSubstring("ETA"))
		})

		It("handles resume downloads with start bytes offset", func() {
			var logBuf bytes.Buffer
			prev := log.Writer()
			log.SetOutput(&logBuf)
			defer log.SetOutput(prev)

			var underlying bytes.Buffer
			// Total is 100 bytes, starting at 50 (50% already downloaded)
			wc := portal.NewWriteCounterWithTotal(&underlying, 100, 50)
			wc.LastUpdate = time.Now().Add(-time.Second)

			// Write 25 more bytes (should now be at 75%)
			data := make([]byte, 25)
			_, err := wc.Write(data)
			Expect(err).NotTo(HaveOccurred())

			out := logBuf.String()
			Expect(out).To(ContainSubstring("75"))
		})
	})
})

var _ = Describe("formatDuration", func() {
	DescribeTable("formats durations correctly",
		func(d time.Duration, expected string) {
			result := portal.FormatDuration(d)
			Expect(result).To(Equal(expected))
		},
		Entry("less than a second", 500*time.Millisecond, "<1s"),
		Entry("exactly one second", 1*time.Second, "1s"),
		Entry("30 seconds", 30*time.Second, "30s"),
		Entry("59 seconds", 59*time.Second, "59s"),
		Entry("1 minute", 1*time.Minute, "1m0s"),
		Entry("1 minute 30 seconds", 1*time.Minute+30*time.Second, "1m30s"),
		Entry("5 minutes 45 seconds", 5*time.Minute+45*time.Second, "5m45s"),
		Entry("59 minutes 59 seconds", 59*time.Minute+59*time.Second, "59m59s"),
		Entry("1 hour", 1*time.Hour, "1h0m"),
		Entry("1 hour 30 minutes", 1*time.Hour+30*time.Minute, "1h30m"),
		Entry("2 hours 15 minutes", 2*time.Hour+15*time.Minute, "2h15m"),
	)
})

var _ = Describe("byteCountToHumanReadable", func() {
	DescribeTable("converts bytes correctly",
		func(bytes int64, expected string) {
			result := portal.ByteCountToHumanReadable(bytes)
			Expect(result).To(Equal(expected))
		},
		Entry("0 bytes", int64(0), "0 B"),
		Entry("512 bytes", int64(512), "512 B"),
		Entry("1023 bytes", int64(1023), "1023 B"),
		Entry("1 KB", int64(1024), "1.0 KB"),
		Entry("1.5 KB", int64(1536), "1.5 KB"),
		Entry("1 MB", int64(1024*1024), "1.0 MB"),
		Entry("1.5 MB", int64(1536*1024), "1.5 MB"),
		Entry("1 GB", int64(1024*1024*1024), "1.0 GB"),
	)
})
