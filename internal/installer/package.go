// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package installer

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/oms/internal/installer/files"
	"github.com/codesphere-cloud/oms/internal/util"
)

const depsDir = "deps"
const depsTar = "deps.tar.gz"
const checksumMarkerFile = ".oms-package-checksum"

type PackageManager interface {
	FileIO() util.FileIO
	GetWorkDir() string
	GetDependencyPath(filename string) string
	Extract(force bool) error
	ExtractDependency(file string, force bool) error
	ExtractOciImageIndex(imagefile string) (files.OCIImageIndex, error)
	GetBaseimageName(baseimage string) (string, error)
	GetBaseimagePath(baseimage string, force bool) (string, error)
	GetCodesphereVersion() (string, error)
}

type Package struct {
	OmsWorkdir string
	Filename   string
	fileIO     util.FileIO
}

func NewPackage(omsWorkdir, filename string) PackageManager {
	return &Package{
		Filename:   filename,
		OmsWorkdir: omsWorkdir,
		fileIO:     &util.FilesystemWriter{},
	}
}

// FileIO returns the FileIO interface used by the package.
func (p *Package) FileIO() util.FileIO {
	return p.fileIO
}

// GetWorkDir returns the working directory path for the package
// by joining the OmsWorkdir and the filename (without the .tar.gz extension).
func (p *Package) GetWorkDir() string {
	return path.Join(p.OmsWorkdir, strings.ReplaceAll(p.Filename, ".tar.gz", ""))
}

// GetDependencyPath returns the full path to a dependency file within the package's deps directory.
func (p *Package) GetDependencyPath(filename string) string {
	workDir := p.GetWorkDir()
	return path.Join(workDir, depsDir, filename)
}

// alreadyExtracted checks if the package has already been extracted to the given directory.
func (p *Package) alreadyExtracted(dir string) (bool, error) {
	if !p.fileIO.Exists(dir) {
		return false, nil
	}
	isDir, err := p.fileIO.IsDirectory(dir)
	if err != nil {
		return false, fmt.Errorf("failed to determine if %s is a folder: %w", dir, err)
	}
	return isDir, nil
}

// getPackageChecksum reads the checksum from the sidecar .md5 file created during download.
func (p *Package) getPackageChecksum() string {
	checksumFile := p.Filename + ".md5"
	data, err := p.fileIO.ReadFile(checksumFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// getExtractedChecksum reads the checksum stored in the workdir's marker file.
func (p *Package) getExtractedChecksum(workDir string) string {
	markerPath := path.Join(workDir, checksumMarkerFile)
	data, err := p.fileIO.ReadFile(markerPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// saveExtractedChecksum writes the checksum to the workdir's marker file.
func (p *Package) saveExtractedChecksum(workDir, checksum string) error {
	if checksum == "" {
		return nil
	}
	markerPath := path.Join(workDir, checksumMarkerFile)
	return p.fileIO.WriteFile(markerPath, []byte(checksum), 0644)
}

// packageChanged checks if the package is different from the one that was previously extracted.
func (p *Package) packageChanged(workDir string) bool {
	packageChecksum := p.getPackageChecksum()
	if packageChecksum == "" {
		return false
	}

	extractedChecksum := p.getExtractedChecksum(workDir)
	if extractedChecksum == "" {
		log.Println("No checksum marker found in extracted directory, will re-extract to ensure consistency.")
		return true
	}

	if packageChecksum != extractedChecksum {
		log.Printf("Package checksum changed (was: %s, now: %s), will re-extract.", extractedChecksum, packageChecksum)
		return true
	}

	return false
}

// Extract extracts the package tar.gz file into its working directory.
// If force is true, it will overwrite existing files.
// If the package checksum has changed since last extraction, it will also re-extract.
func (p *Package) Extract(force bool) error {
	workDir := p.GetWorkDir()
	err := os.MkdirAll(p.OmsWorkdir, 0755)
	if err != nil {
		return fmt.Errorf("failed to ensure workdir exists: %w", err)
	}

	alreadyExtracted, err := p.alreadyExtracted(workDir)
	if err != nil {
		return fmt.Errorf("failed to figure out if package %s is already extracted in %s: %w", p.Filename, workDir, err)
	}

	// Check if the package has changed since last extraction
	needsReExtraction := p.packageChanged(workDir)

	if alreadyExtracted && !force && !needsReExtraction {
		log.Println("Skipping extraction, package already unpacked. Use force option to overwrite.")
		return nil
	}

	if needsReExtraction && !force {
		log.Println("Package has changed, re-extracting...")
	}

	err = util.ExtractTarGz(p.fileIO, p.Filename, workDir)
	if err != nil {
		return fmt.Errorf("failed to extract package %s to %s: %w", p.Filename, workDir, err)
	}

	depsArchivePath := path.Join(workDir, depsTar)
	if p.fileIO.Exists(depsArchivePath) {
		depsTargetDir := path.Join(workDir, depsDir)
		err = util.ExtractTarGz(p.fileIO, depsArchivePath, depsTargetDir)
		if err != nil {
			return fmt.Errorf("failed to extract deps.tar.gz to %s: %w", depsTargetDir, err)
		}
	}

	packageChecksum := p.getPackageChecksum()
	err = p.saveExtractedChecksum(workDir, packageChecksum)
	if err != nil {
		log.Printf("Warning: failed to save checksum marker: %v", err)
	}

	return nil
}

// ExtractDependency extracts a specific dependency file from the deps.tar.gz archive within the package.
func (p *Package) ExtractDependency(file string, force bool) error {
	err := p.Extract(force)
	if err != nil {
		return fmt.Errorf("failed to extract package: %w", err)
	}
	workDir := p.GetWorkDir()

	if p.fileIO.Exists(p.GetDependencyPath(file)) && !force {
		log.Println("Skipping extraction, dependency already unpacked. Use force option to overwrite.")
		return nil
	}

	err = util.ExtractTarGzSingleFile(p.fileIO, path.Join(workDir, "deps.tar.gz"), file, path.Join(workDir, depsDir))
	if err != nil {
		return fmt.Errorf("failed to extract dependency %s from deps archive to %s: %w", file, workDir, err)
	}

	return err
}

// ExtractOciImageIndex extracts and parses the OCI image index from the given image file path.
func (p *Package) ExtractOciImageIndex(imagefile string) (files.OCIImageIndex, error) {
	var ociImageIndex files.OCIImageIndex
	err := util.ExtractTarSingleFile(p.fileIO, imagefile, "index.json", filepath.Dir(imagefile))
	if err != nil {
		return ociImageIndex, fmt.Errorf("failed to extract index.json: %w", err)
	}

	err = ociImageIndex.ParseOCIImageConfig(imagefile)
	if err != nil {
		return ociImageIndex, fmt.Errorf("failed to parse OCI image config: %w", err)
	}

	return ociImageIndex, nil
}

func (p *Package) GetBaseimageName(baseimage string) (string, error) {
	if baseimage == "" {
		return "", fmt.Errorf("baseimage not specified")
	}

	bomJson := files.BomConfig{}
	err := bomJson.ParseBomConfig(p.GetDependencyPath("bom.json"))
	if err != nil {
		return "", fmt.Errorf("failed to load bom.json: %w", err)
	}

	containerImages, err := bomJson.GetCodesphereContainerImages()
	if err != nil {
		return "", fmt.Errorf("failed to get codesphere container images from bom.json: %w", err)
	}

	imageName, exists := containerImages[baseimage]
	if !exists {
		return "", fmt.Errorf("baseimage %s not found in bom.json", baseimage)
	}

	return imageName, nil
}

const baseimagePath = "./codesphere/images"

func (p *Package) GetBaseimagePath(baseimage string, force bool) (string, error) {
	if baseimage == "" {
		return "", fmt.Errorf("baseimage not specified")
	}

	if !strings.HasSuffix(baseimage, ".tar") {
		baseimage = baseimage + ".tar"
	}

	baseImageTarPath := path.Join(baseimagePath, baseimage)
	err := p.ExtractDependency(baseImageTarPath, force)
	if err != nil {
		return "", fmt.Errorf("failed to extract package to workdir: %w", err)
	}

	baseimagePath := p.GetDependencyPath(baseImageTarPath)

	return baseimagePath, nil
}

func (p *Package) GetCodesphereVersion() (string, error) {
	bomJson := files.BomConfig{}
	err := bomJson.ParseBomConfig(p.GetDependencyPath("bom.json"))
	if err != nil {
		return "", fmt.Errorf("failed to load bom.json: %w", err)
	}

	containerImages, err := bomJson.GetCodesphereContainerImages()
	if err != nil {
		return "", fmt.Errorf("failed to get codesphere container images from bom.json: %w", err)
	}

	containerImage := ""
	for _, image := range containerImages {
		if strings.Contains(image, ":codesphere") {
			containerImage = image
			break
		}
	}

	if containerImage == "" {
		return "", fmt.Errorf("no container images found in bom.json")
	}

	parts := strings.Split(containerImage, ":")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid image name format: %s", containerImage)
	}

	return parts[len(parts)-1], nil
}
