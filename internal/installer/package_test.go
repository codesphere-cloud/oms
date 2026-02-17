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
		pkg = installer.NewPackage(omsWorkdir, filename).(*installer.Package)
	})

	Describe("NewPackage", func() {
		It("creates a new Package with correct parameters", func() {
			newPkg := installer.NewPackage("/test/workdir", "package.tar.gz").(*installer.Package)
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
	})

	Describe("Extract", func() {
		Context("with real filesystem operations", func() {
			BeforeEach(func() {
				// Create the package tar.gz file
				packagePath := filepath.Join(tempDir, filename)
				err := createTestPackage(packagePath, PackageFiles{
					MainFiles: map[string]string{
						"test-file.txt": "test content",
					},
				})
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

			Context("when package checksum changes", func() {
				var checksumFile string

				BeforeEach(func() {
					checksumFile = pkg.Filename + ".md5"
					err := os.WriteFile(checksumFile, []byte("original-checksum-123"), 0644)
					Expect(err).ToNot(HaveOccurred())

					err = pkg.Extract(false)
					Expect(err).ToNot(HaveOccurred())
				})

				It("re-extracts when checksum changes", func() {
					workDir := pkg.GetWorkDir()
					markerFile := filepath.Join(workDir, ".oms-package-checksum")
					Expect(markerFile).To(BeAnExistingFile())
					content, err := os.ReadFile(markerFile)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(content)).To(Equal("original-checksum-123"))

					testFile := filepath.Join(workDir, "test-file.txt")
					err = os.WriteFile(testFile, []byte("modified content"), 0644)
					Expect(err).ToNot(HaveOccurred())

					err = os.WriteFile(checksumFile, []byte("new-checksum-456"), 0644)
					Expect(err).ToNot(HaveOccurred())

					err = pkg.Extract(false)
					Expect(err).ToNot(HaveOccurred())

					actualContent, err := os.ReadFile(testFile)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(actualContent)).To(Equal("test content"))

					content, err = os.ReadFile(markerFile)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(content)).To(Equal("new-checksum-456"))
				})

				It("skips extraction when checksum is the same", func() {
					workDir := pkg.GetWorkDir()
					markerFile := filepath.Join(workDir, ".oms-package-checksum")
					Expect(markerFile).To(BeAnExistingFile())

					testFile := filepath.Join(workDir, "test-file.txt")
					err := os.WriteFile(testFile, []byte("modified content"), 0644)
					Expect(err).ToNot(HaveOccurred())

					err = pkg.Extract(false)
					Expect(err).ToNot(HaveOccurred())

					actualContent, err := os.ReadFile(testFile)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(actualContent)).To(Equal("modified content"))
				})

				It("re-extracts when marker file is missing", func() {
					workDir := pkg.GetWorkDir()
					markerFile := filepath.Join(workDir, ".oms-package-checksum")

					err := os.Remove(markerFile)
					Expect(err).ToNot(HaveOccurred())

					testFile := filepath.Join(workDir, "test-file.txt")
					err = os.WriteFile(testFile, []byte("modified content"), 0644)
					Expect(err).ToNot(HaveOccurred())

					err = pkg.Extract(false)
					Expect(err).ToNot(HaveOccurred())

					actualContent, err := os.ReadFile(testFile)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(actualContent)).To(Equal("test content"))
				})
			})

			Context("when no checksum sidecar file exists", func() {
				BeforeEach(func() {
					err := pkg.Extract(false)
					Expect(err).ToNot(HaveOccurred())
				})

				It("skips extraction without checksum (backward compatibility)", func() {
					workDir := pkg.GetWorkDir()
					testFile := filepath.Join(workDir, "test-file.txt")
					err := os.WriteFile(testFile, []byte("modified content"), 0644)
					Expect(err).ToNot(HaveOccurred())

					err = pkg.Extract(false)
					Expect(err).ToNot(HaveOccurred())

					actualContent, err := os.ReadFile(testFile)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(actualContent)).To(Equal("modified content"))
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
				err := createTestPackage(packagePath, PackageFiles{
					MainFiles: map[string]string{
						"main-file.txt": "main package content",
					},
					DepsFiles: map[string]string{
						"test-dep.txt": "dependency content",
					},
				})
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
})

// Tests for ExtractOciImageIndex (moved from config_test.go)
var _ = Describe("Package ExtractOciImageIndex", func() {
	var (
		pkg     *installer.Package
		tempDir string
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		pkg = installer.NewPackage(tempDir, "test-package.tar.gz").(*installer.Package)
	})

	Describe("ExtractOciImageIndex", func() {
		Context("with real filesystem operations", func() {
			var imageFile string

			BeforeEach(func() {
				imageFile = filepath.Join(tempDir, "test-image.tar")
			})

			Context("when image file does not exist", func() {
				It("returns an error", func() {
					_, err := pkg.ExtractOciImageIndex(imageFile)

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to extract index.json"))
				})
			})

			Context("when image file is empty", func() {
				It("returns an error", func() {
					// Create empty tar file
					err := os.WriteFile(imageFile, []byte(""), 0644)
					Expect(err).ToNot(HaveOccurred())

					_, err = pkg.ExtractOciImageIndex(imageFile)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to extract index.json"))
				})
			})

			Context("when image file is a directory", func() {
				It("returns an error", func() {
					// Create directory instead of file
					err := os.Mkdir(imageFile, 0755)
					Expect(err).ToNot(HaveOccurred())

					_, err = pkg.ExtractOciImageIndex(imageFile)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to extract index.json"))
				})
			})

			Context("when index.json file doesn't exist after extraction", func() {
				It("returns an error", func() {
					// Create a minimal tar file without index.json
					err := createTar(imageFile, "not_index.json", "fake content")
					Expect(err).ToNot(HaveOccurred())

					_, err = pkg.ExtractOciImageIndex(imageFile)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to extract index.json"))
				})
			})

			Context("when tar contains valid index.json", func() {
				It("successfully extracts and parses OCI image index", func() {
					// Create a tar file with a valid index.json
					validIndex := `{
						"schemaVersion": 2,
						"mediaType": "application/vnd.oci.image.index.v1+json",
						"manifests": [
							{
								"mediaType": "application/vnd.oci.image.manifest.v1+json",
								"size": 1234,
								"digest": "sha256:abc123def456"
							}
						]
					}`
					err := createTar(imageFile, "index.json", validIndex)
					Expect(err).ToNot(HaveOccurred())

					ociImageIndex, err := pkg.ExtractOciImageIndex(imageFile)
					Expect(err).ToNot(HaveOccurred())
					Expect(ociImageIndex.SchemaVersion).To(Equal(2))
					Expect(ociImageIndex.MediaType).To(Equal("application/vnd.oci.image.index.v1+json"))
					Expect(ociImageIndex.Manifests).To(HaveLen(1))
					Expect(ociImageIndex.Manifests[0].Digest).To(Equal("sha256:abc123def456"))
					Expect(ociImageIndex.Manifests[0].Size).To(Equal(int64(1234)))
				})
			})

			Context("when index.json has invalid JSON", func() {
				It("returns an error", func() {
					// Create a tar file with invalid JSON in index.json
					invalidIndex := `{
						"schemaVersion": 2,
						"manifests": [
							{
								"size": "invalid_json_here",
					`
					err := createTar(imageFile, "index.json", invalidIndex)
					Expect(err).ToNot(HaveOccurred())

					_, err = pkg.ExtractOciImageIndex(imageFile)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to parse OCI image config"))
				})
			})
		})
	})
})

// Tests for GetBaseimageName
var _ = Describe("Package GetBaseimageName", func() {
	var (
		pkg     *installer.Package
		tempDir string
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		omsWorkdir := filepath.Join(tempDir, "oms-workdir")
		pkg = installer.NewPackage(omsWorkdir, "test-package.tar.gz").(*installer.Package)
	})

	Describe("GetBaseimageName", func() {
		Context("when baseimage parameter is empty", func() {
			It("returns an error", func() {
				_, err := pkg.GetBaseimageName("")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("baseimage not specified"))
			})
		})

		Context("when bom.json file does not exist", func() {
			It("returns an error", func() {
				_, err := pkg.GetBaseimageName("workspace-agent-24.04")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load bom.json"))
			})
		})

		Context("when bom.json exists but is invalid", func() {
			It("returns an error", func() {
				// Create invalid bom.json
				workDir := pkg.GetWorkDir()
				err := os.MkdirAll(filepath.Join(workDir, "deps"), 0755)
				Expect(err).NotTo(HaveOccurred())

				bomPath := pkg.GetDependencyPath("bom.json")
				err = os.WriteFile(bomPath, []byte("invalid json"), 0644)
				Expect(err).NotTo(HaveOccurred())

				_, err = pkg.GetBaseimageName("workspace-agent-24.04")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load bom.json"))
			})
		})

		Context("when bom.json exists but codesphere component is missing", func() {
			It("returns an error", func() {
				// Create bom.json without codesphere component
				workDir := pkg.GetWorkDir()
				err := os.MkdirAll(filepath.Join(workDir, "deps"), 0755)
				Expect(err).NotTo(HaveOccurred())

				bomContent := `{
					"components": {
						"docker": {
							"files": {}
						}
					}
				}`
				bomPath := pkg.GetDependencyPath("bom.json")
				err = os.WriteFile(bomPath, []byte(bomContent), 0644)
				Expect(err).NotTo(HaveOccurred())

				_, err = pkg.GetBaseimageName("workspace-agent-24.04")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get codesphere container images from bom.json"))
			})
		})

		Context("when baseimage is not found in bom.json", func() {
			It("returns an error", func() {
				// Create bom.json with codesphere component but without the requested baseimage
				workDir := pkg.GetWorkDir()
				err := os.MkdirAll(filepath.Join(workDir, "deps"), 0755)
				Expect(err).NotTo(HaveOccurred())

				bomContent := `{
					"components": {
						"codesphere": {
							"containerImages": {
								"workspace-agent-20.04": "ghcr.io/codesphere-cloud/workspace-agent-20.04:v1.0.0"
							}
						}
					}
				}`
				bomPath := pkg.GetDependencyPath("bom.json")
				err = os.WriteFile(bomPath, []byte(bomContent), 0644)
				Expect(err).NotTo(HaveOccurred())

				_, err = pkg.GetBaseimageName("workspace-agent-24.04")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("baseimage workspace-agent-24.04 not found in bom.json"))
			})
		})

		Context("when baseimage exists in bom.json", func() {
			BeforeEach(func() {
				// Create valid bom.json with the requested baseimage
				workDir := pkg.GetWorkDir()
				err := os.MkdirAll(filepath.Join(workDir, "deps"), 0755)
				Expect(err).NotTo(HaveOccurred())

				bomContent := `{
					"components": {
						"codesphere": {
							"containerImages": {
								"workspace-agent-24.04": "ghcr.io/codesphere-cloud/workspace-agent-24.04:codesphere-v1.66.0",
								"workspace-agent-20.04": "ghcr.io/codesphere-cloud/workspace-agent-20.04:codesphere-v1.65.0"
							}
						}
					}
				}`
				bomPath := pkg.GetDependencyPath("bom.json")
				err = os.WriteFile(bomPath, []byte(bomContent), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns the correct image name", func() {
				imageName, err := pkg.GetBaseimageName("workspace-agent-24.04")
				Expect(err).NotTo(HaveOccurred())
				Expect(imageName).To(Equal("ghcr.io/codesphere-cloud/workspace-agent-24.04:codesphere-v1.66.0"))
			})

			It("returns the correct image name for different baseimage", func() {
				imageName, err := pkg.GetBaseimageName("workspace-agent-20.04")
				Expect(err).NotTo(HaveOccurred())
				Expect(imageName).To(Equal("ghcr.io/codesphere-cloud/workspace-agent-20.04:codesphere-v1.65.0"))
			})
		})
	})
})

// Tests for GetBaseimagePath
var _ = Describe("Package GetBaseimagePath", func() {
	var (
		pkg     *installer.Package
		tempDir string
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		omsWorkdir := filepath.Join(tempDir, "oms-workdir")
		pkg = installer.NewPackage(omsWorkdir, "test-package.tar.gz").(*installer.Package)
	})

	Describe("GetBaseimagePath", func() {
		Context("when baseimage parameter is empty", func() {
			It("returns an error", func() {
				_, err := pkg.GetBaseimagePath("", false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("baseimage not specified"))
			})
		})

		Context("when ExtractDependency fails", func() {
			It("returns an error", func() {
				// Try to extract non-existent dependency
				_, err := pkg.GetBaseimagePath("nonexistent-image", false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to extract package to workdir"))
			})
		})

		Context("with successful dependency extraction", func() {
			BeforeEach(func() {
				// Create the main package with deps.tar.gz
				packagePath := filepath.Join(tempDir, "test-package.tar.gz")
				err := createTestPackage(packagePath, PackageFiles{
					MainFiles: map[string]string{
						"main-file.txt": "main package content",
					},
					DepsFiles: map[string]string{
						"./codesphere/images/workspace-agent-24.04.tar": "fake image content",
					},
				})
				Expect(err).NotTo(HaveOccurred())
				pkg.Filename = packagePath
			})

			It("returns correct path for baseimage without .tar extension", func() {
				path, err := pkg.GetBaseimagePath("workspace-agent-24.04", false)
				Expect(err).NotTo(HaveOccurred())

				expectedPath := pkg.GetDependencyPath("./codesphere/images/workspace-agent-24.04.tar")
				Expect(path).To(Equal(expectedPath))
			})

			It("returns correct path for baseimage with .tar extension", func() {
				path, err := pkg.GetBaseimagePath("workspace-agent-24.04.tar", false)
				Expect(err).NotTo(HaveOccurred())

				expectedPath := pkg.GetDependencyPath("./codesphere/images/workspace-agent-24.04.tar")
				Expect(path).To(Equal(expectedPath))
			})

			It("uses force parameter correctly", func() {
				// First extraction
				_, err := pkg.GetBaseimagePath("workspace-agent-24.04", false)
				Expect(err).NotTo(HaveOccurred())

				// Second extraction with force
				path, err := pkg.GetBaseimagePath("workspace-agent-24.04", true)
				Expect(err).NotTo(HaveOccurred())

				expectedPath := pkg.GetDependencyPath("./codesphere/images/workspace-agent-24.04.tar")
				Expect(path).To(Equal(expectedPath))
			})
		})
	})
})

// Tests for GetCodesphereVersion
var _ = Describe("Package GetCodesphereVersion", func() {
	var (
		pkg     *installer.Package
		tempDir string
	)

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
		omsWorkdir := filepath.Join(tempDir, "oms-workdir")
		pkg = installer.NewPackage(omsWorkdir, "test-package.tar.gz").(*installer.Package)
	})

	Describe("GetCodesphereVersion", func() {
		Context("when bom.json file does not exist", func() {
			It("returns an error", func() {
				_, err := pkg.GetCodesphereVersion()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load bom.json"))
			})
		})

		Context("when bom.json exists but is invalid", func() {
			It("returns an error", func() {
				// Create invalid bom.json
				workDir := pkg.GetWorkDir()
				err := os.MkdirAll(filepath.Join(workDir, "deps"), 0755)
				Expect(err).NotTo(HaveOccurred())

				bomPath := pkg.GetDependencyPath("bom.json")
				err = os.WriteFile(bomPath, []byte("invalid json"), 0644)
				Expect(err).NotTo(HaveOccurred())

				_, err = pkg.GetCodesphereVersion()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load bom.json"))
			})
		})

		Context("when bom.json exists but codesphere component is missing", func() {
			It("returns an error", func() {
				// Create bom.json without codesphere component
				workDir := pkg.GetWorkDir()
				err := os.MkdirAll(filepath.Join(workDir, "deps"), 0755)
				Expect(err).NotTo(HaveOccurred())

				bomContent := `{
					"components": {
						"docker": {
							"files": {}
						}
					}
				}`
				bomPath := pkg.GetDependencyPath("bom.json")
				err = os.WriteFile(bomPath, []byte(bomContent), 0644)
				Expect(err).NotTo(HaveOccurred())

				_, err = pkg.GetCodesphereVersion()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get codesphere container images from bom.json"))
			})
		})

		Context("when no container images with :codesphere exist", func() {
			BeforeEach(func() {
				// Create bom.json with images but no :codesphere versions
				workDir := pkg.GetWorkDir()
				err := os.MkdirAll(filepath.Join(workDir, "deps"), 0755)
				Expect(err).NotTo(HaveOccurred())

				bomContent := `{
					"components": {
						"codesphere": {
							"containerImages": {
								"workspace-agent-24.04": "ghcr.io/codesphere-cloud/workspace-agent-24.04:v1.0.0",
								"auth-service": "ghcr.io/codesphere-cloud/auth-service:latest"
							}
						}
					}
				}`
				bomPath := pkg.GetDependencyPath("bom.json")
				err = os.WriteFile(bomPath, []byte(bomContent), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an error", func() {
				_, err := pkg.GetCodesphereVersion()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no container images found in bom.json"))
			})
		})

		Context("when valid codesphere versions exist", func() {
			It("returns a valid codesphere version", func() {
				// Create bom.json with multiple different versions (should pick the first one found)
				workDir := pkg.GetWorkDir()
				err := os.MkdirAll(filepath.Join(workDir, "deps"), 0755)
				Expect(err).NotTo(HaveOccurred())

				bomContent := `{
					"components": {
						"codesphere": {
							"containerImages": {
								"workspace-agent-24.04": "ghcr.io/codesphere-cloud/workspace-agent-24.04:codesphere-v1.66.0",
								"auth-service": "ghcr.io/codesphere-cloud/auth-service:codesphere-v1.65.0"
							}
						}
					}
				}`
				bomPath := pkg.GetDependencyPath("bom.json")
				err = os.WriteFile(bomPath, []byte(bomContent), 0644)
				Expect(err).NotTo(HaveOccurred())

				version, err := pkg.GetCodesphereVersion()
				Expect(err).NotTo(HaveOccurred())
				// Should return one of the codesphere versions (depends on map iteration order)
				Expect(version).To(Or(Equal("codesphere-v1.66.0"), Equal("codesphere-v1.65.0")))
			})
		})

		Context("when codesphere-lts version exists", func() {
			It("returns a valid codesphere version", func() {
				// Create bom.json with codesphere-lts version
				workDir := pkg.GetWorkDir()
				err := os.MkdirAll(filepath.Join(workDir, "deps"), 0755)
				Expect(err).NotTo(HaveOccurred())

				bomContent := `{
					"components": {
						"codesphere": {
							"containerImages": {
								"workspace-agent-24.04": "ghcr.io/codesphere-cloud/workspace-agent-24.04:codesphere-lts-v1.70.0"
							}
						}
					}
				}`
				bomPath := pkg.GetDependencyPath("bom.json")
				err = os.WriteFile(bomPath, []byte(bomContent), 0644)
				Expect(err).NotTo(HaveOccurred())

				version, err := pkg.GetCodesphereVersion()
				Expect(err).NotTo(HaveOccurred())
				Expect(version).To(Equal("codesphere-lts-v1.70.0"))
			})
		})
	})
})

// Helper functions for creating test tar.gz files

// PackageFiles represents files to include in a test package
type PackageFiles struct {
	MainFiles map[string]string // filename -> content
	DepsFiles map[string]string // filename -> content for deps.tar.gz
}

// createTar creates a tar file containing a file with the given content
func createTar(tarName string, fileName string, fileContent string) error {
	file, err := os.Create(tarName)
	if err != nil {
		return err
	}
	defer util.CloseFileIgnoreError(file)

	tw := tar.NewWriter(file)
	defer func() { _ = tw.Close() }()

	header := &tar.Header{
		Name: fileName,
		Mode: 0644,
		Size: int64(len(fileContent)),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	if _, err := tw.Write([]byte(fileContent)); err != nil {
		return err
	}

	return nil
}

// createTarGz creates a deps.tar.gz archive content in memory
func createTarGz(files map[string]string) ([]byte, error) {
	var buf []byte
	gzw := gzip.NewWriter(&bytesBuffer{data: &buf})
	tw := tar.NewWriter(gzw)

	for name, content := range files {
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

// createTestPackage creates a tar.gz package with the specified files
func createTestPackage(filename string, files PackageFiles) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer util.CloseFileIgnoreError(file)

	gzw := gzip.NewWriter(file)
	defer func() { _ = gzw.Close() }()

	tw := tar.NewWriter(gzw)
	defer func() { _ = tw.Close() }()

	// Add main files
	for name, content := range files.MainFiles {
		header := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return err
		}
	}

	// Add deps.tar.gz if dependency files are specified
	if len(files.DepsFiles) > 0 {
		depsContent, err := createTarGz(files.DepsFiles)
		if err != nil {
			return err
		}

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
	}

	return nil
}

// bytesBuffer is a simple buffer that implements io.Writer for creating in-memory archives
type bytesBuffer struct {
	data *[]byte
}

func (b *bytesBuffer) Write(p []byte) (n int, err error) {
	*b.data = append(*b.data, p...)
	return len(p), nil
}
