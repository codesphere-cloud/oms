// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/util"
)

// Build the .tar.gz in-memory during test setup using the local fileContents map.
// This avoids reading from disk or using embed during tests.
// This map reflects the content that will be written into the tar archive.
var fileContents = map[string]string{
	"file1.txt":      "some text\n",
	"test/file2.txt": "some more text\n",
}
var _ = Describe("Tar", func() {
	var (
		archiveIn io.Reader
	)
	BeforeEach(func() {
		// Create an in-memory tar.gz containing the embedded files.
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)

		// Helper to add a file to the tar archive using the in-memory `fileContents` map.
		add := func(name, key string) {
			dataStr, ok := fileContents[key]
			Expect(ok).To(BeTrue(), "missing test data for %s", key)
			data := []byte(dataStr)
			hdr := &tar.Header{
				Name: name,
				Mode: 0600,
				Size: int64(len(data)),
			}
			Expect(tw.WriteHeader(hdr)).To(Succeed())
			_, err := tw.Write(data)
			Expect(err).NotTo(HaveOccurred())
		}

		add("file1.txt", "file1.txt")
		add("test/file2.txt", "test/file2.txt")

		Expect(tw.Close()).To(Succeed())
		Expect(gz.Close()).To(Succeed())

		archiveIn = bytes.NewReader(buf.Bytes())
	})

	Describe("StreamFileFromGzip", func() {
		It("streams a single file from a .tar.gz archive", func() {
			out, err := util.StreamFileFromGzip(archiveIn, "file1.txt")
			Expect(err).NotTo(HaveOccurred())
			outputFileContent, err := io.ReadAll(out)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(outputFileContent)).To(Equal(fileContents["file1.txt"]))
		})

		It("finds a file in a subdir of the .tar.gz archive", func() {
			out, err := util.StreamFileFromGzip(archiveIn, "file2.txt")
			Expect(err).NotTo(HaveOccurred())
			outputFileContent, err := io.ReadAll(out)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(outputFileContent)).To(Equal(fileContents["test/file2.txt"]))
		})

		It("Returns an error when the file doesn't exist", func() {
			out, err := util.StreamFileFromGzip(archiveIn, "file3.txt")
			Expect(out).To(BeNil())
			Expect(err).To(MatchError("file file3.txt not found in archive"))
		})
	})
})
