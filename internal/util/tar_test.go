// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"io"

	"embed"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/util"
)

// I didn't find a good way to do this in memory without just reversing the code under test.
// While this is not ideal, at least it doesn't read from file during the test run but during compilation.
//
//go:generate mkdir -p testdata/test
//go:generate sh -c "echo some text > testdata/file1.txt"
//go:generate sh -c "echo some more text > testdata/test/file2.txt"
//go:generate sh -c "cd testdata && tar cfz testdata1.tar.gz file1.txt test/file2.txt"
//go:embed testdata
var testdata1 embed.FS

// this just reflects what's in the tar but doesn't influence the actual contents.
var fileContents = map[string]string{
	"file1.txt":      "some text\n",
	"test/file2.txt": "some more text\n",
}
var _ = Describe("Tar", func() {
	var (
		archiveIn io.Reader
	)
	BeforeEach(func() {
		var err error
		archiveIn, err = testdata1.Open("testdata/testdata1.tar.gz")
		Expect(err).Error().NotTo(HaveOccurred())
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
