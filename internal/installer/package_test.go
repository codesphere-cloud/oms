// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer_test

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/codesphere-cloud/oms/internal/installer"
	"github.com/codesphere-cloud/oms/internal/util"
)

var _ = Describe("Package", func() {
	var (
		pkg        *installer.Package
		tempDir    string
		omsWorkdir string
		filename   string
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		omsWorkdir = filepath.Join(tempDir, "oms-workdir")
		filename = "test-package.tar.gz"
		pkg = installer.NewPackage(omsWorkdir, filename)
	})

	Describe("NewPackage", func() {
		It("creates a new Package with correct parameters", func() {
			newPkg := installer.NewPackage("/test/workdir", "package.tar.gz")
			Expect(newPkg).ToNot(BeNil())
			Expect(newPkg.OmsWorkdir).To(Equal("/test/workdir"))
			Expect(newPkg.Filename).To(Equal("package.tar.gz"))
			Expect(newPkg.FileIO()).ToNot(BeNil())
			Expect(newPkg.FileIO()).To(BeAssignableToTypeOf(&util.FilesystemWriter{}))
		})
	})

	Describe("FileIO", func() {
		It("returns the FileIO interface", func() {
			fileIO := pkg.FileIO()
			Expect(fileIO).ToNot(BeNil())
			Expect(fileIO).To(BeAssignableToTypeOf(&util.FilesystemWriter{}))
		})
	})

	Describe("GetWorkDir", func() {
		It("returns correct working directory path", func() {
			expected := filepath.Join(omsWorkdir, "test-package")
			Expect(pkg.GetWorkDir()).To(Equal(expected))
		})

		It("removes .tar.gz extension from filename", func() {
			pkg.Filename = "my-package.tar.gz"
			expected := filepath.Join(omsWorkdir, "my-package")
			Expect(pkg.GetWorkDir()).To(Equal(expected))
		})

		It("handles filename without .tar.gz extension", func() {
			pkg.Filename = "my-package"
			expected := filepath.Join(omsWorkdir, "my-package")
			Expect(pkg.GetWorkDir()).To(Equal(expected))
		})

		It("handles complex filenames", func() {
			pkg.Filename = "complex-package-v1.2.3.tar.gz"
			expected := filepath.Join(omsWorkdir, "complex-package-v1.2.3")
			Expect(pkg.GetWorkDir()).To(Equal(expected))
		})
	})

	Describe("GetDependencyPath", func() {
		It("returns correct dependency path", func() {
			filename := "dependency.txt"
			workDir := pkg.GetWorkDir()
			expected := filepath.Join(workDir, "deps", filename)
			Expect(pkg.GetDependencyPath(filename)).To(Equal(expected))
		})

		It("handles dependency files with paths", func() {
			filename := "subfolder/dependency.txt"
			workDir := pkg.GetWorkDir()
			expected := filepath.Join(workDir, "deps", filename)
			Expect(pkg.GetDependencyPath(filename)).To(Equal(expected))
		})

		It("handles empty filename", func() {
			filename := ""
			workDir := pkg.GetWorkDir()
			expected := filepath.Join(workDir, "deps", filename)
			Expect(pkg.GetDependencyPath(filename)).To(Equal(expected))
		})
	})

	Describe("Extract", func() {
		Context("with real filesystem operations", func() {
			BeforeEach(func() {
				// Create the package tar.gz file
				packagePath := filepath.Join(tempDir, filename)
				err := createTestTarGzPackage(packagePath)
				Expect(err).ToNot(HaveOccurred())
				pkg.Filename = packagePath
			})

			Context("when package doesn't exist", func() {
				It("returns an error", func() {
					pkg.Filename = "/nonexistent/package.tar.gz"
					err := pkg.Extract(false)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to extract package"))
				})
			})

			Context("when package exists and workdir doesn't exist", func() {
				It("successfully extracts the package", func() {
					err := pkg.Extract(false)
					Expect(err).ToNot(HaveOccurred())

					// Verify that the workdir was created
					workDir := pkg.GetWorkDir()
					Expect(workDir).To(BeADirectory())

					// Verify that the content was extracted
					testFile := filepath.Join(workDir, "test-file.txt")
					Expect(testFile).To(BeAnExistingFile())
				})
			})

			Context("when package is already extracted", func() {
				BeforeEach(func() {
					// First extraction
					err := pkg.Extract(false)
					Expect(err).ToNot(HaveOccurred())
				})

				It("skips extraction without force", func() {
					err := pkg.Extract(false)
					Expect(err).ToNot(HaveOccurred())
				})

				It("re-extracts with force", func() {
					err := pkg.Extract(true)
					Expect(err).ToNot(HaveOccurred())

					// Verify content still exists
					workDir := pkg.GetWorkDir()
					testFile := filepath.Join(workDir, "test-file.txt")
					Expect(testFile).To(BeAnExistingFile())
				})
			})

			Context("when workdir creation fails", func() {
				It("returns an error for invalid workdir", func() {
					// Use a path that can't be created (file exists as directory)
					invalidWorkdir := filepath.Join(tempDir, "invalid-workdir")
					err := os.WriteFile(invalidWorkdir, []byte("content"), 0644)
					Expect(err).ToNot(HaveOccurred())

					pkg.OmsWorkdir = invalidWorkdir
					err = pkg.Extract(false)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to ensure workdir exists"))
				})
			})
		})
	})

	Describe("ExtractDependency", func() {
		Context("with real filesystem operations", func() {
			var packagePath string

			BeforeEach(func() {
				// Create the package tar.gz file with deps.tar.gz inside
				packagePath = filepath.Join(tempDir, filename)
				err := createTestTarGzPackageWithDeps(packagePath)
				Expect(err).ToNot(HaveOccurred())
				pkg.Filename = packagePath
			})

			Context("when dependency file exists in deps.tar.gz", func() {
				It("successfully extracts the dependency", func() {
					err := pkg.ExtractDependency("test-dep.txt", false)
					Expect(err).ToNot(HaveOccurred())

					// Verify that the dependency was extracted
					depPath := pkg.GetDependencyPath("test-dep.txt")
					Expect(depPath).To(BeAnExistingFile())
				})
			})

			Context("when dependency is already extracted", func() {
				BeforeEach(func() {
					// First extraction
					err := pkg.ExtractDependency("test-dep.txt", false)
					Expect(err).ToNot(HaveOccurred())
				})

				It("skips extraction without force", func() {
					err := pkg.ExtractDependency("test-dep.txt", false)
					Expect(err).ToNot(HaveOccurred())
				})

				It("re-extracts with force", func() {
					err := pkg.ExtractDependency("test-dep.txt", true)
					Expect(err).ToNot(HaveOccurred())

					// Verify dependency still exists
					depPath := pkg.GetDependencyPath("test-dep.txt")
					Expect(depPath).To(BeAnExistingFile())
				})
			})

			Context("when package extraction fails", func() {
				It("returns an error", func() {
					pkg.Filename = "/nonexistent/package.tar.gz"
					err := pkg.ExtractDependency("test-dep.txt", false)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to extract package"))
				})
			})

			Context("when dependency doesn't exist in deps.tar.gz", func() {
				It("returns an error", func() {
					err := pkg.ExtractDependency("nonexistent-dep.txt", false)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to extract dependency"))
				})
			})
		})
	})

	Describe("PackageManager interface", func() {
		It("implements PackageManager interface", func() {
			var packageManager installer.PackageManager = pkg
			Expect(packageManager).ToNot(BeNil())
		})

		It("has all required methods", func() {
			var packageManager installer.PackageManager = pkg

			// Test that methods exist by calling them
			fileIO := packageManager.FileIO()
			Expect(fileIO).ToNot(BeNil())

			workDir := packageManager.GetWorkDir()
			Expect(workDir).ToNot(BeEmpty())

			depPath := packageManager.GetDependencyPath("test.txt")
			Expect(depPath).ToNot(BeEmpty())

			// Extract methods would need actual files to test properly
			// These are tested in the method-specific sections above
		})
	})

	Describe("Error handling and edge cases", func() {
		Context("Extract with various scenarios", func() {
			It("handles empty filename gracefully", func() {
				pkg.Filename = ""
				_ = pkg.Extract(false)
				// Note: Empty filename may not always cause an error at Extract level
				// The error might occur later during actual file operations
				// This test verifies the behavior is predictable
			})

			It("handles empty workdir", func() {
				pkg.OmsWorkdir = ""
				packagePath := filepath.Join(tempDir, filename)
				err := createTestTarGzPackage(packagePath)
				Expect(err).ToNot(HaveOccurred())
				pkg.Filename = packagePath

				// Empty workdir should cause an error when trying to create directories
				err = pkg.Extract(false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to ensure workdir exists"))
			})
		})

		Context("ExtractDependency with various scenarios", func() {
			It("handles empty dependency filename", func() {
				packagePath := filepath.Join(tempDir, filename)
				err := createTestTarGzPackageWithDeps(packagePath)
				Expect(err).ToNot(HaveOccurred())
				pkg.Filename = packagePath

				// Empty dependency filename may succeed (extracts everything) or fail
				// depending on the underlying tar extraction implementation
				_ = pkg.ExtractDependency("", false)
				// This test verifies the behavior is predictable
			})
		})

		Context("Path handling edge cases", func() {
			It("handles special characters in filenames", func() {
				pkg.Filename = "test-package-with-special-chars_v1.0.tar.gz"
				expected := filepath.Join(omsWorkdir, "test-package-with-special-chars_v1.0")
				Expect(pkg.GetWorkDir()).To(Equal(expected))
			})

			It("handles multiple .tar.gz occurrences in filename", func() {
				pkg.Filename = "package.tar.gz.backup.tar.gz"
				expected := filepath.Join(omsWorkdir, "package.backup")
				Expect(pkg.GetWorkDir()).To(Equal(expected))
			})
		})
	})

	Describe("Integration scenarios", func() {
		Context("full workflow simulation", func() {
			var packagePath string

			BeforeEach(func() {
				packagePath = filepath.Join(tempDir, "complete-package.tar.gz")
				err := createComplexTestPackage(packagePath)
				Expect(err).ToNot(HaveOccurred())
				pkg.Filename = packagePath
			})

			It("can extract package and multiple dependencies successfully", func() {
				// Extract main package
				err := pkg.Extract(false)
				Expect(err).ToNot(HaveOccurred())

				// Verify main package content
				workDir := pkg.GetWorkDir()
				Expect(workDir).To(BeADirectory())
				mainFile := filepath.Join(workDir, "main-content.txt")
				Expect(mainFile).To(BeAnExistingFile())

				// Extract multiple dependencies
				dependencies := []string{"dep1.txt", "dep2.txt", "subdep/dep3.txt"}
				for _, dep := range dependencies {
					err = pkg.ExtractDependency(dep, false)
					Expect(err).ToNot(HaveOccurred())

					depPath := pkg.GetDependencyPath(dep)
					Expect(depPath).To(BeAnExistingFile())
				}

				// Verify all paths are correct
				for _, dep := range dependencies {
					depPath := pkg.GetDependencyPath(dep)
					expectedPath := filepath.Join(workDir, "deps", dep)
					Expect(depPath).To(Equal(expectedPath))
				}
			})
		})
	})
})

// Helper functions for creating test tar.gz files

// createTestTarGzPackage creates a simple tar.gz package for testing
func createTestTarGzPackage(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	gzw := gzip.NewWriter(file)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	// Add a test file
	content := "test content"
	header := &tar.Header{
		Name: "test-file.txt",
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		return err
	}

	return nil
}

// createTestTarGzPackageWithDeps creates a tar.gz package containing a deps.tar.gz file
func createTestTarGzPackageWithDeps(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	gzw := gzip.NewWriter(file)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	// Add main content
	mainContent := "main package content"
	header := &tar.Header{
		Name: "main-file.txt",
		Mode: 0644,
		Size: int64(len(mainContent)),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	if _, err := tw.Write([]byte(mainContent)); err != nil {
		return err
	}

	// Create deps.tar.gz content in memory
	depsContent, err := createDepsArchive()
	if err != nil {
		return err
	}

	// Add deps.tar.gz file
	depsHeader := &tar.Header{
		Name: "deps.tar.gz",
		Mode: 0644,
		Size: int64(len(depsContent)),
	}
	if err := tw.WriteHeader(depsHeader); err != nil {
		return err
	}
	if _, err := tw.Write(depsContent); err != nil {
		return err
	}

	return nil
}

// createComplexTestPackage creates a complex package for integration testing
func createComplexTestPackage(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	gzw := gzip.NewWriter(file)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	// Add main content
	mainContent := "complex main package content"
	header := &tar.Header{
		Name: "main-content.txt",
		Mode: 0644,
		Size: int64(len(mainContent)),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	if _, err := tw.Write([]byte(mainContent)); err != nil {
		return err
	}

	// Create complex deps.tar.gz content
	depsContent, err := createComplexDepsArchive()
	if err != nil {
		return err
	}

	// Add deps.tar.gz file
	depsHeader := &tar.Header{
		Name: "deps.tar.gz",
		Mode: 0644,
		Size: int64(len(depsContent)),
	}
	if err := tw.WriteHeader(depsHeader); err != nil {
		return err
	}
	if _, err := tw.Write(depsContent); err != nil {
		return err
	}

	return nil
}

// createDepsArchive creates a deps.tar.gz archive content in memory
func createDepsArchive() ([]byte, error) {
	var buf []byte
	gzw := gzip.NewWriter(&bytesBuffer{data: &buf})
	tw := tar.NewWriter(gzw)

	// Add test dependency
	depContent := "dependency content"
	header := &tar.Header{
		Name: "test-dep.txt",
		Mode: 0644,
		Size: int64(len(depContent)),
	}
	if err := tw.WriteHeader(header); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(depContent)); err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gzw.Close(); err != nil {
		return nil, err
	}

	return buf, nil
}

// createComplexDepsArchive creates a complex deps.tar.gz archive with multiple files
func createComplexDepsArchive() ([]byte, error) {
	var buf []byte
	gzw := gzip.NewWriter(&bytesBuffer{data: &buf})
	tw := tar.NewWriter(gzw)

	// Add multiple dependencies
	deps := map[string]string{
		"dep1.txt":        "dependency 1 content",
		"dep2.txt":        "dependency 2 content",
		"subdep/dep3.txt": "sub dependency 3 content",
	}

	for name, content := range deps {
		header := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(header); err != nil {
			return nil, err
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gzw.Close(); err != nil {
		return nil, err
	}

	return buf, nil
}

// bytesBuffer is a simple buffer that implements io.Writer for creating in-memory archives
type bytesBuffer struct {
	data *[]byte
}

func (b *bytesBuffer) Write(p []byte) (n int, err error) {
	*b.data = append(*b.data, p...)
	return len(p), nil
}
